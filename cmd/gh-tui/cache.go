package main

import (
	"compress/gzip"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/cmdhist"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// openDisk opens the disk cache of host as cfg says, and trims it to its
// size in the background. It returns nil when the cache is off, and also
// when it can't be opened, with a warning to show: the app then caches in
// memory only, rather than not starting.
func openDisk(ctx context.Context, cfg config.Disk, host string) (store *disk.Store, warning string) {
	if !cfg.Enabled {
		return nil, ""
	}
	root, err := cfg.Path()
	if err == nil {
		store, err = disk.Open(filepath.Join(root, hostDir(host)),
			disk.WithMaxSize(int64(cfg.MaxSize)),
			disk.WithCompression(gzipLevel(cfg)),
			// The command line's history lives beside the entries of each
			// account, and isn't a cache to trim.
			disk.WithKeep(cmdhist.FileName))
	}
	if err != nil {
		slog.Warn("disk cache off", "span", "cache.disk", "dir", ui.ShortPath(root), "err", err.Error())
		if root == "" {
			return nil, "The cache is in memory only: set cache.disk.dir in the config to a directory to keep it in."
		}
		return nil, "The cache is in memory only: " + couldntOpen(root, err)
	}
	// Objects in use are touched on every session, so trimming the ones
	// used least recently at startup is enough. It is only a cleanup, so a
	// failure is only logged.
	go func() {
		ctx, end := obs.Begin(ctx, "cache.collect")
		u, err := store.Collect(ctx)
		end(err, "span", "cache.disk")
		if err == nil {
			slog.InfoContext(ctx, "cache collected", "span", "cache.disk", "dir", ui.ShortPath(store.Dir()), "files", u.Files, "bytes", u.Size, "removed", u.Removed, "max_bytes", int64(cfg.MaxSize))
		}
	}()
	return store, ""
}

// openEntries opens where the lists and details that account reads are kept,
// below the disk cache of the host, which trims them along with the rest.
// Each account has a directory of its own, named by a hash of who it is, so
// that no account reads what another one kept. It returns nil when cfg
// keeps no entries, or the directory can't be opened: they are then cached
// in memory only.
func openEntries(cfg config.Disk, host *disk.Store, account string) *disk.Store {
	if host == nil || !cfg.Entries {
		return nil
	}
	store, err := disk.Open(filepath.Join(host.Dir(), entryDir, account), disk.WithCompression(gzipLevel(cfg)))
	if err != nil {
		return nil
	}
	return store
}

// accountKept reports whether moveAccount has nothing to move to the
// directory of account on host, since it is there already, or cfg keeps
// nothing, so that it needs no token to tell.
func accountKept(cfg config.Disk, host, account string) bool {
	if !cfg.Enabled {
		return true
	}
	root, err := cfg.Path()
	if err != nil {
		return true
	}
	_, err = os.Lstat(filepath.Join(root, hostDir(host), entryDir, account))
	return !errors.Is(err, fs.ErrNotExist)
}

// moveAccount renames the directory of an account from its old name to its
// new one, so that what it read and typed survives a change of its name:
// from a hash of its token, as gh-tui named accounts before, to a hash of
// its login. Only the account's own directory is moved, and only when the
// new one doesn't exist yet, so it never replaces or removes anything.
func moveAccount(cfg config.Disk, host, from, to string) {
	if !cfg.Enabled || from == "" || from == to {
		return
	}
	root, err := cfg.Path()
	if err != nil {
		return
	}
	dir := filepath.Join(root, hostDir(host), entryDir)
	src, dst := filepath.Join(dir, from), filepath.Join(dir, to)
	if _, err := os.Lstat(dst); !errors.Is(err, fs.ErrNotExist) {
		return
	}
	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() {
		return
	}
	if err := os.Rename(src, dst); err != nil {
		// Another gh-tui that started at the same time may have moved it
		// first.
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		slog.Warn("account cache not moved", "span", "cache.disk", "err", err.Error())
		return
	}
	slog.Info("account cache moved", "span", "cache.disk", "from", from, "to", to)
}

// historyPath returns where the lines of the command line that account
// typed are kept: in its directory of the disk cache of host, beside what
// it read, so no account recalls another's. It returns "" when the disk
// cache is off, and the lines then last for the session.
func historyPath(cfg config.Disk, host, account string) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}
	root, err := cfg.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, hostDir(host), entryDir, account, cmdhist.FileName), nil
}

// entryDir holds the directories of the accounts' entries in the directory
// of a host. Its objects go by kinds of lowercase letters, and account
// names are longer than their two-digit subdirectories, so nothing clashes.
const entryDir = "entry"

// hostDir names the directory of host's objects, since objects of one name
// on two hosts need not be the same.
func hostDir(host string) string {
	host = strings.ToLower(host)
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, host)
}

func gzipLevel(cfg config.Disk) int {
	if cfg.Compression == config.CompressionNone {
		return gzip.NoCompression
	}
	switch cfg.CompressionLevel {
	case config.LevelFastest:
		return gzip.BestSpeed
	case config.LevelBest:
		return gzip.BestCompression
	default:
		return gzip.DefaultCompression
	}
}
