package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gib = 1 << 30

func seedEntries(verified30B bool) []Entry {
	return []Entry{
		{ID: "small", DiskGB: 2.8, Verified: true},
		{ID: "medium", DiskGB: 5.7, Verified: true},
		{ID: "large", DiskGB: 16.4, Verified: verified30B},
	}
}

func TestPickAcrossMemorySizes(t *testing.T) {
	cases := []struct {
		memGiB uint64
		want   string
	}{{8, "small"}, {16, "medium"}, {24, "medium"}, {32, "large"}}
	for _, c := range cases {
		got, err := Pick(c.memGiB*gib, seedEntries(true))
		if err != nil {
			t.Fatalf("%d GiB: %v", c.memGiB, err)
		}
		if got.ID != c.want {
			t.Errorf("%d GiB: picked %s, want %s", c.memGiB, got.ID, c.want)
		}
	}
}

func TestPickSkipsUnverifiedEntries(t *testing.T) {
	got, err := Pick(32*gib, seedEntries(false))
	if err != nil || got.ID != "medium" {
		t.Fatalf("picked %v (err %v), want medium when large is unverified", got.ID, err)
	}
}

func TestPickErrorNamesSmallestRequirement(t *testing.T) {
	_, err := Pick(4*gib, seedEntries(true))
	if err == nil {
		t.Fatal("expected an error at 4 GiB")
	}
	if !strings.Contains(err.Error(), "small") || !strings.Contains(err.Error(), "5.2 GB") {
		t.Errorf("error should name the smallest entry and its 5.2 GB requirement: %v", err)
	}
}

func TestCatalogParsesWithVerifiedQwen35Entries(t *testing.T) {
	entries, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	verified := map[string]bool{}
	for i, e := range entries {
		if i > 0 && e.DiskGB < entries[i-1].DiskGB {
			t.Errorf("catalog not sorted by disk_gb at %s", e.ID)
		}
		if e.Verified {
			verified[e.ID] = true
			if e.ChatTemplateArgs["enable_thinking"] != false {
				t.Errorf("%s: verified Qwen3.5 entries must disable thinking: %v", e.ID, e.ChatTemplateArgs)
			}
		}
	}
	for _, want := range []string{"mlx-community/Qwen3.5-4B-OptiQ-4bit", "mlx-community/Qwen3.5-9B-OptiQ-4bit"} {
		if !verified[want] {
			t.Errorf("%s should be verified", want)
		}
	}
	if len(verified) != 2 {
		t.Errorf("verified entries = %v, want exactly the two probed models", verified)
	}
}

func TestSnapshotDirReturnsLatestSnapshot(t *testing.T) {
	hub := t.TempDir()
	t.Setenv("HF_HUB_CACHE", hub)
	snaps := filepath.Join(hub, "models--org--name", "snapshots")
	for _, h := range []string{"aaa", "bbb"} {
		if err := os.MkdirAll(filepath.Join(snaps, h), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := SnapshotDir("org/name")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(snaps, "bbb") {
		t.Errorf("SnapshotDir = %s, want the bbb snapshot", got)
	}
	if _, err := SnapshotDir("org/missing"); err == nil {
		t.Error("missing model should error")
	}
}
