package jobview

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

func TestPositionWaitsForTheLog(t *testing.T) {
	m := newView(t, newFake(), 80, 12)
	cmd := m.Show(failed(), false, Hints{})
	if m.State() != Loading {
		t.Fatalf("state %d, want loading", m.State())
	}
	// The log fits the view, so its window stays at the top.
	want := pager.Position{Cursor: 2}
	m.SetPosition(want)
	if got := m.Position(); got != want {
		t.Errorf("a loading log reports %+v, want the pending %+v", got, want)
	}
	run(m, cmd)
	if m.State() != Ready {
		t.Fatalf("state %d, want ready", m.State())
	}
	if got := m.view.Position(); got != want {
		t.Errorf("the log shows at %+v once ready, want %+v", got, want)
	}

	// Once ready, the position applies at once.
	at := pager.Position{}
	m.SetPosition(at)
	if got := m.view.Position(); got != at {
		t.Errorf("a ready log is at %+v, want %+v", got, at)
	}

	// Another job forgets a position still pending.
	m.Clear()
	m.SetPosition(want)
	m.Clear()
	if m.pos != nil {
		t.Error("clear keeps a pending position")
	}
}
