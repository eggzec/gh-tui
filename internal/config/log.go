package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"
)

// EnvLog is the environment variable that overrides the log level, such as
// GH_TUI_LOG=debug for one run.
const EnvLog = "GH_TUI_LOG"

// Log configures the log file, which holds one JSON object per line. The
// app owns the terminal, so nothing is logged anywhere else.
type Log struct {
	// Level is the least level logged: LevelDebug, LevelInfo, LevelWarn or
	// LevelError.
	Level string `yaml:"level"`
	// File is the log file. Empty means gh-tui/gh-tui.log in the user's
	// state directory: $XDG_STATE_HOME, or ~/.local/state, also on macOS,
	// and %LocalAppData% on Windows.
	File string `yaml:"file"`
	// MaxSize is how large the file grows before it is rotated.
	MaxSize Size `yaml:"max_size"`
	// Keep is how many rotated files are kept.
	Keep int `yaml:"keep"`
	// Summary is how often a summary of the requests, the caches and the
	// reads ahead is logged. Zero turns these off; one is still logged on
	// exit.
	Summary time.Duration `yaml:"summary"`
}

// Log levels.
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// The bounds of logging: a file smaller than minLogSize would rotate every
// few requests at debug level, and summaries closer than minLogSummary
// would crowd out the rest.
const (
	minLogSize    = 64 * KiB
	maxLogKeep    = 100
	minLogSummary = 10 * time.Second
)

func defaultLog() Log {
	return Log{Level: LevelInfo, MaxSize: 10 * MiB, Keep: 3, Summary: 5 * time.Minute}
}

func (l Log) validate() error {
	var errs []error
	if !slices.Contains([]string{LevelDebug, LevelInfo, LevelWarn, LevelError}, l.Level) {
		errs = append(errs, fmt.Errorf("log.level: must be %s, %s, %s or %s, got %q", LevelDebug, LevelInfo, LevelWarn, LevelError, l.Level))
	}
	if l.File != "" && !filepath.IsAbs(l.File) {
		errs = append(errs, fmt.Errorf("log.file: must be an absolute path, got %q", l.File))
	}
	if l.MaxSize < minLogSize {
		errs = append(errs, fmt.Errorf("log.max_size: must be at least %v, got %v", minLogSize, l.MaxSize))
	}
	if l.Keep < 0 || l.Keep > maxLogKeep {
		errs = append(errs, fmt.Errorf("log.keep: must be between 0 and %d, got %d", maxLogKeep, l.Keep))
	}
	if l.Summary != 0 && l.Summary < minLogSummary {
		errs = append(errs, fmt.Errorf("log.summary: must be 0 or at least %v, got %v", minLogSummary, l.Summary))
	}
	return errors.Join(errs...)
}

// Path returns the log file: File, or gh-tui/gh-tui.log in the user's
// state directory.
func (l Log) Path() (string, error) {
	if l.File != "" {
		return l.File, nil
	}
	dir, err := stateDir()
	if err != nil {
		return "", fmt.Errorf("log path: %w", err)
	}
	return filepath.Join(dir, "gh-tui", "gh-tui.log"), nil
}

// stateDir returns where the user's programs keep state that is worth
// keeping but not backing up, such as logs, following the XDG base
// directories everywhere but on Windows.
func stateDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(dir) {
		return dir, nil
	}
	if runtime.GOOS == "windows" {
		return os.UserCacheDir()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}
