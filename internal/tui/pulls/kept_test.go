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
// kept it, with "Kept " before each title, or every page offline.
type keptService struct {
	*fakeService
	offline bool

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
	if k.offline {
		p.Offline = true
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

func TestOfflineToastOnce(t *testing.T) {
	svc := &keptService{fakeService: newFakeService(), offline: true, served: map[pulls.ListQuery]bool{}}
	h := newTest(t, svc, 100, 12)
	drain(t, h, h.Update(ui.RepoMsg{Repo: repo}))
	var toasts []string
	for _, msg := range drain(t, h, h.Init()) {
		if n, ok := msg.(ui.NotifyMsg); ok {
			toasts = append(toasts, n.Text)
		}
	}
	for _, msg := range press(t, h, "r") {
		if n, ok := msg.(ui.NotifyMsg); ok {
			toasts = append(toasts, n.Text)
		}
	}
	if len(toasts) != 1 || toasts[0] != ui.OfflineText {
		t.Errorf("toasts = %q, want the offline one once", toasts)
	}
}
