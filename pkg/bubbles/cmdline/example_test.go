package cmdline_test

import (
	"strings"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
)

func Example() {
	commands := []string{"goto", "quit", "search", "theme"}
	// Complete offers the commands that start with the first word. It
	// reads only what is in memory, since it runs on every key.
	complete := func(line string, cursor int) []cmdline.Candidate {
		if strings.Contains(line[:cursor], " ") {
			return nil
		}
		var out []cmdline.Candidate
		for _, c := range commands {
			if strings.HasPrefix(c, line[:cursor]) {
				out = append(out, cmdline.Candidate{Text: c, Start: 0, End: cursor})
			}
		}
		return out
	}
	m := cmdline.New(100,
		cmdline.WithSize(80, cmdline.MaxHeight),
		cmdline.WithComplete(complete),
	)

	// Open it when the user presses ":", in place of the help footer, and
	// forward messages to m.Update while m.Focused(). Lay the screen out
	// around m.Height() rows. Close it on cmdline.SubmitMsg, which carries
	// the command, or cmdline.CancelMsg, with m.ID().
	_ = m.Open("")
}
