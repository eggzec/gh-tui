package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
)

func TestOpenDisk(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default().Cache.Disk
	cfg.Dir = root
	store, warning := openDisk(t.Context(), cfg, "api.github.com")
	if store == nil || warning != "" {
		t.Fatalf("openDisk = %v, %q; want a store", store, warning)
	}
	if want := filepath.Join(root, "api.github.com"); store.Dir() != want {
		t.Errorf("Dir() = %q, want %q", store.Dir(), want)
	}
}

func TestOpenDiskDisabled(t *testing.T) {
	cfg := config.Default().Cache.Disk
	cfg.Enabled, cfg.Dir = false, t.TempDir()
	if store, warning := openDisk(t.Context(), cfg, "api.github.com"); store != nil || warning != "" {
		t.Errorf("openDisk = %v, %q; want neither a store nor a warning", store, warning)
	}
}

func TestOpenDiskFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default().Cache.Disk
	cfg.Dir = file
	store, warning := openDisk(t.Context(), cfg, "api.github.com")
	if store != nil || !strings.Contains(warning, "memory only") {
		t.Errorf("openDisk = %v, %q; want no store and a warning", store, warning)
	}
}

func TestHostDir(t *testing.T) {
	for host, want := range map[string]string{
		"api.github.com":  "api.github.com",
		"GHE.Example.com": "ghe.example.com",
		"../etc":          ".._etc",
		"[::1]":           "___1_",
	} {
		if got := hostDir(host); got != want {
			t.Errorf("hostDir(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestGzipLevel(t *testing.T) {
	tests := []struct {
		compression, level string
		want               int
	}{
		{config.CompressionGzip, config.LevelDefault, gzip.DefaultCompression},
		{config.CompressionGzip, config.LevelFastest, gzip.BestSpeed},
		{config.CompressionGzip, config.LevelBest, gzip.BestCompression},
		{config.CompressionNone, config.LevelBest, gzip.NoCompression},
	}
	for _, tt := range tests {
		if got := gzipLevel(config.Disk{Compression: tt.compression, CompressionLevel: tt.level}); got != tt.want {
			t.Errorf("gzipLevel(%s, %s) = %d, want %d", tt.compression, tt.level, got, tt.want)
		}
	}
}

func TestOpenEntries(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default().Cache.Disk
	cfg.Dir = root
	host, _ := openDisk(t.Context(), cfg, "api.github.com")
	const alice, bob = "0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210"

	store := openEntries(cfg, host, alice)
	if store == nil {
		t.Fatal("openEntries = nil, want a store")
	}
	if want := filepath.Join(root, "api.github.com", "entry", alice); store.Dir() != want {
		t.Errorf("Dir() = %q, want %q", store.Dir(), want)
	}
	fi, err := os.Stat(store.Dir())
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("account directory mode = %v, %v; want 0700", fi.Mode().Perm(), err)
	}

	// What one account keeps, another can't read.
	if err := cache.NewShelf[string](store, "issue", 1).Save("issue:o/r#1", cache.Entry[string]{Value: "private"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.NewShelf[string](openEntries(cfg, host, alice), "issue", 1).Load("issue:o/r#1"); !ok {
		t.Error("the same account can't read its entry")
	}
	if e, ok := cache.NewShelf[string](openEntries(cfg, host, bob), "issue", 1).Load("issue:o/r#1"); ok {
		t.Errorf("another account read %q", e.Value)
	}

	cfg.Entries = false
	if store := openEntries(cfg, host, alice); store != nil {
		t.Error("openEntries with entries off != nil")
	}
	if store := openEntries(config.Default().Cache.Disk, nil, alice); store != nil {
		t.Error("openEntries without a disk cache != nil")
	}
}
