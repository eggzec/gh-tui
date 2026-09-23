// Command gh-tui is a terminal client for GitHub.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui"
)

func main() {
	if _, err := tea.NewProgram(tui.New()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh-tui:", err)
		os.Exit(1)
	}
}
