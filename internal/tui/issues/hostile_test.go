package issues

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// An issue's title and labels are text from GitHub, which the rows and
// the head of the detail draw.
func TestViewCleansHostileIssues(t *testing.T) {
	h := termtexttest.Hostile
	for _, w := range []int{40, 80, 200} {
		issues := sampleIssues(3)
		issues[0].Title = h
		issues[0].Labels = []core.Label{{Name: h, Color: "d73a4a"}, {Name: h}}
		s := started(t, newFakeService(issues), w, 10)
		termtexttest.AssertClean(t, s.View(), w)
		press(t, s, "enter")
		m := s.modal()
		m.SetSize(w, 20)
		v := m.View()
		if !strings.Contains(v, "moved") {
			t.Fatalf("the detail doesn't show the title: %q", v)
		}
		termtexttest.AssertClean(t, v, w)
		termtexttest.AssertClean(t, m.Title(), 1000)
	}
}
