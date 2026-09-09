package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iq/internal/lm"
)

// pointAPIAtTestServer redirects Hugging Face API calls to a local server that
// reports pipelineTag for every model, or 404 when pipelineTag is empty, so
// tests never reach the network.
func pointAPIAtTestServer(t *testing.T, pipelineTag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pipelineTag == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"pipeline_tag":%q}`, strings.TrimPrefix(r.URL.Path, "/"), pipelineTag)
	}))
	t.Cleanup(srv.Close)
	old := lm.HFAPIBase
	lm.HFAPIBase = srv.URL
	t.Cleanup(func() { lm.HFAPIBase = old })
}

// writeCachedModel creates a fake HF cache directory for id under the current
// HOME with a snapshot config.json declaring a llama model, and returns its path.
func writeCachedModel(t *testing.T, id string) string {
	t.Helper()
	dir := lm.HFCacheDir(id)
	snap := filepath.Join(dir, "snapshots", "abc123")
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(snap, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "config.json"), []byte(`{"model_type":"llama"}`), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// registeredEntry returns a manifest entry that already carries a task tag.
func registeredEntry(id string) lm.ManifestEntry {
	return lm.ManifestEntry{ID: id, PulledAt: "2026-01-01T00:00:00Z", HFCache: lm.HFCacheDir(id), Task: "text-generation"}
}

func TestListRegistersCachedModels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pointAPIAtTestServer(t, "feature-extraction")
	writeCachedModel(t, "org/reg")
	writeCachedModel(t, "org/new")
	if err := lm.SaveManifest([]lm.ManifestEntry{registeredEntry("org/reg")}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := runCLIOutput(t, "ls")
	for _, id := range []string{"org/reg", "org/new"} {
		if !strings.Contains(stdout, id) {
			t.Errorf("lm ls stdout missing %q:\n%s", id, stdout)
		}
	}
	if !strings.Contains(stderr, "note: registered 1 cached model\n") {
		t.Errorf("lm ls stderr = %q, want registered note", stderr)
	}
	entries, err := lm.LoadManifest()
	if err != nil || len(entries) != 2 {
		t.Fatalf("manifest after lm ls = %+v (err %v), want 2 entries", entries, err)
	}
	// The API tag wins over the local config.json inference (llama → text-generation).
	if entries[1].ID != "org/new" || entries[1].Task != "feature-extraction" {
		t.Errorf("registered entry = %+v, want org/new tagged feature-extraction", entries[1])
	}
}

func TestListDropsStaleManifestEntries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pointAPIAtTestServer(t, "")
	writeCachedModel(t, "org/keep")
	if err := lm.SaveManifest([]lm.ManifestEntry{registeredEntry("org/gone"), registeredEntry("org/keep")}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := runCLIOutput(t, "ls")
	if strings.Contains(stdout, "org/gone") {
		t.Errorf("lm ls stdout still lists org/gone:\n%s", stdout)
	}
	if !strings.Contains(stdout, "org/keep") {
		t.Errorf("lm ls stdout missing org/keep:\n%s", stdout)
	}
	if !strings.Contains(stderr, "note: dropped 1 manifest entry with no cache directory\n") {
		t.Errorf("lm ls stderr = %q, want dropped note", stderr)
	}
	entries, err := lm.LoadManifest()
	if err != nil || len(entries) != 1 || entries[0].ID != "org/keep" {
		t.Errorf("manifest after lm ls = %+v (err %v), want [org/keep]", entries, err)
	}
}

func TestRmUnregisteredCachedModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pointAPIAtTestServer(t, "")
	dir := writeCachedModel(t, "org/x")

	stdout, stderr := runCLIOutput(t, "rm", "-f", "org/x")
	if stdout != "Removed org/x\n" {
		t.Errorf("lm rm stdout = %q, want %q", stdout, "Removed org/x\n")
	}
	if strings.Contains(stderr, "not found in manifest") {
		t.Errorf("lm rm stderr = %q, want no manifest warning", stderr)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cache dir %s still exists (err %v)", dir, err)
	}
}

func TestRmUnknownModelErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pointAPIAtTestServer(t, "")

	cmd := newLmRmCmd()
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"-f", "org/ghost"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not found in manifest or Hugging Face cache") {
		t.Errorf("lm rm org/ghost error = %v, want not-found error", err)
	}
}
