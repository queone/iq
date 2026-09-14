package usage

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var testHeader = Header{Name: "tool", Version: "1.2.3", Description: "Do useful things", URL: "example.test/tool"}

func TestRenderLayout(t *testing.T) {
	var b strings.Builder
	Render(&b, Page{
		Header:       testHeader,
		Synopsis:     []string{"tool COMMAND [options]", "tool [options] MESSAGE"},
		UsageNote:    "Prose closing the Usage section.",
		Commands:     []Row{{Form: "run, r NAME", Meaning: "Run NAME"}, {Form: "ls", Meaning: "List"}},
		CommandsNote: "Run 'tool COMMAND -h' for command-specific options.",
		Options:      []Row{{Form: "-n, --name NAME", Meaning: "Use NAME"}},
		Examples:     []string{"$ tool run x"},
	})
	want := strings.Join([]string{
		"tool v1.2.3",
		"Do useful things",
		"example.test/tool",
		"",
		"Usage",
		"  tool COMMAND [options]",
		"  tool [options] MESSAGE",
		"",
		"  Prose closing the Usage section.",
		"",
		"Commands",
		"  run, r NAME  Run NAME",
		"  ls           List",
		"",
		"  Run 'tool COMMAND -h' for command-specific options.",
		"",
		"Options",
		"  -n, --name NAME  Use NAME",
		"  -v, --version    Print the tool version",
		"  -h, -?, --help   Show this help",
		"",
		"Examples",
		"  $ tool run x",
		"",
	}, "\n")
	if got := b.String(); got != want {
		t.Fatalf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderOmitsEmptySections(t *testing.T) {
	var b strings.Builder
	Render(&b, Page{Header: testHeader, Synopsis: []string{"tool [options]"}})
	got := b.String()
	if strings.Contains(got, "Commands") || strings.Contains(got, "Examples") {
		t.Fatalf("Render() printed an empty section:\n%s", got)
	}
	if !strings.HasSuffix(got, "Options\n  -v, --version   Print the tool version\n  -h, -?, --help  Show this help\n") {
		t.Fatalf("Render() ended with %q", got)
	}
}

func TestNormalizeArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "question mark", in: []string{"-?"}, want: []string{"-h"}},
		{name: "nested question mark", in: []string{"cue", "-?"}, want: []string{"cue", "-h"}},
		{name: "leading help moves last", in: []string{"-h", "cue", "list"}, want: []string{"cue", "list", "-h"}},
		{name: "leading long help", in: []string{"--help", "start"}, want: []string{"start", "--help"}},
		{name: "message untouched", in: []string{"what -? now"}, want: []string{"what -? now"}},
		{name: "empty", in: nil, want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NormalizeArgs(test.in)
			if strings.Join(got, "\x00") != strings.Join(test.want, "\x00") {
				t.Fatalf("NormalizeArgs(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func newTestTree() *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:  "tool",
		Args: cobra.ArbitraryArgs,
		Annotations: map[string]string{
			SynopsisKey: "[options] MESSAGE",
			NoteKey:     "Custom note.",
		},
		Example: "$ tool hello",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.CompletionOptions.DisableDefaultCmd = true
	root.Flags().StringP("cue", "r", "", "Skip classification, use cue `NAME`")
	root.Flags().Bool("no-cache", false, "Disable the cache.")
	root.Flags().Int("top", 0, "Return `N` rows")
	sub := &cobra.Command{
		Use:     "start [model]",
		Aliases: []string{"st"},
		Short:   "Start sidecars; writes run state.",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	hidden := &cobra.Command{Use: "legacy", Hidden: true, Run: func(*cobra.Command, []string) {}}
	parent := &cobra.Command{Use: "cue", Short: "Manage cues"}
	parent.AddCommand(&cobra.Command{Use: "show <name>", Short: "Show a cue", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(sub, hidden, parent)
	Install(root, testHeader)
	return root
}

func TestFromCommandRoot(t *testing.T) {
	root := newTestTree()
	page := FromCommand(root, testHeader)
	if got := strings.Join(page.Synopsis, "|"); got != "tool COMMAND [options]|tool [options] MESSAGE" {
		t.Errorf("Synopsis = %q", got)
	}
	if page.UsageNote != "" {
		t.Errorf("UsageNote = %q, want empty on root", page.UsageNote)
	}
	wantCommands := []Row{{Form: "start, st [MODEL]", Meaning: "Start sidecars; writes run state"}, {Form: "cue", Meaning: "Manage cues"}}
	if len(page.Commands) != len(wantCommands) {
		t.Fatalf("Commands = %+v, want %+v", page.Commands, wantCommands)
	}
	for i, want := range wantCommands {
		if page.Commands[i] != want {
			t.Errorf("Commands[%d] = %+v, want %+v", i, page.Commands[i], want)
		}
	}
	if page.CommandsNote != "Custom note." {
		t.Errorf("CommandsNote = %q", page.CommandsNote)
	}
	wantOptions := []Row{
		{Form: "-r, --cue NAME", Meaning: "Skip classification, use cue NAME"},
		{Form: "    --no-cache", Meaning: "Disable the cache"},
		{Form: "    --top N", Meaning: "Return N rows"},
	}
	if len(page.Options) != len(wantOptions) {
		t.Fatalf("Options = %+v, want %+v", page.Options, wantOptions)
	}
	for i, want := range wantOptions {
		if page.Options[i] != want {
			t.Errorf("Options[%d] = %+v, want %+v", i, page.Options[i], want)
		}
	}
	if got := strings.Join(page.Examples, "|"); got != "$ tool hello" {
		t.Errorf("Examples = %q", got)
	}
}

func TestFromCommandSubcommand(t *testing.T) {
	root := newTestTree()
	sub, _, err := root.Find([]string{"start"})
	if err != nil {
		t.Fatal(err)
	}
	page := FromCommand(sub, testHeader)
	if got := strings.Join(page.Synopsis, "|"); got != "tool start [options] [MODEL]" {
		t.Errorf("Synopsis = %q", got)
	}
	if page.UsageNote != "Start sidecars; writes run state." {
		t.Errorf("UsageNote = %q", page.UsageNote)
	}
	if len(page.Commands) != 0 || len(page.Options) != 0 {
		t.Errorf("subcommand page carried Commands %+v or Options %+v", page.Commands, page.Options)
	}
	parent, _, _ := root.Find([]string{"cue"})
	page = FromCommand(parent, testHeader)
	if got := strings.Join(page.Synopsis, "|"); got != "tool cue COMMAND [options]" {
		t.Errorf("parent Synopsis = %q", got)
	}
	if page.CommandsNote != "Run 'tool cue COMMAND -h' for command-specific options." {
		t.Errorf("parent CommandsNote = %q", page.CommandsNote)
	}
}

func execute(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(NormalizeArgs(args))
	err := root.Execute()
	return out.String(), err
}

func TestInstallRoutesHelpAndVersion(t *testing.T) {
	want, err := execute(t, newTestTree(), "-h")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(want, "tool v1.2.3\nDo useful things\nexample.test/tool\n\nUsage\n") {
		t.Fatalf("root help =\n%s", want)
	}
	for _, args := range [][]string{{"-?"}, {"--help"}, {"help"}} {
		got, err := execute(t, newTestTree(), args...)
		if err != nil || got != want {
			t.Errorf("%q: err=%v, output differs from -h:\n%s", args, err, got)
		}
	}

	got, err := execute(t, newTestTree(), "help", "cue", "show")
	if err != nil || !strings.Contains(got, "\nUsage\n  tool cue show [options] NAME\n") {
		t.Errorf("help cue show: err=%v output=\n%s", err, got)
	}
	if _, err := execute(t, newTestTree(), "help", "nope"); err == nil {
		t.Errorf("help nope: want an unknown-command error")
	}

	for _, args := range [][]string{{"-v"}, {"--version"}, {"start", "-v"}, {"cue", "show", "x", "--version"}} {
		got, err := execute(t, newTestTree(), args...)
		if err != ErrVersionPrinted || got != "tool v1.2.3\n" {
			t.Errorf("%q: err=%v output=%q", args, err, got)
		}
	}
}

func TestRenderWrapsNoteParagraphs(t *testing.T) {
	var b strings.Builder
	long := strings.Repeat("word ", 20) + "end"
	Render(&b, Page{Header: testHeader, Synopsis: []string{"tool"}, UsageNote: long + "\nsecond line"})
	for line := range strings.SplitSeq(b.String(), "\n") {
		if len(line) > noteWidth+2 {
			t.Errorf("line exceeds %d columns: %q", noteWidth+2, line)
		}
	}
	if !strings.Contains(b.String(), "\n  second line\n") {
		t.Errorf("explicit line break lost:\n%s", b.String())
	}
	if got := wrap("", 10); got != nil {
		t.Errorf("wrap(\"\") = %q, want nil", got)
	}
	if got := strings.Join(wrap("aaa bbb ccc", 7), "|"); got != "aaa bbb|ccc" {
		t.Errorf("wrap() = %q", got)
	}
}
