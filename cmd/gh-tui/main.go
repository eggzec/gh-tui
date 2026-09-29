// Command gh-tui is a terminal client for GitHub.
//
// Usage:
//
//	gh-tui [--debug] [--hostname HOST] [--version]
//
// It opens on the dashboard, which shows the repository of the current
// directory first; :goto opens another repository, pull request or issue.
// It talks to the host --hostname names, else to that of the current
// directory's repository (GH_REPO or the git remotes), else to GH_HOST or
// the host gh is logged in to.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	debugLog := flag.Bool("debug", false, "log at debug level for this run, as GH_DEBUG does")
	showVersion := flag.Bool("version", false, "print the version and exit")
	hostname := flag.String("hostname", "", "the GitHub `host` to use, in place of the current repository's or gh's default")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: gh-tui [--debug] [--hostname HOST] [--version]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		return fmt.Errorf("unexpected argument %q: gh-tui takes no arguments; use :goto owner/name inside the app", flag.Arg(0))
	}
	if *showVersion {
		fmt.Println("gh-tui", version())
		return nil
	}

	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg, renamed, err := config.Load(path)
	if err != nil {
		return err
	}

	from := levelFrom(cfg.Log, *debugLog)
	if *debugLog || ghDebug(os.Getenv("GH_DEBUG")) {
		cfg.Log.Level = config.LevelDebug
	}
	closeLog, warning := openLog(cfg.Log)
	defer closeLog()
	logStart(cfg, path, from)
	for _, r := range renamed {
		slog.Warn("config uses an old setting", "span", "config", "old", r.Old, "line", r.Line, "new", strings.Join(r.New, ", "), "note", r.Note)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go obs.Summarize(ctx, cfg.Log.Summary)
	// The last summary covers the whole session.
	defer obs.Default().Log(context.Background())

	app, err := build(ctx, cfg, *hostname, warning, config.RenamedWarning(renamed))
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
