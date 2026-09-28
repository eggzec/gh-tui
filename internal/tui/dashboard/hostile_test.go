package dashboard

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// The profile, work, repositories and notifications are text from GitHub,
// which the dashboard draws.
func TestViewCleansHostileText(t *testing.T) {
	h := termtexttest.Hostile
	for _, w := range []int{80, 190} {
		svc := newFake()
		p := &svc.header.Profile
		p.Name, p.Bio, p.Company, p.Location, p.Status.Message = h, h, h, h, h
		svc.work.ReviewRequested.Items[0].Issue.Title = h
		svc.repos["@me"][0].Description = h
		threads := inboxThreads()
		threads[0].Subject.Title = h
		s := newSection(t, svc, &fakeInbox{threads: threads}, w, 40)
		termtexttest.AssertClean(t, s.View(), w)
	}
}
