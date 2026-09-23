// Command gh-tui is a terminal client for GitHub.
package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := tea.NewProgram(tui.New(ctx, config.Default(), nil)).Run()
	return err
}
