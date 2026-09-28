package notifications

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// keptService serves the first read of each page as an earlier session
// kept it, with "Kept " before each title, or every page offline.
type keptService struct {
	*fakeService
	offline bool

	mu     sync.Mutex
	served map[notifications.ListQuery]bool
}

func (k *keptService) List(ctx context.Context, q notifications.ListQuery) (core.Page[core.Notification], error) {
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
			p.Items[i].Subject.Title = "Kept " + p.Items[i].Subject.Title
		}
	}
	return p, nil
}

// unstarted returns a section over svc that hasn't run Init.
func unstarted(t *testing.T, svc Service) *Section {
	t.Helper()
	s := New(t.Context(), svc, config.Default().Keys, WithNow(func() time.Time { return now }))
	s.SetSize(100, 12)
	s.Focus()
	return s
}

func TestKeptInboxIsShownThenRevalidated(t *testing.T) {
	svc := &keptService{fakeService: newFake(inbox()...), served: map[notifications.ListQuery]bool{}}
	s := unstarted(t, svc)
	run(t, s, s.Init())
	if v := ansi.Strip(s.View()); strings.Contains(v, "Kept") {
		t.Errorf("view after revalidating:\n%s\nwant GitHub's page", v)
	}
	if got := svc.listCount(); got != 2 {
		t.Errorf("lists = %d, want 2: the kept page and its revalidation", got)
	}
}

// TestOfflineShowsKeptInbox checks that an inbox served offline shows
// without a toast: the status bar says the app is offline.
func TestOfflineShowsKeptInbox(t *testing.T) {
	svc := &keptService{fakeService: newFake(inbox()...), offline: true, served: map[notifications.ListQuery]bool{}}
	s := unstarted(t, svc)
	msgs := append(run(t, s, s.Init()), press(t, s, "r")...)
	for _, msg := range msgs {
		if n, ok := msg.(ui.NotifyMsg); ok {
			t.Errorf("toast %q, want none", n.Text)
		}
	}
	if s.feed.Len() == 0 || s.feed.Err() != nil {
		t.Errorf("rows = %d, error %v; want the inbox served offline", s.feed.Len(), s.feed.Err())
	}
}
