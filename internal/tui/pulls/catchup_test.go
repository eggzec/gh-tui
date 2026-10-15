package pulls

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/visits"
)

// lastSeen is a store whose visit to #142 was two days before clock, when the
// head was head.
func lastSeen(head string) *memVisitStore {
	return &memVisitStore{kept: map[string]visits.State{
		visits.Key(repo.String(), 142): {LastSeen: visits.LastSeen{Head: head, At: clock.Add(-48 * time.Hour)}},
	}}
}

// catching opens #142, which is at head a1b2c3d, over store and the service
// that setup changes, at the given width, with the conversation loaded.
func catching(t *testing.T, store *memVisitStore, width int, setup func(*fakeService)) (*host, *detailModal, *fakeService) {
	t.Helper()
	svc := newFakeService()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	// One comment from before the visit, so that nothing is new unless a test adds it.
	svc.thread = []core.Comment{{ID: "0", Author: core.User{Login: "bob"}, Body: "old", CreatedAt: clock.Add(-72 * time.Hour)}}
	if setup != nil {
		setup(svc)
	}
	h := started(t, svc, width, 40, WithVisits(store), WithClock(func() time.Time { return clock }))
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter opened no pull request")
	}
	m.viewer = "me"
	drain(t, h, m.switchTo(conversationTab))
	drain(t, h, m.switchTo(overviewTab))
	return h, m, svc
}

func aheadBy(n int) func(*fakeService) {
	return func(svc *fakeService) {
		svc.sinceCmp = map[string]core.Compare{"old": {Status: core.CompareAhead, AheadBy: n}}
	}
}

func hasCatchRow(m *detailModal, subject string) bool {
	rows := m.attentionRows()
	for i := range rows {
		if strings.Contains(rows[i].subject, subject) {
			return true
		}
	}
	return false
}

func TestCatchupSaysCommitsAndCommentsSinceTheLastVisit(t *testing.T) {
	t.Parallel()
	_, m, svc := catching(t, lastSeen("old"), 100, func(svc *fakeService) {
		aheadBy(3)(svc)
		svc.thread = []core.Comment{{ID: "1", Author: core.User{Login: "alice"}, Body: "fresh", CreatedAt: clock.Add(-time.Hour)}}
	})
	if got := ansi.Strip(m.strip(false)); !strings.Contains(got, "↻ 3 commits · 1 comment since 2d") {
		t.Errorf("strip = %q, want the catch-up item", got)
	}
	if !hasCatchRow(m, "3 new commits since you looked") {
		t.Errorf("no row for the new commits in %+v", m.attentionRows())
	}
	if n := len(svc.sinceReads); n != 1 {
		t.Fatalf("Since read %d times, want once", n)
	}
	if q := svc.sinceReads[0]; q.Base != "old" || q.Head != "a1b2c3d" {
		t.Errorf("Since asked %+v, want old...a1b2c3d", q)
	}
}

func TestCatchupRowLeadsToFiles(t *testing.T) {
	t.Parallel()
	h, m, _ := catching(t, lastSeen("old"), 100, aheadBy(2))
	rows := m.attentionRows()
	if len(rows) == 0 || rows[0].kind != attSince {
		t.Fatalf("rows = %+v, want the catch-up row first", rows)
	}
	for onRow(m) != rows[0].subject {
		before := m.cursorRow(rows)
		press(t, h, "k")
		if m.cursorRow(m.attentionRows()) == before {
			t.Fatalf("the cursor stays on %q, want the catch-up row", onRow(m))
		}
	}
	press(t, h, "enter")
	if m.tab != filesTab {
		t.Errorf("enter on the row went to tab %v, want Files", m.tab)
	}
}

func TestCatchupSaysRewrittenWhenTheOldHeadIsGone(t *testing.T) {
	t.Parallel()
	_, m, _ := catching(t, lastSeen("old"), 100, func(svc *fakeService) {
		svc.sinceErr = fmt.Errorf("compare: %w", core.ErrRewritten)
	})
	if got := ansi.Strip(m.strip(false)); !strings.Contains(got, "↻ rewritten since 2d") {
		t.Errorf("strip = %q, want rewritten", got)
	}
	if !hasCatchRow(m, "Rewritten since you looked") {
		t.Errorf("no row for the rewrite in %+v", m.attentionRows())
	}
}

func TestCatchupShowsNothingForTheSameHeadOrOtherErrors(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		store *memVisitStore
		setup func(*fakeService)
		reads bool
	}{
		"same head":       {lastSeen("a1b2c3d"), aheadBy(3), false},
		"no visit":        {&memVisitStore{}, aheadBy(3), false},
		"other error":     {lastSeen("old"), func(svc *fakeService) { svc.sinceErr = errors.New("boom") }, true},
		"nothing ahead":   {lastSeen("old"), nil, true},
		"identical heads": {lastSeen("old"), func(svc *fakeService) { svc.sinceCmp = map[string]core.Compare{"old": {Status: core.CompareIdentical}} }, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, m, svc := catching(t, tt.store, 100, tt.setup)
			if got := ansi.Strip(m.strip(false)); strings.Contains(got, "since") {
				t.Errorf("strip = %q, want no catch-up item", got)
			}
			if hasCatchRow(m, "since you looked") {
				t.Errorf("rows = %+v, want no catch-up row", m.attentionRows())
			}
			if got := len(svc.sinceReads) > 0; got != tt.reads {
				t.Errorf("Since read = %v, want %v", got, tt.reads)
			}
		})
	}
}

func TestCatchupCountsCommentsAloneWhenTheHeadDidNotMove(t *testing.T) {
	t.Parallel()
	_, m, svc := catching(t, lastSeen("a1b2c3d"), 100, func(svc *fakeService) {
		svc.thread = []core.Comment{{ID: "1", Author: core.User{Login: "alice"}, Body: "fresh", CreatedAt: clock.Add(-time.Hour)}}
	})
	if got := ansi.Strip(m.strip(false)); !strings.Contains(got, "↻ 1 comment since 2d") {
		t.Errorf("strip = %q, want the comment alone", got)
	}
	if hasCatchRow(m, "since you looked") {
		t.Errorf("rows = %+v, want no commits row", m.attentionRows())
	}
	if len(svc.sinceReads) != 0 {
		t.Errorf("Since read with the same head: %+v", svc.sinceReads)
	}
}

func TestCatchupFitsEightyColumns(t *testing.T) {
	t.Parallel()
	_, m, _ := catching(t, lastSeen("old"), 80, func(svc *fakeService) {
		aheadBy(12)(svc)
		svc.thread = []core.Comment{{ID: "1", Author: core.User{Login: "alice"}, Body: "fresh", CreatedAt: clock.Add(-time.Hour)}}
	})
	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("line %d is %d columns wide: %q", i, w, ansi.Strip(line))
		}
	}
}
