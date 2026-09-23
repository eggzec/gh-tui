// Command gh-tui is a terminal client for GitHub.
//
// Usage:
//
//	gh-tui [owner/name]
//
// Without a repository it uses the one in the current directory, then the
// first pinned repository in the config.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: gh-tui [owner/name]")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app, err := build(ctx, cfg, flag.Arg(0))
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app).Run()
	return err
}
