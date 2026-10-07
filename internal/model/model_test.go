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

func TestCatalogParsesWithOneVerifiedSeed(t *testing.T) {
	entries, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].ID != "mlx-community/Qwen3.5-4B-OptiQ-4bit" || !entries[0].Verified {
		t.Errorf("first entry = %+v, want the verified Qwen3.5-4B seed", entries[0])
	}
	if entries[0].ChatTemplateArgs["enable_thinking"] != false {
		t.Errorf("seed should disable thinking: %v", entries[0].ChatTemplateArgs)
	}
	for _, e := range entries[1:] {
		if e.Verified {
			t.Errorf("%s is marked verified without an AT8 run", e.ID)
		}
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
