package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
	"iq/internal/color"
	"iq/internal/usage"
)

const (
	programName    = "iq"
	programVersion = "0.19.3"
	programURL     = "iq"
	programSummary = "Work with IQ from the command line"
)

// errSilent is returned when the error has already been printed.
// Using a named type ensures errors.Is comparisons work correctly.
type silentErr struct{}

func (silentErr) Error() string { return "" }

var errSilent error = silentErr{}

// argsUsage wraps a cobra arg validator to print yellow error + help on failure.
func argsUsage(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n\n", color.Yel5(err.Error()))
			cmd.Help()
			return errSilent
		}
		return nil
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"ver"},
		Short:   "Print the iq version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s v%s\n", programName, programVersion)
		},
	}
}

// newStatusCmd returns a top-level `iq status` / `iq st` command.
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "status",
		Aliases:      []string{"st"},
		Short:        "Show running sidecars and memory use",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printStatus()
		},
	}
}

// newRootCmd wires every iq command under one root whose help pages share one renderer.
func newRootCmd() *cobra.Command {
	cobra.EnableCommandSorting = false
	var rootOpts promptOpts

	root := &cobra.Command{
		Use:          programName,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		Annotations: map[string]string{
			usage.SynopsisKey: "[options] MESSAGE",
			usage.NoteKey: "Run 'iq COMMAND -h' for command-specific options.\n" +
				"Model downloads and benchmarks live in the separate lm utility.",
		},
		Example: "$ iq \"explain transformers\"\n" +
			"$ iq -d \"explain transformers\"\n" +
			"$ iq ask\n" +
			"$ iq start\n" +
			"$ iq st\n" +
			"$ iq doc",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()
			input := strings.Join(args, " ")
			var sess *session
			if rootOpts.sessionID != "" {
				var err error
				sess, err = loadSession(rootOpts.sessionID)
				if err != nil {
					return err
				}
			}
			_, err := executePrompt(ctx, input, rootOpts, sess)
			return err
		},
	}

	root.CompletionOptions.DisableDefaultCmd = true
	root.SilenceErrors = true
	addPromptFlags(root, &rootOpts)

	root.AddCommand(
		newPromptCmd(),
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
		newStatusCmd(),
		newDocCmd(),
		newPoolCmd(),
		newEmbedCmd(),
		newCueCmd(),
		newKbCmd(),
		newConfigCmd(),
		newProbeCmd(),
		newVersionCmd(),
		newSvcCmd(), // hidden backward-compat alias
	)

	usage.Install(root, usage.Header{
		Name:        programName,
		Version:     programVersion,
		Description: programSummary,
		URL:         programURL,
	})
	return root
}

func runCLI() {
	root := newRootCmd()
	root.SetArgs(usage.NormalizeArgs(os.Args[1:]))
	if err := root.Execute(); err != nil {
		if errors.Is(err, usage.ErrVersionPrinted) {
			return
		}
		if !errors.Is(err, errSilent) {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
		os.Exit(1)
	}
}

// shellescape single-quotes a string for safe shell interpolation.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func main() {
	runCLI()
}
