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
	TTL        time.Duration `yaml:"ttl"`
	Disk       Disk          `yaml:"disk"`
	Revalidate Revalidate    `yaml:"revalidate"`
}

// Revalidate configures checking cached entries in the background, such as
// the lists, issues and branches kept on disk, with one conditional request
// each, so that what a view reads is known to be current and needs no
// request when it loads. A request that finds nothing changed is free
// against GitHub's rate limit, but not against its limits on how many
// requests come at once, hence the budget.
type Revalidate struct {
	Enabled bool `yaml:"enabled"`
	// Interval is how often a pass over the entries starts. A pass
	// checks at most what the budget allows in an interval.
	Interval time.Duration `yaml:"interval"`
	// Budget is how many requests a minute the checks send at most. It
	// shrinks while the terminal is unfocused.
	Budget int `yaml:"budget"`
	// Scope is which entries are checked: ScopeRecent, those of the
	// selected repository and those used in the last week, or ScopeAll.
	Scope string `yaml:"scope"`
}

// Scopes of revalidation.
const (
	ScopeRecent = "recent"
	ScopeAll    = "all"
)

// The bounds of revalidation: passes closer than minRevalidateInterval
// would mostly find entries checked by the last one, and a budget above
// maxRevalidateBudget would come close to GitHub's secondary rate limits.
const (
	minRevalidateInterval = 10 * time.Second
	maxRevalidateBudget   = 300
)

// Disk configures the disk cache, which keeps what doesn't change, such as
// the files of a commit, across sessions, and where branches pointed, so
// that a new session asks GitHub only whether they moved.
type Disk struct {
	Enabled bool `yaml:"enabled"`
	// Entries keeps lists and details too, such as pull requests, issues
	// and notifications, apart for each account, so that a new session
	// shows them at once while it asks GitHub whether they changed.
	Entries bool `yaml:"entries"`
	// Dir is the cache directory. Empty means gh-tui in
	// [os.UserCacheDir].
	Dir string `yaml:"dir" scope:"global"`
	// MaxSize is the space the cache may take on disk. When it takes more
	// at startup, the objects used least recently go.
	MaxSize Size `yaml:"max_size" scope:"global"`
	// Compression is how new objects are stored: CompressionGzip or
	// CompressionNone. Objects stored either way stay readable.
	Compression string `yaml:"compression" scope:"global"`
	// CompressionLevel trades the time gzip takes for the space it saves:
	// LevelFastest, LevelDefault or LevelBest.
	CompressionLevel string `yaml:"compression_level" scope:"global"`
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
	r := c.Revalidate
	if r.Interval < minRevalidateInterval {
		errs = append(errs, fmt.Errorf("cache.revalidate.interval: must be at least %v, got %v", minRevalidateInterval, r.Interval))
	}
	if r.Budget < 1 || r.Budget > maxRevalidateBudget {
		errs = append(errs, fmt.Errorf("cache.revalidate.budget: must be between 1 and %d, got %d", maxRevalidateBudget, r.Budget))
	}
	if !slices.Contains([]string{ScopeRecent, ScopeAll}, r.Scope) {
		errs = append(errs, fmt.Errorf("cache.revalidate.scope: must be %s or %s, got %q", ScopeRecent, ScopeAll, r.Scope))
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
