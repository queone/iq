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
	programName    = "kb"
	programVersion = "0.2.0"
	programURL     = "iq/cmd/kb"
	programSummary = "Private knowledge base — ingest, search, ask"
)

// errSilent is returned when the error has already been printed.
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
		Short:   "Print the kb version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s v%s\n", programName, programVersion)
		},
	}
}

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

// newRootCmd wires every kb command under one root whose help pages share one renderer.
func newRootCmd() *cobra.Command {
	cobra.EnableCommandSorting = false
	var rootOpts askOpts

	root := &cobra.Command{
		Use:          programName,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		Annotations:  map[string]string{usage.SynopsisKey: "[options] QUERY"},
		Example: "$ kb ingest ~/projects/notes\n" +
			"$ kb list\n" +
			"$ kb \"how does auth work\"\n" +
			"$ kb ask \"explain the key concepts\"\n" +
			"$ kb start && kb \"what is X?\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()
			rootOpts.query = strings.Join(args, " ")
			return runAsk(ctx, rootOpts)
		},
	}

	root.CompletionOptions.DisableDefaultCmd = true
	root.SilenceErrors = true
	addAskFlags(root, &rootOpts)

	root.AddCommand(
		newAskCmd(),
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
		newStatusCmd(),
		newKbIngestCmd(),
		newKbListCmd(),
		newKbSearchCmd(),
		newKbRmCmd(),
		newKbClearCmd(),
		newConfigCmd(),
		newVersionCmd(),
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

func main() {
	runCLI()
}
