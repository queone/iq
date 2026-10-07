package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iq/internal/config"
	"iq/internal/sidecar"
)

func inferState(model string, port int) *sidecar.State {
	return &sidecar.State{Tier: "infer", Model: model, Port: port, PID: os.Getpid()}
}

func TestPiProviderIncludesConfiguredLimits(t *testing.T) {
	cfg := &config.Config{Model: "org/model", ContextWindow: 32768, MaxTokens: 4096}
	entry := piProvider(inferState("org/model", 27001), cfg)

	if got := entry["baseUrl"]; got != "http://127.0.0.1:27001/v1" {
		t.Errorf("baseUrl = %v", got)
	}
	if got := entry["api"]; got != "openai-completions" {
		t.Errorf("api = %v", got)
	}
	if got := entry["apiKey"]; got != "iq" {
		t.Errorf("apiKey = %v", got)
	}
	models := entry["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %v, want one entry", models)
	}
	model := models[0].(map[string]any)
	if model["id"] != "default_model" || model["name"] != "org/model" {
		t.Errorf("model id/name = %v/%v", model["id"], model["name"])
	}
	if model["contextWindow"] != 32768 || model["maxTokens"] != 4096 {
		t.Errorf("contextWindow/maxTokens = %v/%v", model["contextWindow"], model["maxTokens"])
	}
}

func TestPiProviderOmitsUnsetLimits(t *testing.T) {
	cfg := &config.Config{Model: "org/model"}
	model := piProvider(inferState("org/model", 27001), cfg)["models"].([]any)[0].(map[string]any)
	if len(model) != 2 {
		t.Fatalf("model = %v, want only id and name", model)
	}
}

func TestPiProviderIgnoresLimitsOfAnotherModel(t *testing.T) {
	cfg := &config.Config{Model: "org/configured", ContextWindow: 32768, MaxTokens: 2048}
	model := piProvider(inferState("org/running", 27003), cfg)["models"].([]any)[0].(map[string]any)
	if len(model) != 2 {
		t.Errorf("limits of a different configured model must not leak: %v", model)
	}
}

func TestSelectInferStateRequiresExactlyOne(t *testing.T) {
	embed := &sidecar.State{Tier: "embed", Model: "org/embed", Port: 27000}

	if _, err := selectInferState([]*sidecar.State{embed}); err == nil || !strings.Contains(err.Error(), "iq start") {
		t.Errorf("no sidecar: err = %v, want the iq start hint", err)
	}
	one := inferState("org/a", 27001)
	if got, err := selectInferState([]*sidecar.State{embed, one}); err != nil || got != one {
		t.Errorf("one sidecar: got %v, err %v", got, err)
	}
	two := []*sidecar.State{embed, one, inferState("org/b", 27002)}
	if _, err := selectInferState(two); err == nil || !strings.Contains(err.Error(), "2 inference sidecars are running; stop all but one") {
		t.Errorf("two sidecars: err = %v", err)
	}
}

const existingPiConfig = `{
  "defaultModel": "ollama/qwen2.5-coder:7b",
  "providers": {
    "ollama": {"baseUrl": "http://localhost:11434/v1", "api": "openai-completions", "apiKey": "ollama",
      "models": [{"id": "qwen2.5-coder:7b"}, {"id": "llama3.2"}]},
    "iq": {"baseUrl": "http://127.0.0.1:1/v1", "models": [{"id": "stale"}]}
  }
}
`

func compactJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("compact: %v", err)
	}
	return buf.String()
}

func readProviders(t *testing.T, path string) (map[string]json.RawMessage, map[string]json.RawMessage) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	providers := map[string]json.RawMessage{}
	if err := json.Unmarshal(root["providers"], &providers); err != nil {
		t.Fatalf("parse providers: %v", err)
	}
	return root, providers
}

func TestMergePiProviderKeepsOtherKeysAndReplacesIQ(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(path, []byte(existingPiConfig), 0644); err != nil {
		t.Fatal(err)
	}
	beforeRoot, beforeProviders := readProviders(t, path)

	cfg := &config.Config{}
	if err := mergePiProvider(path, piProvider(inferState("org/model", 27001), cfg)); err != nil {
		t.Fatalf("mergePiProvider: %v", err)
	}
	afterRoot, afterProviders := readProviders(t, path)

	if compactJSON(t, beforeProviders["ollama"]) != compactJSON(t, afterProviders["ollama"]) {
		t.Errorf("ollama provider changed:\n%s\n%s", beforeProviders["ollama"], afterProviders["ollama"])
	}
	if compactJSON(t, beforeRoot["defaultModel"]) != compactJSON(t, afterRoot["defaultModel"]) {
		t.Errorf("defaultModel changed")
	}
	var iq map[string]any
	if err := json.Unmarshal(afterProviders["iq"], &iq); err != nil {
		t.Fatal(err)
	}
	if iq["baseUrl"] != "http://127.0.0.1:27001/v1" {
		t.Errorf("iq provider not replaced: %v", iq)
	}
	if len(afterProviders) != 2 {
		t.Errorf("providers = %d keys, want ollama and iq only", len(afterProviders))
	}
}

func TestMergePiProviderCreatesMissingFileAndDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "agent", "models.json")
	if err := mergePiProvider(path, piProvider(inferState("org/model", 27001), &config.Config{})); err != nil {
		t.Fatalf("mergePiProvider: %v", err)
	}
	_, providers := readProviders(t, path)
	if _, ok := providers["iq"]; !ok || len(providers) != 1 {
		t.Errorf("providers = %v, want only iq", providers)
	}
}

func TestRunPiPrintsEntryByDefault(t *testing.T) {
	var out bytes.Buffer
	states := []*sidecar.State{inferState("org/model", 27001)}
	if err := runPi(states, &config.Config{}, false, false, "", &out); err != nil {
		t.Fatalf("runPi: %v", err)
	}
	var doc map[string]map[string]map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if doc["providers"]["iq"]["baseUrl"] != "http://127.0.0.1:27001/v1" {
		t.Errorf("printed entry = %v", doc)
	}
}

func TestRunPiWithTwoSidecarsLeavesFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(path, []byte(existingPiConfig), 0644); err != nil {
		t.Fatal(err)
	}
	states := []*sidecar.State{inferState("org/a", 27001), inferState("org/b", 27002)}
	var out bytes.Buffer
	err := runPi(states, &config.Config{}, true, false, path, &out)
	if err == nil || !strings.Contains(err.Error(), "stop all but one") {
		t.Fatalf("err = %v, want the stop-all-but-one message", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != existingPiConfig {
		t.Errorf("file was modified despite the error")
	}
}

func TestRunPiWithNoSidecarHintsStart(t *testing.T) {
	var out bytes.Buffer
	err := runPi(nil, &config.Config{}, true, false, filepath.Join(t.TempDir(), "models.json"), &out)
	if err == nil || !strings.Contains(err.Error(), "iq start") {
		t.Fatalf("err = %v, want the iq start hint", err)
	}
}

func TestMergePiProviderRejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	bad := "{not json"
	if err := os.WriteFile(path, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	err := mergePiProvider(path, piProvider(inferState("org/model", 27001), &config.Config{}))
	if err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Fatalf("err = %v, want a cannot-parse error", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != bad {
		t.Errorf("malformed file was modified")
	}
}

// ── -d / --default ────────────────────────────────────────────────────────────

func TestSetPiDefaultKeepsOtherKeysAndCreatesFile(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "agent", "settings.json")
	if err := setPiDefault(settings); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := os.WriteFile(settings, []byte(`{"lastChangelogVersion":"1.0.4","defaultModel":"old/x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := setPiDefault(settings); err != nil {
		t.Fatalf("update: %v", err)
	}
	var got map[string]any
	data, _ := os.ReadFile(settings)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["defaultProvider"] != "iq" || got["defaultModel"] != "default_model" || got["lastChangelogVersion"] != "1.0.4" {
		t.Errorf("settings = %v", got)
	}
}

func TestSetPiDefaultRejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{nope"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := setPiDefault(path); err == nil || !strings.Contains(err.Error(), "cannot parse") {
		t.Fatalf("err = %v, want a cannot-parse error", err)
	}
}

func TestRunPiDefaultImpliesWriteAndTargetsSiblingSettings(t *testing.T) {
	dir := t.TempDir()
	models := filepath.Join(dir, "models.json")
	var out bytes.Buffer
	states := []*sidecar.State{inferState("org/model", 27001)}
	if err := runPi(states, &config.Config{}, false, true, models, &out); err != nil {
		t.Fatalf("runPi: %v", err)
	}
	if _, providers := readProviders(t, models); len(providers) != 1 {
		t.Errorf("-d must imply -w; providers = %v", providers)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not written beside models.json: %v", err)
	}
	if !strings.Contains(string(data), `"defaultProvider": "iq"`) || !strings.Contains(out.String(), "startup model") {
		t.Errorf("settings = %s\noutput = %s", data, out.String())
	}
}
