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
	programName    = "iq"
	programVersion = "0.20.0"
	programURL     = "iq"
	programSummary = "Run one local MLX model for pi"
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

// newRootCmd wires every iq command under one root whose help pages share one renderer.
func newRootCmd() *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:          programName,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Annotations: map[string]string{
			usage.SynopsisKey: "COMMAND [options]",
			usage.NoteKey: "Run 'iq COMMAND -h' for command-specific options.\n" +
				"iq runs one MLX model as an mlx_lm.server sidecar and hands it to pi.",
		},
		Example: "$ iq doc\n" +
			"$ iq pick -w\n" +
			"$ iq start\n" +
			"$ iq pi -w\n" +
			"$ pi --model iq/default_model\n" +
			"$ iq stop",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SilenceErrors = true
	root.AddCommand(
		newDocCmd(),
		newPickCmd(),
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
		newStatusCmd(),
		newPiCmd(),
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
