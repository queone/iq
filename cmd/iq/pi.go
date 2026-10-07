package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"iq/internal/config"
	"iq/internal/sidecar"
)

// piProviderKey is the provider name iq writes into pi's models.json.
const piProviderKey = "iq"

// piModelID is the only model id mlx_lm.server accepts for its loaded model.
const piModelID = "default_model"

// defaultPiModelsPath returns pi's models.json path under the user's home.
func defaultPiModelsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi", "agent", "models.json"), nil
}

// piProvider builds pi's provider entry for one running inference sidecar.
// contextWindow and maxTokens are included only when config.yaml sets them for
// the running model.
func piProvider(state *sidecar.State, cfg *config.Config) map[string]any {
	model := map[string]any{"id": piModelID, "name": state.Model}
	if state.Model == cfg.Model {
		if cfg.ContextWindow > 0 {
			model["contextWindow"] = cfg.ContextWindow
		}
		if cfg.MaxTokens > 0 {
			model["maxTokens"] = cfg.MaxTokens
		}
	}
	return map[string]any{
		"baseUrl": fmt.Sprintf("http://127.0.0.1:%d/v1", state.Port),
		"api":     "openai-completions",
		"apiKey":  piProviderKey,
		"models":  []any{model},
	}
}

// selectInferState returns the single running inference sidecar, or an error
// that tells the user how to get to exactly one.
func selectInferState(states []*sidecar.State) (*sidecar.State, error) {
	var infer []*sidecar.State
	for _, s := range states {
		if s.Tier == "infer" {
			infer = append(infer, s)
		}
	}
	switch len(infer) {
	case 0:
		return nil, errors.New("no inference sidecar is running — run: iq start")
	case 1:
		return infer[0], nil
	default:
		return nil, fmt.Errorf("%d inference sidecars are running; stop all but one", len(infer))
	}
}

// mergePiProvider writes the iq provider entry into the pi config file at path,
// creating the file and its directory when absent and keeping every other key.
func mergePiProvider(path string, entry map[string]any) error {
	root := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &root); err != nil {
				return fmt.Errorf("cannot parse %s: %w", path, err)
			}
		}
	case errors.Is(err, os.ErrNotExist):
		// First write — start from an empty document.
	default:
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	providers := map[string]json.RawMessage{}
	if raw, ok := root["providers"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &providers); err != nil {
			return fmt.Errorf("cannot parse providers in %s: %w", path, err)
		}
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	providers[piProviderKey] = entryRaw
	provRaw, err := json.Marshal(providers)
	if err != nil {
		return err
	}
	root["providers"] = provRaw
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0644); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}

// piSettingsPath returns pi's settings.json, which lives beside its models.json.
func piSettingsPath(modelsPath string) string {
	return filepath.Join(filepath.Dir(modelsPath), "settings.json")
}

// setPiDefault makes iq/default_model pi's startup model by writing
// defaultProvider and defaultModel into the settings file at path, creating
// the file when absent and keeping every other key.
func setPiDefault(path string) error {
	root := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &root); err != nil {
				return fmt.Errorf("cannot parse %s: %w", path, err)
			}
		}
	case errors.Is(err, os.ErrNotExist):
		// First write — start from an empty document.
	default:
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	root["defaultProvider"], _ = json.Marshal(piProviderKey)
	root["defaultModel"], _ = json.Marshal(piModelID)
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0644); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}

// runPi prints the provider entry for the running sidecar, or merges it into
// the pi config file when write is set. setDefault also makes iq/default_model
// pi's startup model and implies write. An empty path means pi's default file.
func runPi(states []*sidecar.State, cfg *config.Config, write, setDefault bool, path string, out io.Writer) error {
	state, err := selectInferState(states)
	if err != nil {
		return err
	}
	entry := piProvider(state, cfg)
	if setDefault {
		write = true
	}
	if !write {
		doc := map[string]any{"providers": map[string]any{piProviderKey: entry}}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(b))
		return nil
	}
	if path == "" {
		if path, err = defaultPiModelsPath(); err != nil {
			return err
		}
	}
	if err := mergePiProvider(path, entry); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote provider %q for %s (%s) to %s\n", piProviderKey, state.Model, sidecar.Endpoint(state.Port), path)
	if setDefault {
		settings := piSettingsPath(path)
		if err := setPiDefault(settings); err != nil {
			return err
		}
		fmt.Fprintf(out, "set pi's startup model to %s/%s in %s\n", piProviderKey, piModelID, settings)
	}
	return nil
}

// newPiCmd returns the `iq pi` command.
func newPiCmd() *cobra.Command {
	var write, setDefault bool
	var file string
	cmd := &cobra.Command{
		Use:          "pi",
		Short:        "Print pi's provider entry for the running sidecar; -w writes ~/.pi/agent/models.json, -d also makes it pi's default",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			states, err := sidecar.AllLiveStates()
			if err != nil {
				return fmt.Errorf("cannot read sidecar state: %w", err)
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return runPi(states, cfg, write, setDefault, file, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&write, "write", "w", false, "Merge the entry into pi's models.json instead of printing it")
	cmd.Flags().BoolVarP(&setDefault, "default", "d", false, "Also set pi's startup model to iq/default_model in settings.json (implies -w)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Target file for -w (default ~/.pi/agent/models.json)")
	return cmd
}
