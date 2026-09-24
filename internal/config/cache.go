package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Cache configures the response cache.
type Cache struct {
	// TTL is how long a cached response is served before it is revalidated.
	TTL  time.Duration `yaml:"ttl"`
	Disk Disk          `yaml:"disk"`
}

// Disk configures the disk cache, which keeps what doesn't change, such as
// the files of a commit, across sessions, and where branches pointed, so
// that a new session asks GitHub only whether they moved.
type Disk struct {
	Enabled bool `yaml:"enabled"`
	// Dir is the cache directory. Empty means gh-tui in
	// [os.UserCacheDir].
	Dir string `yaml:"dir"`
	// MaxSize is the space the cache may take on disk. When it takes more
	// at startup, the objects used least recently go.
	MaxSize Size `yaml:"max_size"`
	// Compression is how new objects are stored: CompressionGzip or
	// CompressionNone. Objects stored either way stay readable.
	Compression string `yaml:"compression"`
	// CompressionLevel trades the time gzip takes for the space it saves:
	// LevelFastest, LevelDefault or LevelBest.
	CompressionLevel string `yaml:"compression_level"`
}

// Compressions and their levels.
const (
	CompressionGzip = "gzip"
	CompressionNone = "none"

	LevelFastest = "fastest"
	LevelDefault = "default"
	LevelBest    = "best"
)

// minDiskSize is the smallest disk cache worth having: a few listings of
// large repositories.
const minDiskSize = 8 * MiB

func defaultCache() Cache {
	return Cache{
		TTL: 5 * time.Minute,
		Disk: Disk{
			Enabled:          true,
			MaxSize:          512 * MiB,
			Compression:      CompressionGzip,
			CompressionLevel: LevelDefault,
		},
	}
}

func (c Cache) validate() error {
	var errs []error
	if c.TTL <= 0 {
		errs = append(errs, fmt.Errorf("cache.ttl: must be positive, got %v", c.TTL))
	}
	d := c.Disk
	if d.MaxSize < minDiskSize {
		errs = append(errs, fmt.Errorf("cache.disk.max_size: must be at least %v, got %v", minDiskSize, d.MaxSize))
	}
	if d.Dir != "" && !filepath.IsAbs(d.Dir) {
		errs = append(errs, fmt.Errorf("cache.disk.dir: must be an absolute path, got %q", d.Dir))
	}
	if !slices.Contains([]string{CompressionGzip, CompressionNone}, d.Compression) {
		errs = append(errs, fmt.Errorf("cache.disk.compression: must be %s or %s, got %q", CompressionGzip, CompressionNone, d.Compression))
	}
	if !slices.Contains([]string{LevelFastest, LevelDefault, LevelBest}, d.CompressionLevel) {
		errs = append(errs, fmt.Errorf("cache.disk.compression_level: must be %s, %s or %s, got %q", LevelFastest, LevelDefault, LevelBest, d.CompressionLevel))
	}
	return errors.Join(errs...)
}

// Path returns the cache directory: Dir, or gh-tui in [os.UserCacheDir].
func (d Disk) Path() (string, error) {
	if d.Dir != "" {
		return d.Dir, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache path: %w", err)
	}
	return filepath.Join(dir, "gh-tui"), nil
}
