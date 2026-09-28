package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cmdhist"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/obs"
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	file := filepath.Join(home, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default().Cache.Disk
	cfg.Dir = file
	store, warning := openDisk(t.Context(), cfg, "api.github.com")
	want := "The cache is in memory only: couldn't open " + filepath.Join("~", "file") + " ("
	if store != nil || !strings.HasPrefix(warning, want) || strings.Contains(warning, home) {
		t.Errorf("openDisk = %v, %q; want no store and a warning that starts %q and doesn't name the home directory", store, warning, want)
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

func TestHistoryPath(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default().Cache.Disk
	cfg.Dir = root
	a, err := historyPath(cfg, "GitHub.com", "acct1")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "github.com", entryDir, "acct1", "cmdline-history.json"); a != want {
		t.Errorf("historyPath = %q, want %q, beside what the account keeps", a, want)
	}
	b, _ := historyPath(cfg, "github.com", "acct2")
	c, _ := historyPath(cfg, "ghe.example:8443", "acct1")
	if a == b || a == c {
		t.Errorf("accounts share a history: %q, %q, %q", a, b, c)
	}
	cfg.Enabled = false
	if p, err := historyPath(cfg, "github.com", "acct1"); p != "" || err != nil {
		t.Errorf("historyPath with the disk cache off = %q, %v, want none", p, err)
	}
}

func TestDiskCacheKeepsTheHistory(t *testing.T) {
	cfg := config.Default().Cache.Disk
	cfg.Dir = t.TempDir()
	cfg.MaxSize = 1
	store, _ := openDisk(t.Context(), cfg, "api.github.com")
	if store == nil {
		t.Fatal("no store")
	}
	path, _ := historyPath(cfg, "api.github.com", "acct1")
	if err := cmdhist.New(path, 0).Save([]string{"goto cli/cli"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Collect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("collecting the cache removed the history: %v", err)
	}
}

func TestMoveAccount(t *testing.T) {
	const host = "api.github.com"
	setup := func(t *testing.T, dirs ...string) (config.Disk, string) {
		t.Helper()
		cfg := config.Default().Cache.Disk
		cfg.Dir = t.TempDir()
		base := filepath.Join(cfg.Dir, host, entryDir)
		for _, d := range dirs {
			if err := os.MkdirAll(filepath.Join(base, d), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(base, d, cmdhist.FileName), []byte(d+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return cfg, base
	}
	history := func(t *testing.T, base, account string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(base, account, cmdhist.FileName))
		if err != nil {
			return ""
		}
		return string(b)
	}

	t.Run("moves the token's directory", func(t *testing.T) {
		cfg, base := setup(t, "token", "other")
		moveAccount(cfg, host, "token", "login")
		if got := history(t, base, "login"); got != "token\n" {
			t.Errorf("history under the new name = %q, want the old one's", got)
		}
		if _, err := os.Stat(filepath.Join(base, "token")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the old directory is still there: %v", err)
		}
		if got := history(t, base, "other"); got != "other\n" {
			t.Errorf("another account's history = %q, want it untouched", got)
		}
	})
	t.Run("keeps an existing directory", func(t *testing.T) {
		cfg, base := setup(t, "token", "login")
		moveAccount(cfg, host, "token", "login")
		if got := history(t, base, "login"); got != "login\n" {
			t.Errorf("history under the new name = %q, want it kept", got)
		}
		if got := history(t, base, "token"); got != "token\n" {
			t.Errorf("history under the old name = %q, want it left alone", got)
		}
	})
	t.Run("nothing to move", func(t *testing.T) {
		cfg, base := setup(t)
		moveAccount(cfg, host, "token", "login")
		if _, err := os.Stat(filepath.Join(base, "login")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("a directory was made: %v", err)
		}
	})
	t.Run("same name", func(t *testing.T) {
		cfg, base := setup(t, "token")
		moveAccount(cfg, host, "token", "token")
		if got := history(t, base, "token"); got != "token\n" {
			t.Errorf("history = %q, want it kept", got)
		}
	})
	t.Run("disk cache off", func(t *testing.T) {
		cfg, base := setup(t, "token")
		cfg.Enabled = false
		moveAccount(cfg, host, "token", "login")
		if got := history(t, base, "token"); got != "token\n" {
			t.Errorf("history = %q, want it left alone", got)
		}
	})
}

// syncBuffer is a buffer that a goroutine may write while a test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The records of the disk cache say which directory, with the home
// directory as ~, and, as every record of the session, which host.
func TestDiskCacheRecordsNameTheDirectory(t *testing.T) {
	restoreLogger(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	var buf syncBuffer
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelInfo, "s_test"))
	logHost("github.com")

	cfg := config.Default().Cache.Disk
	cfg.Dir = filepath.Join(home, "cache")
	if store, _ := openDisk(t.Context(), cfg, "api.github.com"); store == nil {
		t.Fatal("no store")
	}
	file := filepath.Join(home, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Dir = file
	openDisk(t.Context(), cfg, "api.github.com")

	want := map[string]string{
		"cache collected": filepath.Join("~", "cache", "api.github.com"),
		"disk cache off":  filepath.Join("~", "file"),
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := map[string]string{}
		for _, r := range readRecords(t, []byte(buf.String())) {
			msg, _ := r["msg"].(string)
			if _, ok := want[msg]; ok {
				if r["host"] != "github.com" {
					t.Errorf("record %v doesn't name the host", r)
				}
				got[msg], _ = r["dir"].(string)
			}
		}
		if len(got) == len(want) || time.Now().After(deadline) {
			if !maps.Equal(got, want) {
				t.Errorf("dirs = %v, want %v", got, want)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
