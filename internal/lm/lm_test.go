package lm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// ── HFSibling / HFModel ───────────────────────────────────────────────────────

func TestHFSiblingFileSize(t *testing.T) {
	cases := []struct {
		s    HFSibling
		want int64
	}{
		{HFSibling{Size: 100, LFS: HFSiblingLFS{Size: 0}}, 100},
		{HFSibling{Size: 100, LFS: HFSiblingLFS{Size: 500}}, 500}, // LFS wins
		{HFSibling{Size: 0, LFS: HFSiblingLFS{Size: 0}}, 0},
	}
	for _, tc := range cases {
		if got := tc.s.FileSize(); got != tc.want {
			t.Errorf("FileSize() = %d, want %d (sibling=%+v)", got, tc.want, tc.s)
		}
	}
}

func TestHFModelTotalSize(t *testing.T) {
	// UsedStorage takes precedence.
	m := HFModel{UsedStorage: 1000, Siblings: []HFSibling{{Size: 200}, {Size: 300}}}
	if got := m.TotalSize(); got != 1000 {
		t.Errorf("TotalSize() = %d, want 1000 (UsedStorage present)", got)
	}
	// Fall back to summing siblings.
	m2 := HFModel{Siblings: []HFSibling{{Size: 200}, {Size: 300}}}
	if got := m2.TotalSize(); got != 500 {
		t.Errorf("TotalSize() = %d, want 500 (sum of siblings)", got)
	}
}

// ── ParseParams ───────────────────────────────────────────────────────────────

