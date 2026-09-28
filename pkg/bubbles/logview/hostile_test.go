package logview

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileTitles(t *testing.T) {
	h := termtexttest.Hostile
	m := New(WithSize(60, 5))
	m.Focus()
	m.SetTitle(h)
	m.SetLines([]Line{{Text: "x"}}, []Section{{Title: h, Start: 0, End: 1}})
	termtexttest.AssertClean(t, m.View(), 60)
}

// TestViewCleansHostileLines shows a line of a log that a workflow echoed,
// which anyone who opens a pull request can write.
func TestViewCleansHostileLines(t *testing.T) {
	m := New(WithSize(200, 5))
	m.Focus()
	m.SetLines([]Line{{Text: termtexttest.Hostile}}, nil)
	termtexttest.AssertClean(t, m.View(), 200)
}
