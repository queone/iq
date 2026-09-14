// Package usage renders the shared help pages for every command-line utility in this module.
//
// One renderer produces every page, so each utility and each of its commands prints the same
// three-line header, the same section order (Usage, Commands, Options, Examples), and the same
// alignment. Cobra commands feed the renderer through Install.
package usage

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"iq/internal/color"
)

// Annotation keys read by the cobra adapter.
const (
	// SynopsisKey holds extra Usage forms, one per line, written relative to the command path.
	SynopsisKey = "usage.synopsis"
	// NoteKey overrides the paragraph that closes the Commands section.
	NoteKey = "usage.commands-note"
)

// noteWidth is the column at which section paragraphs wrap, leaving room for the two-space indent.
const noteWidth = 76

// ErrVersionPrinted reports that -v or --version printed the version instead of running a command.
// Callers treat it as a successful exit; roots silence cobra error and usage output so nothing else prints.
var ErrVersionPrinted = errors.New("version printed")

// Header is the three-line page header shared by every command page of one utility.
type Header struct {
	Name        string // utility name, bold on line one
	Version     string // bare version, plain on line one
	Description string // one line, no trailing period, line two
	URL         string // URL without scheme, line three
}

// Row is one form/meaning pair inside a section body.
type Row struct {
	Form    string
	Meaning string
}

// Page is one resolved help page.
type Page struct {
	Header       Header
	Synopsis     []string // one Usage line per invocation form
	UsageNote    string   // optional paragraph closing Usage
	Commands     []Row
	CommandsNote string // optional paragraph closing Commands
	Options      []Row  // Render appends the version and help rows
	Examples     []string
}

// Render writes page to w with the shared header, section order, and alignment rules.
func Render(w io.Writer, page Page) {
	h := page.Header
	var b strings.Builder
	fmt.Fprintf(&b, "%s v%s\n", color.Whi5B(h.Name), h.Version)
	fmt.Fprintf(&b, "%s\n", color.Gra5(h.Description))
	fmt.Fprintf(&b, "%s\n", color.Gra2(h.URL))

	writeSection(&b, "Usage", page.Synopsis, page.UsageNote)
	if len(page.Commands) > 0 {
		writeSection(&b, "Commands", alignRows(page.Commands), page.CommandsNote)
	}
	options := append(append([]Row{}, page.Options...),
		Row{Form: "-v, --version", Meaning: "Print the " + h.Name + " version"},
		Row{Form: "-h, -?, --help", Meaning: "Show this help"},
	)
	writeSection(&b, "Options", alignRows(options), "")
	if len(page.Examples) > 0 {
		writeSection(&b, "Examples", page.Examples, "")
	}
	io.WriteString(w, b.String())
}

// NormalizeArgs rewrites -? to -h and moves a leading help flag after the command it names.
func NormalizeArgs(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		if arg == "-?" {
			arg = "-h"
		}
		out[i] = arg
	}
	if len(out) >= 2 && (out[0] == "-h" || out[0] == "--help") {
		out = append(out[1:], out[0])
	}
	return out
}

// Install routes help for root and every descendant through Render, adds the help command,
// and accepts -v/--version on every command.
func Install(root *cobra.Command, header Header) {
	root.PersistentFlags().BoolP("version", "v", false, "Print the "+header.Name+" version")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		Render(cmd.OutOrStdout(), FromCommand(cmd, header))
	})
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [COMMAND]",
		Short: "Show help for a command",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			target, rest, err := root.Find(args)
			if err != nil || len(rest) > 0 {
				return fmt.Errorf("unknown command %q: run '%s -h' for the command list",
					strings.Join(args, " "), root.Name())
			}
			return target.Help()
		},
	})
	previous := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if v, _ := cmd.Flags().GetBool("version"); v {
			fmt.Fprintf(cmd.OutOrStdout(), "%s v%s\n", header.Name, header.Version)
			return ErrVersionPrinted
		}
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
}

