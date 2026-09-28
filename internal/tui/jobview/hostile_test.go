package jobview

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// The path, title and message of an annotation are text from GitHub,
// which the view draws.
func TestViewCleansHostileAnnotations(t *testing.T) {
	h := termtexttest.Hostile
	for _, w := range []int{40, 80, 200} {
		f := newFake()
		note := &f.notes[failedJob][0]
		note.Path, note.Title, note.Message = h, h, h
		m := newView(t, f, w, 20)
		run(m, m.Show(failed(), false, Hints{SHA: "f00d"}))
		v := m.View()
		if !strings.Contains(v, "moved") {
			t.Fatalf("the view doesn't show the annotation: %q", v)
		}
		termtexttest.AssertClean(t, v, w)
	}
}
