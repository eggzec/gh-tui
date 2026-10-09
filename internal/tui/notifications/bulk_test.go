package notifications

import (
	"errors"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// markRows marks the rows at the offsets from the top of the list, which
// ascend, and leaves the cursor on the last.
func markRows(t *testing.T, s *Section, rows ...int) {
	t.Helper()
	at := 0
	for _, r := range rows {
		for ; at < r; at++ {
			press(t, s, "down")
		}
		press(t, s, "space")
	}
}

func bulkDone(msgs []tea.Msg) (ui.BulkDoneMsg, bool) {
	for _, m := range msgs {
		if b, ok := m.(ui.BulkDoneMsg); ok {
			return b, true
		}
	}
	return ui.BulkDoneMsg{}, false
}

// One question covers every marked thread, the changes are sent once the
// user says yes, each once, with one report, and the marks are gone after.
func TestBulkReadAsksOnce(t *testing.T) {
	svc := newFake(inbox()...)
	s := newSection(t, svc, 100, 20)
	markRows(t, s, 0, 1, 2)
	press(t, s, "U")
	if got, want := question(s), "Mark 3 notifications as read?"; got != want {
		t.Fatalf("asks %q, want %q", got, want)
	}
	if len(svc.reads) != 0 {
		t.Fatalf("read %v before the answer", svc.reads)
	}
	msgs := press(t, s, "y")
	got := slices.Clone(svc.reads)
	slices.Sort(got)
	if want := []string{"1", "2", "3"}; !slices.Equal(got, want) {
		t.Errorf("read %v, want %v", got, want)
	}
	if done, ok := bulkDone(msgs); !ok || len(done.Failed) != 0 {
		t.Errorf("report %+v (found %v), want one without failures", done, ok)
	}
	for _, m := range msgs {
		if _, ok := m.(ui.DoneMsg); ok {
			t.Errorf("a change reported on its own: %v", m)
		}
	}
	if s.feed.Marks() != 0 {
		t.Errorf("%d rows are still marked", s.feed.Marks())
	}
}

// Only the unread ones are marked read, and the rest are counted; every
// one can be marked done.
func TestBulkCountsWhatItSkips(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		svc := newFake(inbox()...)
		s := newSection(t, svc, 100, 20)
		press(t, s, showAll)
		markRows(t, s, 0, 3, 4)
		press(t, s, "U")
		if got, want := question(s), "Mark 1 of 3 marked as read (2 already read)?"; got != want {
			t.Fatalf("asks %q, want %q", got, want)
		}
		press(t, s, "y")
		if want := []string{"1"}; !slices.Equal(svc.reads, want) {
			t.Errorf("read %v, want %v", svc.reads, want)
		}
	})
	t.Run("done", func(t *testing.T) {
		svc := newFake(inbox()...)
		s := newSection(t, svc, 100, 20)
		press(t, s, showAll)
		markRows(t, s, 0, 3)
		press(t, s, "D")
		if got, want := question(s), "Mark 2 notifications as done?"; got != want {
			t.Fatalf("asks %q, want %q", got, want)
		}
		press(t, s, "y")
		got := slices.Clone(svc.dones)
		slices.Sort(got)
		if want := []string{"1", "4"}; !slices.Equal(got, want) {
			t.Errorf("done %v, want %v", got, want)
		}
	})
}

// A change that fails is rolled back and reported by name, and its row
// stays marked.
func TestBulkKeepsMarksOfWhatFailed(t *testing.T) {
	svc := newFake(inbox()...)
	svc.fail = errors.New("boom")
	s := newSection(t, svc, 100, 20)
	markRows(t, s, 0, 1)
	press(t, s, "U")
	msgs := press(t, s, "y")
	done, ok := bulkDone(msgs)
	if !ok || len(done.Failed) != 2 {
		t.Fatalf("report %+v (found %v), want both failed", done, ok)
	}
	if got := done.Failed[0].Item.Name; got != "charmbracelet/bubbletea#1" {
		t.Errorf("the first failure is %q", got)
	}
	if s.feed.Marks() != 2 {
		t.Errorf("%d rows are marked, want the 2 that failed", s.feed.Marks())
	}
}

// With nothing to mark, a warning says why. Marking the whole inbox read
// is not about the marks, and still asks about all of it.
func TestBulkNothingAndReadAll(t *testing.T) {
	svc := newFake(inbox()...)
	s := newSection(t, svc, 100, 20)
	press(t, s, showAll)
	markRows(t, s, 3)
	msgs := press(t, s, "U")
	want := ui.NotifyMsg{Level: toast.Warning, Text: "No marked notification can be marked as read: 1 already read."}
	if !slices.Contains(msgs, tea.Msg(want)) || question(s) != "" {
		t.Errorf("messages %v and question %q, want %v and none", msgs, question(s), want)
	}
	press(t, s, "M")
	if got, want := question(s), "Mark all notifications as read?"; got != want {
		t.Errorf("asks %q, want %q", got, want)
	}
}
