// Package config loads, defaults and validates the user configuration.
//
// It has no knowledge of the tui: colors are hex strings and keys are the
// names bubbletea reports (such as "ctrl+c"), converted later by the tui.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/eggzec/gh-tui/internal/core"
)

// EnvPath is the environment variable that overrides the config file path.
const EnvPath = "GH_TUI_CONFIG"

// Config is the user configuration.
type Config struct {
	// Repos are the user's pinned repositories, as "owner/name".
	Repos []string `yaml:"repos"`
	// Theme is the name of the active theme, built in or from Themes.
	Theme string `yaml:"theme"`
	// Themes are user-defined themes. They shadow built-in themes of the
	// same name.
	Themes map[string]Theme `yaml:"themes"`
	// Keys maps action names to keys. An entry replaces the default keys
	// of that action only.
	Keys  map[string][]string `yaml:"keys"`
	Cache Cache               `yaml:"cache"`
	Sync  Sync                `yaml:"sync"`
	Files Files               `yaml:"files"`
}

// Cache configures the response cache.
type Cache struct {
	// TTL is how long a cached response is served before it is revalidated.
	TTL time.Duration `yaml:"ttl"`
}

// Sync configures background polling.
type Sync struct {
	Enabled bool `yaml:"enabled"`
	// Interval is the polling interval when the server doesn't ask for a
	// longer one.
	Interval time.Duration `yaml:"interval"`
}

// Default returns the default configuration. Each call returns fresh maps
// and slices, so callers may modify the result.
func Default() Config {
	return Config{
		Repos:  []string{},
		Theme:  DefaultTheme,
		Themes: map[string]Theme{},
		Keys:   defaultKeys(),
		Cache:  Cache{TTL: 5 * time.Minute},
		Sync:   Sync{Enabled: true, Interval: time.Minute},
		Files:  defaultFiles(),
	}
}

// Path returns the config file path: $GH_TUI_CONFIG if set, else
// gh-tui/config.yaml in [os.UserConfigDir].
func Path() (string, error) {
	if p := os.Getenv(EnvPath); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config path: %w", err)
	}
	return filepath.Join(dir, "gh-tui", "config.yaml"), nil
}

// Load reads the config file at path over the defaults and validates the
// result. A missing or empty file yields the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// Decoding into the defaults merges maps key by key, so overriding one
	// action or theme leaves the others in place.
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config %s:\n%w", path, err)
	}
	return cfg, nil
}

// Validate reports every problem in c at once, joined with [errors.Join].
func (c Config) Validate() error {
	var errs []error
	for i, r := range c.Repos {
		if _, err := core.ParseRepoRef(r); err != nil {
			errs = append(errs, fmt.Errorf("repos[%d]: %w", i, err))
		}
	}

	if _, ok := c.theme(c.Theme); !ok {
		errs = append(errs, fmt.Errorf("theme: unknown theme %q", c.Theme))
	}
	for _, name := range slices.Sorted(maps.Keys(c.Themes)) {
		errs = append(errs, c.Themes[name].validate("themes."+name))
	}

	for _, action := range slices.Sorted(maps.Keys(c.Keys)) {
		errs = append(errs, validateKeys(action, c.Keys[action]))
	}

	if c.Cache.TTL <= 0 {
		errs = append(errs, fmt.Errorf("cache.ttl: must be positive, got %v", c.Cache.TTL))
	}
	if c.Sync.Interval <= 0 {
		errs = append(errs, fmt.Errorf("sync.interval: must be positive, got %v", c.Sync.Interval))
	}
	errs = append(errs, c.Files.validate())
	return errors.Join(errs...)
}
