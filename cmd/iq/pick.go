package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"iq/internal/config"
	"iq/internal/model"
)

// pickEntry reads the machine's memory and picks the catalog model that fits.
func pickEntry() (model.Entry, uint64, error) {
	mem, err := model.MemBytes()
	if err != nil {
		return model.Entry{}, 0, err
	}
	entries, err := model.Catalog()
	if err != nil {
		return model.Entry{}, 0, err
	}
	e, err := model.Pick(mem, entries)
	if err != nil {
		return model.Entry{}, mem, err
	}
	return e, mem, nil
}

// applyPick stores a catalog entry as the configured model with its limits.
func applyPick(cfg *config.Config, e model.Entry) {
	cfg.Model = e.ID
	cfg.ContextWindow = e.ContextWindow
	cfg.ChatTemplateArgs = e.ChatTemplateArgs
}

// formatPick renders the pick, its estimate, and the budget it was judged against.
func formatPick(e model.Entry, mem uint64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s %s\n", "memory", model.FormatGB(mem))
	fmt.Fprintf(&b, "%-10s %s (%.0f%% of memory)\n", "budget", model.FormatGB(model.Budget(mem)), model.BudgetFraction*100)
	fmt.Fprintf(&b, "%-10s %s\n", "model", e.ID)
	fmt.Fprintf(&b, "%-10s %.1f GB on disk, about %s in memory\n", "estimate", e.DiskGB, model.FormatGB(model.EstimateBytes(e)))
	if e.ContextWindow > 0 {
		fmt.Fprintf(&b, "%-10s %d\n", "context", e.ContextWindow)
	}
	if len(e.ChatTemplateArgs) > 0 {
		fmt.Fprintf(&b, "%-10s %s\n", "template", (&config.Config{Model: e.ID, ChatTemplateArgs: e.ChatTemplateArgs}).ChatTemplateArgsJSON(e.ID))
	}
	if e.Notes != "" {
		fmt.Fprintf(&b, "%-10s %s\n", "notes", e.Notes)
	}
	return b.String()
}

// newPickCmd returns the `iq pick` command.
func newPickCmd() *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:          "pick",
		Short:        "Pick the largest verified catalog model that fits this machine; -w writes it to config.yaml",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, mem, err := pickEntry()
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), formatPick(e, mem))
			if !write {
				return nil
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			applyPick(cfg, e)
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("saving config.yaml: %w", err)
			}
			path, _ := config.Path()
			fmt.Fprintf(cmd.OutOrStdout(), "wrote model %s to %s\n", e.ID, path)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&write, "write", "w", false, "Write the pick to config.yaml as the configured model")
	return cmd
}
