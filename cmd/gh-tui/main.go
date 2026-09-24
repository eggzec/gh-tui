// Command gh-tui is a terminal client for GitHub.
//
// Usage:
//
//	gh-tui [--debug] [owner/name]
//
// It opens on the repository given, or else on the dashboard, which shows
// the repository of the current directory first.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/obs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	debugLog := flag.Bool("debug", false, "log at debug level for this run")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: gh-tui [--debug] [owner/name]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		return fmt.Errorf("want at most one repository, got %d arguments", flag.NArg())
	}

	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	if *debugLog {
		cfg.Log.Level = config.LevelDebug
	}
	closeLog, warning := openLog(cfg.Log)
	defer closeLog()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go obs.Summarize(ctx, cfg.Log.Summary)
	// The last summary covers the whole session.
	defer obs.Default().Log(context.Background())

	app, err := build(ctx, cfg, flag.Arg(0), warning)
	if err != nil {
		slog.Error("start failed", "err", err.Error())
		return err
	}
	_, err = tea.NewProgram(app).Run()
	if err != nil {
		slog.Error("exit", "err", err.Error())
	} else {
		slog.Info("exit")
	}
	return err
}
