package pulls

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// keptService serves the first read of each page as an earlier session
// kept it, with "Kept " before each title, or every page offline, or
// rate limited.
type keptService struct {
	*fakeService
	offline, limited bool

	mu     sync.Mutex
	served map[pulls.ListQuery]bool
}

func (k *keptService) List(ctx context.Context, q pulls.ListQuery) (core.Page[core.PullRequest], error) {
	// The kept page is served until a read with Again set.
	again := q.Again
	q.Again = false
	p, err := k.fakeService.List(ctx, q)
	if err != nil {
		return p, err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if again {
		k.served[q] = true
	}
	if k.offline || k.limited {
		// What GitHub sends next is no older than this.
		k.served[q], p.Offline, p.Limited = true, k.offline, k.limited
		return p, nil
	}
	if !k.served[q] {
		k.served[q], p.Stale = true, true
		for i := range p.Items {
			p.Items[i].Title = "Kept " + p.Items[i].Title
		}
	}
	return p, nil
}

func TestKeptPageIsShownThenRefetched(t *testing.T) {
	svc := &keptService{fakeService: newFakeService(), served: map[pulls.ListQuery]bool{}}
	h := started(t, svc, 100, 12)
	if v := ansi.Strip(h.View()); strings.Contains(v, "Kept") || !strings.Contains(v, "disk layer") {
		t.Errorf("view after refetching:\n%s\nwant GitHub's page", v)
	}
	if got := len(svc.listed()); got != 2 {
		t.Errorf("lists = %d, want 2: the kept page and its refetch", got)
	}
}

// TestOfflineShowsKeptRows checks that rows served offline show without
// a toast: the status bar says the app is offline.
func TestOfflineShowsKeptRows(t *testing.T) {
	svc := &keptService{fakeService: newFakeService(), offline: true, served: map[pulls.ListQuery]bool{}}
	h := newTest(t, svc, 100, 12)
	drain(t, h, h.Update(ui.RepoMsg{Repo: repo}))
	msgs := append(drain(t, h, h.Init()), press(t, h, "r")...)
	for _, msg := range msgs {
		if n, ok := msg.(ui.NotifyMsg); ok {
			t.Errorf("toast %q, want none", n.Text)
		}
	}
	if h.feed.Len() == 0 || h.feed.Err() != nil {
		t.Errorf("rows = %d, error %v; want the rows served offline", h.feed.Len(), h.feed.Err())
	}
}

// TestKeptRowsReadAgainOnline checks that rows served offline, or rate
// limited, are read again once GitHub answers or the limit lifts, once.
func TestKeptRowsReadAgainOnline(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{false: "offline", true: "limited"}[limited], func(t *testing.T) {
			svc := &keptService{fakeService: newFakeService(), offline: !limited, limited: limited, served: map[pulls.ListQuery]bool{}}
			h := started(t, svc, 100, 12)
			svc.mu.Lock()
			svc.offline, svc.limited = false, false
			svc.mu.Unlock()

			n := len(svc.listed())
			drain(t, h, h.Update(ui.OnlineMsg{}))
			drain(t, h, h.Update(ui.OnlineMsg{}))
			if got := len(svc.listed()) - n; got != 1 {
				t.Errorf("lists read after two OnlineMsg = %d, want 1", got)
			}
		})
	}
}
