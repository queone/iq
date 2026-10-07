package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileReturnsDefaultsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := LoadAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != ConfigVersion || cfg.Model != "" {
		t.Errorf("defaults = %+v", cfg)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Load must not create the file")
	}
}

const v2Config = `version: 2
repetition_penalty: 1.3
max_tokens: 8192
models:
    - id: mlx-community/Qwen3.5-4B-OptiQ-4bit
      context_window: 32768
      max_tokens: 4096
      temperature: 0.3
      chat_template_args: {enable_thinking: false}
    - id: mlx-community/Llama-3.2-3B-Instruct-4bit
embed_model: mlx-community/bge-small-en-v1.5-bf16
kb_min_score: 0.72
tool_paths: [/tmp]
`

func TestV2MigratesToV3InMemory(t *testing.T) {
	path := writeConfig(t, v2Config)
	cfg, err := LoadAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "mlx-community/Qwen3.5-4B-OptiQ-4bit" || cfg.ContextWindow != 32768 || cfg.MaxTokens != 4096 {
		t.Errorf("migrated = %+v", cfg)
	}
	if got := cfg.ChatTemplateArgsJSON(cfg.Model); got != `{"enable_thinking":false}` {
		t.Errorf("ChatTemplateArgsJSON = %q", got)
	}
	for _, want := range []string{"kept model mlx-community/Qwen3.5-4B-OptiQ-4bit", "dropped 1 additional pool model", "embed_model", "kb_min_score", "tool_paths", "repetition_penalty", "models[0].temperature"} {
		if !strings.Contains(cfg.MigrationNotice, want) {
			t.Errorf("notice lacks %q: %s", want, cfg.MigrationNotice)
		}
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "version: 2") {
		t.Errorf("Load must not rewrite the file")
	}
	if err := SaveAt(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "version: 3") || strings.Contains(string(data), "embed_model") || strings.Contains(string(data), "MigrationNotice") {
		t.Errorf("saved file:\n%s", data)
	}
}

func TestV3RoundTripKeepsTemplateArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{Model: "org/model", ContextWindow: 32768, ChatTemplateArgs: map[string]any{"zeta": 1, "enable_thinking": false}}
	if err := SaveAt(path, cfg); err != nil {
		t.Fatal(err)
	}
	for round := 1; round <= 2; round++ {
		loaded, err := LoadAt(path)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if loaded.Version != 3 || loaded.Model != "org/model" || loaded.MigrationNotice != "" {
			t.Errorf("round %d: %+v", round, loaded)
		}
		if got := loaded.ChatTemplateArgsJSON("org/model"); got != `{"enable_thinking":false,"zeta":1}` {
			t.Errorf("round %d: JSON = %q", round, got)
		}
		if got := loaded.ChatTemplateArgsJSON("org/other"); got != "" {
			t.Errorf("round %d: other model should get no args, got %q", round, got)
		}
		if err := SaveAt(path, loaded); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFutureAndAncientVersionsError(t *testing.T) {
	if _, err := LoadAt(writeConfig(t, "version: 4\nmodel: x\n")); err == nil || !strings.Contains(err.Error(), "upgrade iq") {
		t.Errorf("v4: err = %v", err)
	}
	if _, err := LoadAt(writeConfig(t, "version: 1\ntiers: {}\n")); err == nil || !strings.Contains(err.Error(), "iq pick -w") {
		t.Errorf("v1: err = %v", err)
	}
	if _, err := LoadAt(writeConfig(t, "model: [unclosed\n")); err == nil {
		t.Error("invalid YAML should error")
	}
}
