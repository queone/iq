package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"iq/internal/color"
	"iq/internal/usage"
)

const (
	programName    = "lm"
	programVersion = "0.3.0"
	programURL     = "iq/cmd/lm"
	programSummary = "Local model manager"
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
		Short:   "Print the lm version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s v%s\n", programName, programVersion)
		},
	}
}

// newRootCmd wires every lm command under one root whose help pages share one renderer.
func newRootCmd() *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:          programName,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Example: "$ lm search gemma\n" +
			"$ lm get mlx-community/gemma-3-1b-it-4bit\n" +
			"$ lm list\n" +
			"$ lm show mlx-community/gemma-3-1b-it-4bit\n" +
			"$ lm perf bench --type infer --model mlx-community/gemma-3-1b-it-4bit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SilenceErrors = true

	root.AddCommand(
		newLmSearchCmd(),
		newLmGetCmd(),
		newLmListCmd(),
		newLmShowCmd(),
		newLmRmCmd(),
		newPerfCmd(),
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
