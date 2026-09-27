package pulls

import (
	"errors"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// holds counts the pauses that hold it.
type holds struct {
	mu sync.Mutex
	n  int
}

func (h *holds) Pause() func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.n++
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.n--
		})
	}
}

func (h *holds) held() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.n
}

// The modal holds the reads ahead of the list it was opened from, such as
// the dashboard's, until it has loaded.
func TestOpenHoldsTheListItCameFrom(t *testing.T) {
	other := core.RepoRef{Owner: "cli", Name: "cli"}
	svc := newFakeService()
	svc.pulls = append(svc.pulls, core.PullRequest{Repo: other, Number: 7, Title: "Speed up gh pr list", State: core.StateOpen})
	h := started(t, svc, 80, 30)
	p := &holds{}
	cmd := h.Update(ui.OpenPullMsg{Repo: other, Number: 7, Pause: p})
	if p.held() != 1 {
		t.Fatal("opening didn't hold the reads ahead of the list it came from")
	}
	drain(t, h, cmd)
	if p.held() != 0 {
		t.Error("the reads ahead are still held once the modal loaded")
	}
}

// Whatever becomes of the modal, the list it came from is held until the
// modal loads or closes, and is never left held.
func TestOpenReleasesTheListItCameFrom(t *testing.T) {
	other := core.RepoRef{Owner: "cli", Name: "cli"}
	// open opens number over h, holding p, and returns its two steps:
	// the modal, then its loads.
	open := func(t *testing.T, h *host, number int, p ui.Pauser) (show, load tea.Cmd) {
		t.Helper()
		seq, ok := sequence(h.Update(ui.OpenPullMsg{Repo: other, Number: number, Pause: p})())
		if !ok || len(seq) != 2 {
			t.Fatal("opening should show the modal, then start its loads")
		}
		return seq[0], seq[1]
	}
	tests := []struct {
		name string
		run  func(t *testing.T, h *host, svc *fakeService, p *holds)
	}{
		{"the detail fails to load", func(t *testing.T, h *host, svc *fakeService, p *holds) {
			t.Helper()
			svc.getErr = errors.New("boom")
			drain(t, h, h.Update(ui.OpenPullMsg{Repo: other, Number: 7, Pause: p}))
		}},
		{"closed before it loads", func(t *testing.T, h *host, _ *fakeService, p *holds) {
			t.Helper()
			show, load := open(t, h, 7, p)
			drain(t, h, show)
			press(t, h, "esc")
			if p.held() != 0 {
				t.Error("closing before the load left the list held")
			}
			drain(t, h, load)
		}},
		{"replaced before it loads, both loading late", func(t *testing.T, h *host, _ *fakeService, p *holds) {
			t.Helper()
			show, load := open(t, h, 7, p)
			drain(t, h, show)
			show2, load2 := open(t, h, 8, p)
			drain(t, h, show2)
			if p.held() != 2 {
				t.Errorf("held by %d modals, want 2", p.held())
			}
			drain(t, h, load2)
			drain(t, h, load)
		}},
		{"refreshed, then closed", func(t *testing.T, h *host, _ *fakeService, p *holds) {
			t.Helper()
			drain(t, h, h.Update(ui.OpenPullMsg{Repo: other, Number: 7, Pause: p}))
			press(t, h, "r")
			press(t, h, "esc")
		}},
		{"without a list to hold", func(t *testing.T, h *host, _ *fakeService, _ *holds) {
			t.Helper()
			drain(t, h, h.Update(ui.OpenPullMsg{Repo: other, Number: 7}))
			press(t, h, "esc")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			svc.pulls = append(svc.pulls,
				core.PullRequest{Repo: other, Number: 7, Title: "Speed up gh pr list", State: core.StateOpen},
				core.PullRequest{Repo: other, Number: 8, Title: "Speed up gh pr view", State: core.StateOpen})
			h := started(t, svc, 80, 30, WithPrefetch(3, 0))
			p := &holds{}
			tt.run(t, h, svc, p)
			if p.held() != 0 {
				t.Errorf("the list is held by %d modals at the end, want none", p.held())
			}
		})
	}
}
