package notifications

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// A thread's title and reason are text from GitHub, which the rows draw.
func TestViewCleansHostileThreads(t *testing.T) {
	h := termtexttest.Hostile
	for _, w := range []int{40, 80, 200} {
		n := thread("1", "cli/cli", core.SubjectIssue, h, h, true, time.Hour)
		s := newSection(t, newFake(n), w, 4)
		termtexttest.AssertClean(t, s.View(), w)
	}
}
