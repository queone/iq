// Package config reads and writes ~/.config/iq/config.yaml, the single-model
// configuration that tells iq which MLX model to run for pi.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigVersion is the schema version written by Save.
// Version 2 was the flat model pool; version 3 holds one model.
const ConfigVersion = 3

// Config is the schema-3 file: one model plus the limits pi should know about.
type Config struct {
	Version          int            `yaml:"version"`
	Model            string         `yaml:"model,omitempty"`
	ContextWindow    int            `yaml:"context_window,omitempty"`
	MaxTokens        int            `yaml:"max_tokens,omitempty"`
	ChatTemplateArgs map[string]any `yaml:"chat_template_args,omitempty"`

	// MigrationNotice describes a schema migration applied in memory on load.
	// Callers print it once; it is never written to disk.
	MigrationNotice string `yaml:"-"`
}

// ChatTemplateArgsJSON returns chat_template_args as compact JSON with sorted
// keys for mlx_lm.server's --chat-template-args flag. It returns "" when the
// requested model is not the configured one or no arguments are set.
func (c *Config) ChatTemplateArgsJSON(modelID string) string {
	if modelID != "" && modelID != c.Model {
		return ""
	}
	if len(c.ChatTemplateArgs) == 0 {
		return ""
	}
	b, err := json.Marshal(c.ChatTemplateArgs)
	if err != nil {
		return ""
	}
	return string(b)
}

// ── Directory helpers ─────────────────────────────────────────────────────────

// Dir returns ~/.config/iq, creating it if needed.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "iq")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// Path returns the config.yaml path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// ── Load / Save ───────────────────────────────────────────────────────────────

func defaultConfig() *Config {
	return &Config{Version: ConfigVersion}
}

// Load reads the default config.yaml. A missing file yields in-memory defaults
// and writes nothing.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return defaultConfig(), nil
	}
	return LoadAt(path)
}

// LoadAt reads config from an explicit path. A version-2 file is migrated in
// memory and reported through MigrationNotice; nothing is written until Save.
func LoadAt(cfgPath string) (*Config, error) {
	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	var vp struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &vp); err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}
	switch {
	case vp.Version > ConfigVersion:
		return nil, fmt.Errorf("config.yaml uses schema v%d (this build supports up to v%d); upgrade iq",
			vp.Version, ConfigVersion)
	case vp.Version == ConfigVersion:
		cfg := &Config{}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config.yaml: %w", err)
		}
		return cfg, nil
	case vp.Version == 2:
		return migrateV2(data)
	default:
		return nil, fmt.Errorf("config.yaml uses schema v%d; this build migrates only from v2 — recreate it with: iq pick -w",
			vp.Version)
	}
}

// Save writes cfg to the default path with the current schema version.
func Save(cfg *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveAt(path, cfg)
}

// SaveAt writes cfg to an explicit path with the current schema version.
func SaveAt(cfgPath string, cfg *Config) error {
	cfg.Version = ConfigVersion
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

// ── Schema 2 migration ────────────────────────────────────────────────────────

var v2KeptTopLevel = map[string]bool{"version": true, "models": true, "max_tokens": true}
var v2KeptModelKeys = map[string]bool{"id": true, "context_window": true, "max_tokens": true, "chat_template_args": true}

// migrateV2 takes the first pool model, its limits, and its template args, and
// reports every dropped key in MigrationNotice.
func migrateV2(data []byte) (*Config, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}
	cfg := defaultConfig()
	var dropped []string
	for k := range raw {
		if !v2KeptTopLevel[k] {
			dropped = append(dropped, k)
		}
	}
	if mt, ok := raw["max_tokens"].(int); ok {
		cfg.MaxTokens = mt
	}
	models, _ := raw["models"].([]any)
	if len(models) > 0 {
		first, _ := models[0].(map[string]any)
		if id, ok := first["id"].(string); ok {
			cfg.Model = id
		}
		if cw, ok := first["context_window"].(int); ok {
			cfg.ContextWindow = cw
		}
		if mt, ok := first["max_tokens"].(int); ok {
			cfg.MaxTokens = mt
		}
		if args, ok := first["chat_template_args"].(map[string]any); ok && len(args) > 0 {
			cfg.ChatTemplateArgs = args
		}
		for k := range first {
			if !v2KeptModelKeys[k] {
				dropped = append(dropped, "models[0]."+k)
			}
		}
	}
	sort.Strings(dropped)
	var parts []string
	if cfg.Model != "" {
		parts = append(parts, "kept model "+cfg.Model)
	} else {
		parts = append(parts, "no pool model to keep")
	}
	if len(models) > 1 {
		parts = append(parts, fmt.Sprintf("dropped %d additional pool model(s)", len(models)-1))
	}
	if len(dropped) > 0 {
		parts = append(parts, "dropped keys: "+strings.Join(dropped, ", "))
	}
	cfg.MigrationNotice = "config.yaml migrated from schema v2 to v3 in memory: " + strings.Join(parts, "; ") +
		". Run 'iq start' or 'iq pick -w' to save it."
	return cfg, nil
}
