package notifications

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
	"github.com/eggzec/gh-tui/internal/watch"
)

var (
	// noScope is a classic token that GitHub said lacks notifications
	// and repo.
	noScope = core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"gist"}}
	// withRepo is a classic token that may read notifications.
	withRepo = core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}}
)

// tokenWith returns the access service of a token that GitHub said a
// is, which refuses what a lacks unless checks is false.
func tokenWith(a core.Access, checks bool) *access.Service {
	s := access.New("github.com", access.Token{}, access.WithChecks(checks))
	s.Set(a)
	return s
}

func TestRefusedAsksNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access core.Access
		// want reports whether err is the refusal expected.
		want  func(err error) bool
		grant string
	}{
		{
			name:   "no scope",
			access: noScope,
			want:   func(err error) bool { _, ok := errors.AsType[*core.ScopeError](err); return ok },
			grant:  "notifications",
		},
		{
			name:   "fine-grained",
			access: core.Access{Kind: core.TokenFineGrained},
			want:   func(err error) bool { _, ok := errors.AsType[*core.KindError](err); return ok },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{}
			var marks int
			api.onMark = func() { marks++ }
			s := New(api, WithStore(keptInbox(t)), WithAccess(tokenWith(tc.access, true)))
			s.cache.Set(inbox.key(30), entry(page{Items: []core.Notification{a, b}}, modified1))

			if _, ok := s.CachedList(inbox); ok {
				t.Error("CachedList served a page the token may not read")
			}
			_, err := s.List(t.Context(), inbox)
			if !tc.want(err) {
				t.Errorf("List = %v, want the refusal", err)
			}
			if p := core.Explain("load notifications", err); p.Kind != core.Auth || p.Grant != tc.grant {
				t.Errorf("Explain = %+v, want Auth granting %q", p, tc.grant)
			}
			if res, err := s.Poll(t.Context()); err != nil || res != (watch.Result{}) {
				t.Errorf("Poll = %+v, %v; want no change and no error", res, err)
			}
			if entries := s.Kept(); len(entries) != 0 {
				t.Errorf("Kept = %d entries, want none", len(entries))
			}
			for _, op := range []func() error{
				func() error { return s.MarkRead("a").Do(t.Context()) },
				func() error { return s.MarkDone("a").Do(t.Context()) },
				func() error { return s.MarkAllRead(now).Do(t.Context()) },
			} {
				if err := op(); !tc.want(err) {
					t.Errorf("mark = %v, want the refusal", err)
				}
			}
			if n := api.lists.Load(); n != 0 || marks != 0 {
				t.Errorf("%d lists and %d marks sent, want none", n, marks)
			}
			// The refused marks changed nothing, so allowing them shows
			// the inbox as it was.
			if e, _ := s.cache.Get(inbox.key(30)); !equal(e.Value, page{Items: []core.Notification{a, b}}) {
				t.Errorf("cached inbox = %+v, want it unchanged", e.Value)
			}
		})
	}
}

func TestAllowedAsks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access *access.Service
	}{
		{"unknown", tokenWith(core.Access{}, true)},
		{"classic, scopes not yet known", tokenWith(core.Access{Kind: core.TokenClassic}, true)},
		{"with repo", tokenWith(withRepo, true)},
		{"checks off", tokenWith(noScope, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{list: servePage1, markRead: func(string) error { return nil }}
			s := New(api, WithAccess(tc.access))
			if _, err := s.List(t.Context(), inbox); err != nil {
				t.Fatalf("List: %v", err)
			}
			if err := s.MarkRead("1").Do(t.Context()); err != nil {
				t.Errorf("MarkRead: %v", err)
			}
			if n := api.lists.Load(); n != 1 {
				t.Errorf("%d lists sent, want 1", n)
			}
		})
	}
}

func TestPollPausesWhileRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tokens := tokenWith(noScope, true)
		api := &fakeAPI{list: servePage1}
		s := New(api, WithAccess(tokens))
		e := watch.New(watch.WithInterval(time.Minute))
		e.Subscribe(SyncKey, s.Poll)
		go func() { _ = e.Run(t.Context()) }()

		time.Sleep(10 * time.Minute)
		synctest.Wait()
		if n := api.lists.Load(); n != 0 {
			t.Fatalf("%d polls sent while refused, want none", n)
		}
		tokens.Set(withRepo)
		time.Sleep(time.Minute)
		synctest.Wait()
		if n := api.lists.Load(); n != 1 {
			t.Errorf("%d polls sent a minute after the token may, want 1", n)
		}
	})
}