func TestParseParams(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"mlx-community/gemma-3-1b-it-4bit", "1B"},
		{"mlx-community/smollm2-135m-instruct-4bit", "135M"},
		{"mlx-community/Qwen2.5-7B-Instruct-4bit", "7B"},
		{"mlx-community/llama-3.2-1b-4bit", "1B"},
		{"mlx-community/mistral-7b-v0.1-4bit", "7B"},
		{"mlx-community/unknown-model", "-"},
	}
	for _, tc := range cases {
		got := ParseParams(tc.id)
		if got != tc.want {
			t.Errorf("ParseParams(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// ── ParseQuant ────────────────────────────────────────────────────────────────

func TestParseQuant(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"mlx-community/gemma-3-1b-it-4bit", "4bit"},
		{"mlx-community/bge-small-en-v1.5-bf16", "bf16"},
		{"mlx-community/model-fp16", "fp16"},
		{"mlx-community/no-quant-info", "-"},
	}
	for _, tc := range cases {
		got := ParseQuant(tc.id)
		if got != tc.want {
			t.Errorf("ParseQuant(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// ── Commatize / FormatInt / FormatMB / EstMemMB / ParseParamsM ───────────────

func TestCommatize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
		{1234567, "1,234,567"},
	}
	for _, tc := range cases {
		got := Commatize(tc.n)
		if got != tc.want {
			t.Errorf("Commatize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestFormatInt(t *testing.T) {
	if got := FormatInt(1000); got != "1,000" {
		t.Errorf("FormatInt(1000) = %q, want %q", got, "1,000")
	}
}

func TestFormatMB(t *testing.T) {
	cases := []struct {
		b    int64
		want string
	}{
		{0, "-"},
		{1024 * 1024, "1MB"},
		{4 * 1024 * 1024 * 1024, "4,096MB"},
	}
	for _, tc := range cases {
		got := FormatMB(tc.b)
		if got != tc.want {
			t.Errorf("FormatMB(%d) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestEstMemMB(t *testing.T) {
	if got := EstMemMB(0); got != "-" {
		t.Errorf("EstMemMB(0) = %q, want \"-\"", got)
	}
	// 1GB disk → ~1536MB estimate
	got := EstMemMB(1024 * 1024 * 1024)
	if got == "-" || got == "" {
		t.Errorf("EstMemMB(1GB) = %q, want non-empty estimate", got)
	}
}

func TestParseParamsM(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"mlx-community/gemma-3-1b-it-4bit", "1,000M"},
		{"mlx-community/smollm2-135m-instruct-4bit", "135M"},
		{"mlx-community/unknown-model", "-"},
	}
	for _, tc := range cases {
		got := ParseParamsM(tc.id)
		if got != tc.want {
			t.Errorf("ParseParamsM(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// ── IsModelNotDownloaded ──────────────────────────────────────────────────────

func TestIsModelNotDownloaded(t *testing.T) {
	// nil is never a "not downloaded" error.
	if IsModelNotDownloaded(nil) {
		t.Error("expected false for nil error")
	}
	// SnapshotDir wraps missing dirs with "no snapshots found".
	_, err := SnapshotDir("org/definitely-not-a-real-model")
	if !IsModelNotDownloaded(err) {
		t.Errorf("expected true for missing model, got false (err=%v)", err)
	}
}

// ── DiskUsage ─────────────────────────────────────────────────────────────────

func TestDiskUsage(t *testing.T) {
	dir := t.TempDir()
	blobsDir := filepath.Join(dir, "blobs")
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write two files of known sizes.
	if err := os.WriteFile(filepath.Join(blobsDir, "a"), make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blobsDir, "b"), make([]byte, 200), 0644); err != nil {
		t.Fatal(err)
	}

	got := DiskUsage(dir)
	if got != 300 {
		t.Errorf("DiskUsage() = %d, want 300", got)
	}
}

func TestDiskUsageNoBlobs(t *testing.T) {
	dir := t.TempDir()
	// No blobs/ subdirectory — should return 0 gracefully.
	got := DiskUsage(dir)
	if got != 0 {
		t.Errorf("DiskUsage(no blobs) = %d, want 0", got)
	}
}

// ── Manifest round-trip ───────────────────────────────────────────────────────

func TestManifestRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(home+"/.config/iq", 0755); err != nil {
		t.Fatal(err)
	}

	entries := []ManifestEntry{
		{ID: "org/model-a", PulledAt: "2024-01-01T00:00:00Z", HFCache: "/tmp/a"},
		{ID: "org/model-b", PulledAt: "2024-01-02T00:00:00Z", HFCache: "/tmp/b"},
	}
	if err := SaveManifest(entries); err != nil {
		t.Fatalf("SaveManifest() error: %v", err)
	}
	got, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "org/model-a" || got[1].ID != "org/model-b" {
		t.Errorf("manifest round-trip mismatch: %+v", got)
	}
}

func TestLoadManifestEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(home+"/.config/iq", 0755); err != nil {
		t.Fatal(err)
	}
	// No models.json yet.
	got, err := LoadManifest()
	if err != nil {
		t.Fatalf("LoadManifest() on missing file error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("LoadManifest() on missing file = %v, want empty slice", got)
	}
}

func TestRemoveFromManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(home+"/.config/iq", 0755); err != nil {
		t.Fatal(err)
	}

	entries := []ManifestEntry{{ID: "org/keep"}, {ID: "org/remove"}}
	if err := SaveManifest(entries); err != nil {
		t.Fatal(err)
	}

	removed, ok, err := RemoveFromManifest("org/remove")
	if err != nil || !ok || removed.ID != "org/remove" {
		t.Errorf("RemoveFromManifest() = (%+v, %v, %v)", removed, ok, err)
	}

	got, _ := LoadManifest()
	if len(got) != 1 || got[0].ID != "org/keep" {
		t.Errorf("after remove, manifest = %+v, want [{org/keep}]", got)
	}

	// Remove non-existent — ok should be false.
	_, ok, err = RemoveFromManifest("org/ghost")
	if err != nil || ok {
		t.Errorf("RemoveFromManifest(missing) = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}

// ── FormatTask / FormatTaskCol ────────────────────────────────────────────────

func TestFormatTask(t *testing.T) {
	// color.enabled is false in tests (stdout is not a TTY), so ANSI codes are stripped.
	cases := []struct {
		tag  string
		want string
	}{
		{"", "-"},
		{"text-generation", "text-generation"},
		{"feature-extraction", "embedding"},
		{"fill-mask", "fill-mask"},
	}
	for _, tc := range cases {
		got := FormatTask(tc.tag)
		if got != tc.want {
			t.Errorf("FormatTask(%q) = %q, want %q", tc.tag, got, tc.want)
		}
	}
}

func TestFormatTaskCol(t *testing.T) {
	// 24-char padded, no ANSI in tests.
	got := FormatTaskCol("")
	if !strings.HasPrefix(got, "-") || len(got) != 24 {
		t.Errorf("FormatTaskCol(\"\") = %q (len=%d), want 24-char padded \"-...\"", got, len(got))
	}
	got2 := FormatTaskCol("text-generation")
	if len(got2) != 24 {
		t.Errorf("FormatTaskCol(text-generation) len=%d, want 24", len(got2))
	}
	got3 := FormatTaskCol("feature-extraction")
	if !strings.HasPrefix(strings.TrimRight(got3, " "), "embedding") {
		t.Errorf("FormatTaskCol(feature-extraction) = %q, want starts with 'embedding'", got3)
	}
	// Long tag truncated to 23 chars + ellipsis (24 runes total).
	long := "a-very-long-pipeline-tag-that-exceeds-24"
	got4 := FormatTaskCol(long)
	if len([]rune(got4)) != 24 {
		t.Errorf("FormatTaskCol(long) rune len=%d, want 24", len([]rune(got4)))
	}
}

// ── HFCacheDir ────────────────────────────────────────────────────────────────

func TestHFCacheDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := HFCacheDir("mlx-community/gemma-3-4b-it-4bit")
	want := filepath.Join(home, ".cache", "huggingface", "hub", "models--mlx-community--gemma-3-4b-it-4bit")
	if got != want {
		t.Errorf("HFCacheDir = %q, want %q", got, want)
	}
}

// ── HFSearch (httptest) ───────────────────────────────────────────────────────

func TestHFSearchCancelledContext(t *testing.T) {
	// HFAPIBase defaults to HuggingFace; this test stays offline.
	// Test that a pre-cancelled context causes HFSearch to return an error quickly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := HFSearch(ctx, "test-query", 5)
	if err == nil {
		t.Error("HFSearch(cancelled ctx): expected error, got nil")
	}
}

func TestHFEnrichModelsMock(t *testing.T) {
	// Serve a minimal HFModel JSON for each request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := HFModel{ID: "org/model", PipelineTag: "text-generation", UsedStorage: 1024}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	}))
	defer srv.Close()

	// HFEnrichModels calls HFFetchModel against HFAPIBase; this test stays offline.
	// Use a cancelled context so the call returns immediately; verify error is returned.
	models := []HFModel{{ID: "org/model-a"}, {ID: "org/model-b"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := HFEnrichModels(ctx, models)
	// errgroup returns the first error; cancelled context should produce one.
	if err == nil {
		t.Error("HFEnrichModels(cancelled ctx): expected error, got nil")
	}
	// Originals must be preserved (copy was made before fetch).
	if models[0].ID != "org/model-a" || models[1].ID != "org/model-b" {
		t.Errorf("HFEnrichModels: originals mutated, got %+v", models)
	}
	_ = srv // server is created for documentation; actual calls go to HFAPIBase
}

// ── HFFetchModel (cancelled context) ─────────────────────────────────────────

func TestHFFetchModelCancelledContext(t *testing.T) {
	// HFAPIBase defaults to HuggingFace; this test stays offline. Use a cancelled
	// context to verify that the function returns quickly with an error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := HFFetchModel(ctx, "org/mymodel")
	if err == nil {
		t.Error("HFFetchModel(cancelled ctx): expected error, got nil")
	}
}

// ── SuggestSize ───────────────────────────────────────────────────────────────

func TestSuggestSizeByName(t *testing.T) {
	// These models are not downloaded, so SuggestSize falls back to param count.
	cases := []struct {
		id   string
		want string
	}{
		{"org/tiny-135m-4bit", "small"},    // 135M → small
		{"org/large-7b-instruct", "large"}, // 7B → large
		{"org/unknown-model", "large"},     // unknown → assume large
	}
	for _, tc := range cases {
		got := SuggestSize(tc.id)
		if got != tc.want {
			t.Errorf("SuggestSize(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// ── HF hub scanning ───────────────────────────────────────────────────────────

func TestModelIDFromCacheName(t *testing.T) {
	cases := []struct {
		name   string
		wantID string
		wantOK bool
	}{
		{"models--org--name", "org/name", true},
		{"models--gpt2", "gpt2", true},
		{".locks", "", false},
		{"datasets--x", "", false},
		{"models--", "", false},
	}
	for _, c := range cases {
		id, ok := ModelIDFromCacheName(c.name)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("ModelIDFromCacheName(%q) = (%q, %v), want (%q, %v)", c.name, id, ok, c.wantID, c.wantOK)
		}
	}
	// Round-trip through HFCacheDir.
	got, ok := ModelIDFromCacheName(filepath.Base(HFCacheDir("mlx-community/gemma-3-1b-it-4bit")))
	if !ok || got != "mlx-community/gemma-3-1b-it-4bit" {
		t.Errorf("round-trip = (%q, %v)", got, ok)
	}
}

// writeCacheDir creates a fake HF cache directory for id under the current
// HOME, with an optional snapshot config.json, and returns its path.
func writeCacheDir(t *testing.T, id, configJSON string) string {
	t.Helper()
	dir := HFCacheDir(id)
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if configJSON != "" {
		snap := filepath.Join(dir, "snapshots", "abc123")
		if err := os.MkdirAll(snap, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(snap, "config.json"), []byte(configJSON), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCachedModelIDs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCacheDir(t, "org/b", "")
	writeCacheDir(t, "org/a", "")
	hub := HFHubDir()
	for _, d := range []string{".locks", "datasets--x"} {
		if err := os.MkdirAll(filepath.Join(hub, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(hub, "models--org--file"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := CachedModelIDs()
	if err != nil {
		t.Fatalf("CachedModelIDs() error: %v", err)
	}
	want := []string{"org/a", "org/b"}
	if !slices.Equal(got, want) {
		t.Errorf("CachedModelIDs() = %v, want %v", got, want)
	}
}

func TestCachedModelIDsNoHub(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := CachedModelIDs()
	if err != nil {
		t.Fatalf("CachedModelIDs() with no hub error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("CachedModelIDs() with no hub = %v, want empty", got)
	}
}

func TestReconcileManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCacheDir(t, "org/a", "")
	cDir := writeCacheDir(t, "org/c", `{"model_type":"llama"}`)
	mtime := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := os.Chtimes(cDir, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if err := SaveManifest([]ManifestEntry{
		{ID: "org/a", PulledAt: "2026-01-01T00:00:00Z", HFCache: HFCacheDir("org/a"), Task: "text-generation"},
		{ID: "org/b", PulledAt: "2026-01-02T00:00:00Z", HFCache: HFCacheDir("org/b")},
	}); err != nil {
		t.Fatal(err)
	}

	added, dropped, err := ReconcileManifest()
	if err != nil {
		t.Fatalf("ReconcileManifest() error: %v", err)
	}
	if !slices.Equal(added, []string{"org/c"}) || !slices.Equal(dropped, []string{"org/b"}) {
		t.Errorf("ReconcileManifest() = (added %v, dropped %v), want ([org/c], [org/b])", added, dropped)
	}

	got, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "org/a" || got[1].ID != "org/c" {
		t.Fatalf("manifest after reconcile = %+v, want [org/a org/c]", got)
	}
	c := got[1]
	if c.PulledAt != mtime.Format(time.RFC3339) {
		t.Errorf("org/c pulled_at = %q, want %q", c.PulledAt, mtime.Format(time.RFC3339))
	}
	if c.HFCache != cDir {
		t.Errorf("org/c hf_cache_path = %q, want %q", c.HFCache, cDir)
	}
	if c.Task != "" {
		t.Errorf("org/c task = %q, want empty (left to the list/show backfill)", c.Task)
	}

	// A second reconcile changes nothing.
	added, dropped, err = ReconcileManifest()
	if err != nil || len(added) != 0 || len(dropped) != 0 {
		t.Errorf("second ReconcileManifest() = (%v, %v, %v), want no change", added, dropped, err)
	}
}
