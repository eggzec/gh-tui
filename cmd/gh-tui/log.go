package main

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/buildinfo"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/logfile"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// openLog makes the default logger write to the log file that cfg names,
// one JSON object per line, each with the id of this session. It returns
// the function that closes the file. If the file can't be opened, the app
// logs nothing rather than not starting, and openLog returns a warning to
// show.
func openLog(cfg config.Log) (closeLog func(), warning string) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = slog.LevelInfo
	}
	path, err := cfg.Path()
	var f *logfile.File
	if err == nil {
		f, err = logfile.Open(path, int64(cfg.MaxSize), cfg.Keep)
	}
	if err != nil {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		if path == "" {
			return func() {}, "Logging is off: set log.file in the config to a file to log to."
		}
		return func() {}, "Logging is off: " + couldntOpen(path, err)
	}
	session := obs.NewID(obs.SessionPrefix)
	slog.SetDefault(obs.NewLogger(f, level, session))
	return func() { _ = f.Close() }, ""
}

// logStart logs the start of the session: what the binary was built
// from, how it is set up and the terminal it runs in, so that a log says
// what a problem needs to be reproduced. cfg was read from configPath,
// and levelFrom says what set its log level. It reads only the
// environment and what the binary knows of itself, and never logs a
// value that may be secret.
func logStart(cfg config.Config, configPath, levelFrom string) {
	b := buildinfo.Read()
	// Read as openLog reads it, which takes a level it can't read for info.
	var level slog.Level
	_ = level.UnmarshalText([]byte(cfg.Log.Level))
	logPath, _ := cfg.Log.Path()
	_, statErr := os.Stat(configPath)
	attrs := []slog.Attr{
		slog.String("span", "app"),
		slog.Int("pid", os.Getpid()),
		slog.String("version", version()),
	}
	if b.Revision != "" {
		attrs = append(attrs, slog.String("vcs_revision", b.Revision), slog.String("vcs_time", b.Time), slog.Bool("vcs_modified", b.Modified))
	}
	attrs = append(attrs,
		slog.String("go", b.Go),
		slog.String("goos", runtime.GOOS),
		slog.String("goarch", runtime.GOARCH),
		slog.String("cgo", b.CGO),
		slog.String("log_level", level.String()),
		slog.String("level_from", levelFrom),
		// A log may be shared, so the paths don't name the user's home.
		slog.String("file", ui.ShortPath(logPath)),
		slog.String("config_path", ui.ShortPath(configPath)),
		slog.Bool("config_exists", statErr == nil),
	)
	if changed := cfg.Changed(); len(changed) > 0 {
		set := make([]any, len(changed))
		for i, s := range changed {
			set[i] = slog.String(s.Key, s.Value)
		}
		attrs = append(attrs, slog.Group("non_default", set...))
	}
	attrs = append(attrs, terminalEnv(os.Getenv, os.Environ())...)
	slog.LogAttrs(context.Background(), slog.LevelInfo, "start", attrs...)
}

// levelFrom says what set the log level of cfg, before --debug, which
// debugFlag says was given, and GH_DEBUG raise it: the flag, GH_DEBUG,
// GH_TUI_LOG, the config file or the default.
func levelFrom(cfg config.Log, debugFlag bool) string {
	switch {
	case debugFlag:
		return "--debug"
	case ghDebug(os.Getenv("GH_DEBUG")):
		return "GH_DEBUG"
	case os.Getenv(config.EnvLog) != "":
		return config.EnvLog
	case cfg.Level != config.Default().Log.Level:
		return "config"
	}
	return "default"
}

// terminalEnv returns what the environment, read with getenv and listed
// in environ, says of the terminal and the locale, which decide how the
// app looks and what it can draw: the variables that are set, and never
// any other.
func terminalEnv(getenv func(string) string, environ []string) []slog.Attr {
	var attrs []slog.Attr
	for _, v := range []string{"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION"} {
		if val := getenv(v); val != "" {
			attrs = append(attrs, slog.String(strings.ToLower(v), val))
		}
	}
	for _, m := range []struct{ env, name string }{{"TMUX", "tmux"}, {"STY", "screen"}, {"ZELLIJ", "zellij"}} {
		if getenv(m.env) != "" {
			attrs = append(attrs, slog.String("multiplexer", m.name))
			break
		}
	}
	attrs = append(attrs,
		slog.Bool("ssh", getenv("SSH_TTY") != "" || getenv("SSH_CONNECTION") != ""),
		slog.Bool("no_color", getenv("NO_COLOR") != ""))
	var locale []any
	for _, kv := range slices.Sorted(slices.Values(environ)) {
		name, val, _ := strings.Cut(kv, "=")
		if val != "" && (name == "LANG" || strings.HasPrefix(name, "LC_")) {
			locale = append(locale, slog.String(strings.ToLower(name), val))
		}
	}
	if len(locale) > 0 {
		attrs = append(attrs, slog.Group("locale", locale...))
	}
	return attrs
}

// ghDebug reports whether GH_DEBUG, set to value, asks for debug output.
// gh reads it so, and prints to stderr; gh-tui's screen is the terminal,
// so it logs at debug level to its log file instead.
func ghDebug(value string) bool {
	switch value {
	case "", "0", "false", "no":
		return false
	}
	return true
}

// version returns the version of the module the binary was built from.
func version() string {
	return cmp.Or(buildinfo.Version(), "unknown")
}

// couldntOpen says, in a warning, that path couldn't be opened and why, if
// the system said, with the home directory as ~, so that the screen doesn't
// show where it is.
func couldntOpen(path string, err error) string {
	s := "couldn't open " + ui.ShortPath(path)
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		s += " (" + pe.Err.Error() + ")"
	}
	return s + "."
}
