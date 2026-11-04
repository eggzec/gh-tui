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
		issues[0].Author.Login = termtexttest.HostileLogin
		issues[0].Assignees = []core.User{{Login: termtexttest.HostileLogin}}
		s := started(t, newFakeService(issues), w, 10)
		termtexttest.AssertClean(t, s.View(), w)
		// The column cuts the login.
		if w == 200 && !strings.Contains(s.View(), "malicio") {
			t.Errorf("the rows don't show the author cleaned: %q", s.View())
		}
		press(t, s, "enter")
		m := s.modal()
		m.SetSize(w, 20)
		v := m.View()
		termtexttest.AssertClean(t, v, w)
	}
}
