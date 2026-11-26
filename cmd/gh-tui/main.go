// Command gh-tui is a terminal client for GitHub.
//
// Usage:
//
//	gh-tui [--debug] [--hostname HOST] [--user USER] [--version]
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
	"github.com/eggzec/gh-tui/internal/target"
)

func main() {
	again, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gh-tui:", err)
		os.Exit(1)
	}
	// run has unwound by now: the log is closed and the images are cleared.
	if again != nil {
		err := restart(*again)
		fmt.Fprintln(os.Stderr, "gh-tui:", err)
		os.Exit(1)
	}
}

// usage prints how to run gh-tui, with its flags spelled as the usage line
// and gh spell them, with two dashes.
func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintln(out, "usage: gh-tui [--debug] [--hostname HOST] [--user USER] [--version]")
	flag.VisitAll(func(f *flag.Flag) {
		arg, text := flag.UnquoteUsage(f)
		if arg != "" {
			arg = " " + arg
		}
		fmt.Fprintf(out, "  --%s%s\n    \t%s\n", f.Name, arg, text)
	})
}

// run runs the program until it quits. It returns the identity to start
// again as when the app quit to switch to it, else nil.
func run() (again *target.Target, err error) {
	debugLog := flag.Bool("debug", false, "log at debug level for this run, as GH_DEBUG does")
	showVersion := flag.Bool("version", false, "print the version and exit")
	hostname := flag.String("hostname", "", "use the GitHub host `HOST`, in place of the current repository's or gh's default")
	user := flag.String("user", "", "sign in as the GitHub login `USER`, in place of the account gh uses on the host")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		return nil, fmt.Errorf("unexpected argument %q: gh-tui takes no arguments; use :goto owner/name inside the app", flag.Arg(0))
	}
	if *hostname != "" {
		if err := checkHostname(*hostname); err != nil {
			return nil, err
		}
	}
	if *user != "" {
		t, err := target.Parse("user "+*user, "", "github.com")
		if err != nil {
			return nil, fmt.Errorf("--user %q is not a GitHub login", *user)
		}
		*user = t.User
	}
	if *showVersion {
		fmt.Println("gh-tui", version())
		return nil, nil
	}

	path, err := config.Path()
	if err != nil {
		return nil, err
	}
	file, err := config.Load(path)
	if err != nil {
		return nil, err
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

	app, found, err := build(ctx, file, cfg.Log.Level, *hostname, *user, warning, config.RenamedWarning(renamed))
	if err != nil {
		slog.Error("start failed", "err", err.Error())
		return nil, err
	}
	// The app deletes the images it sent before the program quits.
	p := tea.NewProgram(app, tea.WithFilter(app.Filter))
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
	// The images are deleted as the app quits, but not when the program
	// ended otherwise, as on a panic; the terminal holds them still.
	if seq := app.ClearImages(); seq != "" {
		_, _ = os.Stdout.WriteString(seq)
	}
	select {
	case err := <-failed:
		slog.Error("start failed", "err", err.Error())
		return nil, err
	default:
	}
	if err != nil {
		slog.Error("exit", "err", err.Error())
		return nil, err
	}
	if t, ok := app.Restart(); ok {
		slog.Info("exit to restart", "to", t.String())
		return &t, nil
	}
	slog.Info("exit")
	return nil, nil
}
