package pulls

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// A pull request's title, labels and refs are text from GitHub, which
// the rows and the head of the detail draw.
func TestViewCleansHostilePulls(t *testing.T) {
	h := termtexttest.Hostile
	for _, w := range []int{40, 80, 200} {
		svc := newFakeService()
		pr := &svc.pulls[0]
		pr.Title, pr.HeadRef, pr.BaseRef = h, h, h
		pr.Labels = []core.Label{{Name: h}, {Name: h}}
		s := started(t, svc, w, 10)
		termtexttest.AssertClean(t, s.View(), w)
		press(t, s, "enter")
		m := s.modal()
		m.SetSize(w, 20)
		v := m.View()
		if !strings.Contains(v, "moved") {
			t.Fatalf("the detail doesn't show the title: %q", v)
		}
		termtexttest.AssertClean(t, v, w)
	}
}
