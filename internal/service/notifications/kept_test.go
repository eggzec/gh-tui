package notifications

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func openStore(t *testing.T) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// keptInbox returns a store that a session which listed the inbox from
// servePage1 left behind.
func keptInbox(t *testing.T) *disk.Store {
	t.Helper()
	store := openStore(t)
	if _, err := New(&fakeAPI{list: servePage1}, WithStore(cachetest.Aged(store, time.Hour))).List(t.Context(), inbox); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestKeptInboxIsServedStaleThenRevalidated(t *testing.T) {
	store := keptInbox(t)
	var conds []github.Conditional
	api := &fakeAPI{list: func(f core.NotificationFilter, n int, cursor string, cond github.Conditional) (page, github.Response, error) {
		conds = append(conds, cond)
		return servePage1(f, n, cursor, cond)
	}}
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.List(t.Context(), inbox)
	if err != nil || !p.Stale || !equal(p, page1) {
		t.Fatalf("List in a new session = %+v, %v; want the kept page, stale", p, err)
	}
	if got, ok := s.CachedList(inbox); !ok || !equal(got, page1) {
		t.Errorf("CachedList = %+v, %v; want the kept page for the badge", got, ok)
	}
	p, err = s.List(t.Context(), inbox)
	if err != nil || p.Stale || !equal(p, page1) {
		t.Fatalf("second List = %+v, %v; want the page revalidated", p, err)
	}
	if len(conds) != 1 || conds[0].LastModified != modified1 {
		t.Errorf("requests = %+v, want one with the kept Last-Modified", conds)
	}
}

func TestKeptInboxPollIsConditional(t *testing.T) {
	store := keptInbox(t)
	api := &fakeAPI{list: servePage1}
	res, err := New(api, WithStore(cachetest.Aged(store, time.Hour))).Poll(t.Context())
	if err != nil || res.Changed {
		t.Errorf("Poll = %+v, %v; want no change, as the kept page is current", res, err)
	}
}

func TestKeptInboxOffline(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback bool
	}{
		{"unreachable", &url.Error{Op: "Get", URL: "https://api.github.com/notifications", Err: errors.New("refused")}, true},
		{"server error", &github.Error{StatusCode: 503}, true},
		{"unauthorized", &github.Error{StatusCode: 401}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := keptInbox(t)
			api := &fakeAPI{list: func(core.NotificationFilter, int, string, github.Conditional) (page, github.Response, error) {
				return page{}, github.Response{}, tt.err
			}}
			s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
			_, _ = s.List(t.Context(), inbox)
			p, err := s.List(t.Context(), inbox)
			if !tt.fallback {
				if err == nil {
					t.Fatalf("List = %+v, want the error", p)
				}
				if p, _ := New(api, WithStore(cachetest.Aged(store, time.Hour))).List(t.Context(), inbox); p.Stale {
					t.Error("List after a refusal = stale, want the kept page gone")
				}
				return
			}
			if err != nil || !p.Offline || !equal(p, page1) {
				t.Errorf("List = %+v, %v; want the kept page, offline", p, err)
			}
		})
	}
}

func TestKeptInboxMark(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		store := keptInbox(t)
		api := &fakeAPI{list: servePage1, markRead: func(string) error {
			if confirm {
				return nil
			}
			return errFailed
		}}
		s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
		_, _ = s.List(t.Context(), inbox)
		_, _ = s.List(t.Context(), inbox)
		_ = s.MarkRead("1").Do(t.Context())

		e, ok := New(api, WithStore(cachetest.Aged(store, time.Hour))).kept.Load(inbox.key())
		if !ok || e.Value.Items[0].Unread == confirm {
			t.Errorf("confirmed=%v: kept page = %+v, %v; want unread %v", confirm, e.Value, ok, !confirm)
		}
		if confirm && e.LastModified != "" {
			t.Errorf("kept page after a change has Last-Modified %q, want none", e.LastModified)
		}
		if !confirm && e.LastModified != modified1 {
			t.Errorf("kept page after a rollback has Last-Modified %q, want GitHub's", e.LastModified)
		}
	}
}
