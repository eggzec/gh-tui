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

// usage prints how to run gh-tui, with its flags spelled as the usage line
// and gh spell them, with two dashes.
func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintln(out, "usage: gh-tui [--debug] [--hostname HOST] [--version]")
	flag.VisitAll(func(f *flag.Flag) {
		arg, text := flag.UnquoteUsage(f)
		if arg != "" {
			arg = " " + arg
		}
		fmt.Fprintf(out, "  --%s%s\n    \t%s\n", f.Name, arg, text)
	})
}

func run() error {
	debugLog := flag.Bool("debug", false, "log at debug level for this run, as GH_DEBUG does")
	showVersion := flag.Bool("version", false, "print the version and exit")
	hostname := flag.String("hostname", "", "use the GitHub host `HOST`, in place of the current repository's or gh's default")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		return fmt.Errorf("unexpected argument %q: gh-tui takes no arguments; use :goto owner/name inside the app", flag.Arg(0))
	}
	if *hostname != "" {
		if err := checkHostname(*hostname); err != nil {
			return err
		}
	}
	if *showVersion {
		fmt.Println("gh-tui", version())
		return nil
	}

	path, err := config.Path()
	if err != nil {
		return err
	}
	file, err := config.Load(path)
	if err != nil {
		return err
	}
	// The log opens before the host is known, on the settings of the top
	// level of the file, which are the same for every host and account.
	cfg, renamed := file.Base(), file.Renamed()

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

	app, found, err := build(ctx, file, cfg.Log.Level, *hostname, warning, config.RenamedWarning(renamed))
	if err != nil {
		slog.Error("start failed", "err", err.Error())
		return err
	}
	p := tea.NewProgram(app)
	// A token that gh couldn't read after all fails the start, as one
	// that build found missing does.
	failed := make(chan error, 1)
	go func() {
		if err := <-found; err != nil {
			failed <- err
			p.Quit()
		}
	}()
	_, err = p.Run()
	select {
	case err := <-failed:
		slog.Error("start failed", "err", err.Error())
		return err
	default:
	}
	if err != nil {
		slog.Error("exit", "err", err.Error())
	} else {
		slog.Info("exit")
	}
	return err
}
