package main

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"

	"github.com/eggzec/gh-tui/internal/buildinfo"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/logfile"
	"github.com/eggzec/gh-tui/internal/obs"
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
		return func() {}, fmt.Sprintf("Logging is off: %v", err)
	}
	session := obs.NewID(obs.SessionPrefix)
	slog.SetDefault(obs.NewLogger(f, level, session))
	slog.Info("start", "pid", os.Getpid(), "version", version(), "log_level", level.String(), "file", path)
	return func() { _ = f.Close() }, ""
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
