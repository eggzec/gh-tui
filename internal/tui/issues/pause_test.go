package issues

import (
	"sync"
	"testing"

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
	svc := newFakeService(sampleIssues(12))
	svc.issues = append(svc.issues, core.Issue{Repo: other, Number: 7, Title: "gh issue view hangs", State: core.StateOpen})
	h := started(t, svc, 80, 20)
	p := &holds{}
	cmd := h.Update(ui.OpenIssueMsg{Repo: other, Number: 7, Pause: p})
	if p.held() != 1 {
		t.Fatal("opening didn't hold the reads ahead of the list it came from")
	}
	run(t, h, cmd)
	if p.held() != 0 {
		t.Error("the reads ahead are still held once the modal loaded")
	}
}
