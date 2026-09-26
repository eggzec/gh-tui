package cmdline_test

import (
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
)

func Example() {
	m := cmdline.New(cmdline.WithSize(80, cmdline.MaxHeight))

	// Open it when the user presses ":", in place of the help footer, and
	// forward messages to m.Update while m.Focused(). Lay the screen out
	// around m.Height() rows. Close it on cmdline.SubmitMsg, which carries
	// the command, or cmdline.CancelMsg, with m.ID().
	_ = m.Open("")
}
