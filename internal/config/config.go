// Package config loads, defaults and validates the user configuration.
//
// It has no knowledge of the tui: colors are hex strings and keys are the
// names bubbletea reports (such as "ctrl+c"), converted later by the tui.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/eggzec/gh-tui/internal/core"
)

// EnvPath is the environment variable that overrides the config file path.
const EnvPath = "GH_TUI_CONFIG"

// Config is the user configuration.
type Config struct {
	// Repos are the user's pinned repositories, as "owner/name".
	Repos []string `yaml:"repos" when:"startup" why:"the pinned repositories are read at startup"`
	// Theme is the name of the active theme, one of Themes.
	Theme string `yaml:"theme"`
	// Themes are the themes by name: those of default.yaml, and the user's.
	Themes map[string]Theme `yaml:"themes" scope:"global"`
	// Keys maps action names to keys. An entry replaces the default keys
	// of that action only.
	Keys  map[string][]string `yaml:"keys" scope:"global"`
	Cache Cache               `yaml:"cache" when:"startup" why:"the cache is opened at startup"`
	Sync  Sync                `yaml:"sync"`
	Files Files               `yaml:"files"`
	// Details configures the pull request and issue modals, and reading
	// them ahead, which the notifications follow too.
	Details Details `yaml:"details"`
	// Notifications configures the notifications screen and the
	// dashboard's inbox.
	Notifications Notifications `yaml:"notifications"`
	// History configures the History modal of the repository screen.
	History History `yaml:"history"`
	// Dashboard configures the screen the app opens on.
	Dashboard Dashboard `yaml:"dashboard"`
	UI        UI        `yaml:"ui"`
	// Images configures the images drawn inline, which only some
	// terminals show.
	Images Images `yaml:"images" when:"startup" why:"the terminal is asked whether it shows images at startup" scope:"global"`
	Auth   Auth   `yaml:"auth" when:"startup" why:"the token's checks start with the app"`
	Log    Log    `yaml:"log" scope:"global"`
	// Editor is the command of the editor that v opens a file in from the
	// pager, such as "vim" or "code --wait": a program and its arguments,
	// split at white space and run without a shell. Empty, as by default,
	// takes $VISUAL, and else $EDITOR.
	Editor string `yaml:"editor" scope:"global"`
}

// Sync configures background polling.
type Sync struct {
	Enabled bool `yaml:"enabled" when:"startup" why:"the polls are set up at startup"`
	// Interval is the polling interval when the server doesn't ask for a
	// longer one. It is at least minSyncInterval.
	Interval time.Duration `yaml:"interval"`
}

// minSyncInterval is the shortest polling interval: polls closer than that
// would spend the rate limit on changes that rarely come so often. The
// sync engine never polls more often either.
const minSyncInterval = 10 * time.Second

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

// Load reads the config file at path over the defaults, default.yaml,
// applies $GH_TUI_LOG over the log level, and validates the result. A
// mapping in the file merges key by key with the defaults, and anything
// else, a list too, replaces the default; an empty value is refused. A
// missing or empty file yields the defaults. A setting under the name it
// had before it was renamed is read under its new name, and returned
// among renamed, for the user to be told.
func Load(path string) (cfg Config, renamed []Renamed, err error) {
	tree := defaultTree()
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, nil, fmt.Errorf("load config: %w", err)
	default:
		if tree, renamed, err = overlay(tree, data); err != nil {
			return Config{}, nil, fmt.Errorf("load config %s: %w", path, err)
		}
	}
	// A value of the wrong type is reported at its line in the file, which
	// the nodes laid over the defaults keep.
	if cfg, err = decode(tree); err != nil {
		return Config{}, nil, fmt.Errorf("load config %s: %w", path, err)
	}
	if level := os.Getenv(EnvLog); level != "" {
		cfg.Log.Level = strings.ToLower(level)
	}
	if err := renamedErrors(cfg.Validate(), renamed); err != nil {
		return Config{}, nil, fmt.Errorf("invalid config %s:\n%w", path, err)
	}
	return cfg, renamed, nil
}

// overlay returns the user's file, data, merged over the tree of
// defaults, with its renamed settings moved to their new names first.
// Every key must then be known, so that a typo doesn't pass silently.
func overlay(tree *yaml.Node, data []byte) (*yaml.Node, []Renamed, error) {
	root, err := parseYAML(data)
	if err != nil || root == nil {
		return tree, nil, err
	}
	over, err := plain(root, "")
	if err != nil {
		return nil, nil, err
	}
	renamed, err := migrate(over, renames)
	if err != nil {
		return nil, nil, err
	}
	if err := errors.Join(checkKnown(over, reflect.TypeFor[Config](), "")...); err != nil {
		return nil, nil, err
	}
	return merge(tree, over), renamed, nil
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

	errs = append(errs, c.Cache.validate())
	if c.Sync.Interval < minSyncInterval {
		errs = append(errs, fmt.Errorf("sync.interval: must be at least %v, got %v", minSyncInterval, c.Sync.Interval))
	}
	errs = append(errs, c.Files.validate(), c.Details.validate(), c.History.validate(), c.Dashboard.validate(), c.UI.validate(), c.Images.validate(), c.Log.validate(),
		validateEditor(c.Editor))
	return errors.Join(errs...)
}
