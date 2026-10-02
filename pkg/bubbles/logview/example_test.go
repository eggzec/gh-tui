package logview_test

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

func Example() {
	// The caller parses the runner's markers into kinds, and takes the
	// steps from the jobs API.
	lines := []logview.Line{
		{Text: "Current runner version: '2.337.0'"},
		{Kind: logview.Group, Text: "Run go test ./..."},
		{Text: "\x1b[36;1mgo test ./...\x1b[0m"},
		{Kind: logview.EndGroup},
		{Text: "--- FAIL: TestClipboard (0.00s)"},
		{Kind: logview.Error, Text: "Process completed with exit code 1."},
	}
	steps := []logview.Section{
		{Title: "Set up job", Start: 0, End: 1, Duration: time.Second},
		{Title: "Run go test ./...", Start: 1, End: 6, Failed: true, Duration: 2 * time.Second},
	}

	v := logview.New(logview.WithSize(50, 6), logview.WithFocusFailed(true))
	v.SetTitle("build (windows-latest)")
	v.Focus()
	v.SetLines(lines, steps)
	printPlain(v.View())

	// space folds the step the cursor is in.
	v, _ = v.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	printPlain(v.View())
	// Output:
	//      ▸ Set up job                               1s
	//    ✗ ▾ Run go test ./...                        2s
	//  2     ▸ Run go test ./...
	//  5     --- FAIL: TestClipboard (0.00s)
	// ▌6 ✗   Process completed with exit code 1.
	// build (windows-latest)    error 1/1  row 5/5  100%
	//      ▸ Set up job                               1s
	// ▌  ✗ ▸ Run go test ./...                        2s
	// build (windows-latest)    error 1/1  row 2/2  100%
}

// printPlain prints a view without its styles, trailing blanks and empty
// rows.
func printPlain(v string) {
	for l := range strings.SplitSeq(ansi.Strip(v), "\n") {
		if l = strings.TrimRight(l, " "); l != "" {
			fmt.Println(l)
		}
	}
}
