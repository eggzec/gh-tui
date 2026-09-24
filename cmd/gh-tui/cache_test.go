package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