// FromCommand builds the help page for cmd from its cobra metadata.
func FromCommand(cmd *cobra.Command, header Header) Page {
	page := Page{Header: header}
	path := cmd.CommandPath()

	if cmd.HasAvailableSubCommands() {
		page.Synopsis = append(page.Synopsis, path+" COMMAND [options]")
	} else {
		form := path + " [options]"
		if args := useArgs(cmd.Use); args != "" {
			form += " " + args
		}
		page.Synopsis = append(page.Synopsis, form)
	}
	for _, extra := range splitLines(cmd.Annotations[SynopsisKey]) {
		page.Synopsis = append(page.Synopsis, path+" "+extra)
	}
	page.UsageNote = cmd.Long
	if page.UsageNote == "" && cmd.HasParent() {
		page.UsageNote = cmd.Short
	}

	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Deprecated != "" {
			continue
		}
		page.Commands = append(page.Commands, Row{Form: commandForm(sub), Meaning: trimPeriod(sub.Short)})
	}
	if len(page.Commands) > 0 {
		page.CommandsNote = cmd.Annotations[NoteKey]
		if page.CommandsNote == "" {
			page.CommandsNote = fmt.Sprintf("Run '%s COMMAND -h' for command-specific options.", path)
		}
	}

	cmd.Flags().SortFlags = false
	flags := cmd.LocalFlags()
	flags.SortFlags = false
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "version" || f.Name == "help" {
			return
		}
		page.Options = append(page.Options, flagRow(f))
	})

	page.Examples = splitLines(cmd.Example)
	return page
}

func writeSection(b *strings.Builder, title string, body []string, note string) {
	b.WriteString("\n")
	b.WriteString(color.Whi5B(title))
	b.WriteString("\n")
	for _, line := range body {
		b.WriteString("  " + line + "\n")
	}
	if note == "" {
		return
	}
	b.WriteString("\n")
	for _, line := range splitLines(note) {
		for _, wrapped := range wrap(line, noteWidth) {
			b.WriteString("  " + wrapped + "\n")
		}
	}
}

// wrap splits text into lines of at most width runes, breaking only at spaces.
func wrap(text string, width int) []string {
	var lines []string
	var current string
	for word := range strings.FieldsSeq(text) {
		switch {
		case current == "":
			current = word
		case len(current)+1+len(word) > width:
			lines = append(lines, current)
			current = word
		default:
			current += " " + word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func alignRows(rows []Row) []string {
	width := 0
	for _, row := range rows {
		width = max(width, len(row.Form))
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Meaning == "" {
			lines = append(lines, row.Form)
			continue
		}
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, row.Form, row.Meaning))
	}
	return lines
}

func commandForm(cmd *cobra.Command) string {
	form := cmd.Name()
	if len(cmd.Aliases) > 0 {
		form += ", " + strings.Join(cmd.Aliases, ", ")
	}
	if args := useArgs(cmd.Use); args != "" {
		form += " " + args
	}
	return form
}

func flagRow(f *pflag.Flag) Row {
	name, meaning := pflag.UnquoteUsage(f)
	form := "    --" + f.Name
	if f.Shorthand != "" {
		form = "-" + f.Shorthand + ", --" + f.Name
	}
	if f.Value.Type() != "bool" && name != "" {
		form += " " + strings.ToUpper(name)
	}
	return Row{Form: form, Meaning: trimPeriod(meaning)}
}

// useArgs returns the positional part of a cobra Use line as uppercase placeholders.
func useArgs(use string) string {
	fields := strings.Fields(use)
	if len(fields) < 2 {
		return ""
	}
	args := make([]string, 0, len(fields)-1)
	for _, field := range fields[1:] {
		if field == "[flags]" || field == "[options]" {
			continue
		}
		field = strings.NewReplacer("<", "", ">", "").Replace(field)
		args = append(args, strings.ToUpper(field))
	}
	return strings.Join(args, " ")
}

func splitLines(s string) []string {
	s = strings.Trim(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func trimPeriod(s string) string {
	return strings.TrimSuffix(strings.TrimSpace(s), ".")
}
