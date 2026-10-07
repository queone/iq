package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"iq/internal/color"
	"iq/internal/config"
	"iq/internal/model"
)

// newConfigCmd returns the `iq config` command with show and validate subcommands.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "config",
		Aliases:      []string{"cfg"},
		Short:        "Show and validate config.yaml",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:          "show",
			Short:        "Show the effective configuration",
			SilenceUsage: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runConfigShow()
			},
		},
		&cobra.Command{
			Use:          "validate",
			Short:        "Validate config.yaml and the configured model",
			SilenceUsage: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runConfigValidate()
			},
		},
	)
	return cmd
}

// runConfigShow prints the effective configuration as YAML. It never writes.
func runConfigShow() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	path, _ := config.Path()
	fmt.Printf("# %s (schema v%d)\n", path, config.ConfigVersion)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

// runConfigValidate loads the file, reports what iq will run, and warns about
// gaps. It never writes.
func runConfigValidate() error {
	path, _ := config.Path()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println(color.Yel5("config.yaml does not exist — run: iq pick -w"))
		return nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Printf("%-20s %s\n", "schema", fmt.Sprintf("v%d", config.ConfigVersion))
	if cfg.Model == "" {
		fmt.Println(color.Yel5("model not set — run: iq pick -w"))
		return nil
	}
	fmt.Printf("%-20s %s\n", "model", cfg.Model)
	if snap, err := model.SnapshotDir(cfg.Model); err == nil {
		fmt.Printf("%-20s %s\n", "snapshot", snap)
	} else {
		fmt.Printf("%-20s %s\n", "snapshot", color.Yel5("not in the Hugging Face cache — iq start downloads it"))
	}
	if cfg.ContextWindow > 0 {
		fmt.Printf("%-20s %d\n", "context_window", cfg.ContextWindow)
	}
	if cfg.MaxTokens > 0 {
		fmt.Printf("%-20s %d\n", "max_tokens", cfg.MaxTokens)
	}
	if args := cfg.ChatTemplateArgsJSON(cfg.Model); args != "" {
		fmt.Printf("%-20s %s\n", "chat_template_args", args)
	}
	fmt.Println("config.yaml is valid.")
	return nil
}
