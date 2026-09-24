package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/config"
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
			disk.WithCompression(gzipLevel(cfg)))
	}
	if err != nil {
		return nil, fmt.Sprintf("The cache is in memory only: %v", err)
	}
	// Objects in use are touched on every session, so trimming the ones
	// used least recently at startup is enough. It is only a cleanup, so a
	// failure is ignored.
	go func() { _, _ = store.Collect(ctx) }()
	return store, ""
}

// openEntries opens where the lists and details that account reads are kept,
// below the disk cache of the host, which trims them along with the rest.
// Each account has a directory of its own, named by a hash of its token, so
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
