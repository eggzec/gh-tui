package notifications

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A row links its title to the thread's page, whatever its width, read or
// unread.
func TestRowLinks(t *testing.T) {
	s := newSection(t, newFake(), 100, 10)
	for _, unread := range []bool{false, true} {
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			n := core.Notification{Repo: core.RepoRef{Owner: "o", Name: "r"}, Unread: unread, Subject: core.Subject{
				Title: title, Type: core.SubjectPullRequest, WebURL: ui.WebURL(host, "o/r/pull/7"),
			}}
			return s.render(n, true, width), n.Subject.WebURL
		}, 120, 80, 40, 20, 8, 1)
	}
}
