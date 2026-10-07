// Package model picks, locates, and downloads the one MLX model iq runs for pi.
package model

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var catalogYAML []byte

// Entry is one curated catalog model.
type Entry struct {
	ID               string         `yaml:"id"`
	DiskGB           float64        `yaml:"disk_gb"`
	ContextWindow    int            `yaml:"context_window"`
	ChatTemplateArgs map[string]any `yaml:"chat_template_args"`
	Verified         bool           `yaml:"verified"`
	Notes            string         `yaml:"notes"`
}

// Fit-rule constants. The budget is the share of unified memory a model may
// use; the estimate is disk size with weight overhead plus key-value cache headroom.
const (
	BudgetFraction  = 0.70
	DiskOverhead    = 1.15
	CacheHeadroomGB = 2.0
	gb              = 1e9
)

// Catalog parses the embedded catalog.
func Catalog() ([]Entry, error) {
	var doc struct {
		Entries []Entry `yaml:"entries"`
	}
	if err := yaml.Unmarshal(catalogYAML, &doc); err != nil {
		return nil, fmt.Errorf("parsing embedded catalog: %w", err)
	}
	if len(doc.Entries) == 0 {
		return nil, fmt.Errorf("embedded catalog has no entries")
	}
	return doc.Entries, nil
}

// EstimateBytes returns the memory an entry is expected to need.
func EstimateBytes(e Entry) uint64 {
	return uint64(e.DiskGB*DiskOverhead*gb + CacheHeadroomGB*gb)
}

// Budget returns the memory iq allows a model on a machine with memBytes.
func Budget(memBytes uint64) uint64 {
	return uint64(float64(memBytes) * BudgetFraction)
}

// Pick returns the largest verified entry whose estimate fits the budget.
// Unverified entries are never picked.
func Pick(memBytes uint64, entries []Entry) (Entry, error) {
	budget := Budget(memBytes)
	var best *Entry
	var smallest *Entry
	for i := range entries {
		e := &entries[i]
		if !e.Verified {
			continue
		}
		if smallest == nil || e.DiskGB < smallest.DiskGB {
			smallest = e
		}
		if EstimateBytes(*e) <= budget && (best == nil || e.DiskGB > best.DiskGB) {
			best = e
		}
	}
	if best != nil {
		return *best, nil
	}
	if smallest == nil {
		return Entry{}, fmt.Errorf("no verified model in the catalog")
	}
	return Entry{}, fmt.Errorf("no catalog model fits %s of memory: the smallest, %s, needs about %s against a budget of %s",
		FormatGB(memBytes), smallest.ID, FormatGB(EstimateBytes(*smallest)), FormatGB(budget))
}

// MemBytes returns the machine's unified memory via sysctl hw.memsize.
func MemBytes() (uint64, error) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("reading hw.memsize: %w", err)
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing hw.memsize %q: %w", strings.TrimSpace(string(out)), err)
	}
	return n, nil
}

// FormatGB renders bytes as decimal gigabytes with one decimal place.
func FormatGB(b uint64) string {
	return fmt.Sprintf("%.1f GB", float64(b)/gb)
}

// HFHubDir returns the Hugging Face hub cache directory, honouring
// HF_HUB_CACHE, then HF_HOME, then the default under the home directory.
func HFHubDir() string {
	if v := os.Getenv("HF_HUB_CACHE"); v != "" {
		return v
	}
	if v := os.Getenv("HF_HOME"); v != "" {
		return filepath.Join(v, "hub")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "huggingface", "hub")
}

// HFCacheDir returns the hub cache directory for a model ID.
func HFCacheDir(id string) string {
	return filepath.Join(HFHubDir(), "models--"+strings.ReplaceAll(id, "/", "--"))
}

// SnapshotDir returns the path of the most recent snapshot directory for a
// model, which is what mlx_lm.server expects as its --model argument.
func SnapshotDir(modelID string) (string, error) {
	snapshotsDir := filepath.Join(HFCacheDir(modelID), "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		return "", fmt.Errorf("no snapshots found for %s: %w", modelID, err)
	}
	var snapDir string
	for _, e := range entries {
		if e.IsDir() {
			snapDir = filepath.Join(snapshotsDir, e.Name())
		}
	}
	if snapDir == "" {
		return "", fmt.Errorf("no snapshots found for %s", modelID)
	}
	return snapDir, nil
}

// Download fetches a model into the hub cache with the hf CLI, streaming its
// output to w.
func Download(modelID string, w io.Writer) error {
	hf, err := exec.LookPath("hf")
	if err != nil {
		return fmt.Errorf("hf CLI not found — install with: pipx install huggingface_hub")
	}
	cmd := exec.Command(hf, "download", modelID)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hf download %s: %w", modelID, err)
	}
	return nil
}
