package access

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// fakeClient is a client whose token the tests set, and whose probe
// tells the service what learned tells, as the real one's answers do.
type fakeClient struct {
	svc     *Service
	initial core.Access
	learned core.Access
	err     error

	mu     sync.Mutex
	tokens []string
	probes int
}

func (c *fakeClient) Access() core.Access { return c.initial }

func (c *fakeClient) SetToken(token string) {
	c.mu.Lock()
	c.tokens = append(c.tokens, token)
	c.mu.Unlock()
	c.svc.Set(core.Access{Kind: core.TokenClassic})
}

func (c *fakeClient) ProbeAccess(context.Context) error {
	c.mu.Lock()
	c.probes++
	c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.svc.Set(c.learned)
	return nil
}

func (c *fakeClient) counts() (tokens []string, probes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.tokens), c.probes
}

// bound returns a service for tok bound to a fake client.
func bound(tok Token, opts ...Option) (*Service, *fakeClient) {
	s := New("github.com", tok, opts...)
	c := &fakeClient{svc: s}
	s.Bind(c)
	return s, c
}

func classic(scopes ...string) core.Access {
	return core.Access{Kind: core.TokenClassic, Known: true, Scopes: scopes}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		access core.Access
		need   core.Need
		checks bool
		want   error
		grant  string
	}{
		{name: "unknown token", access: core.Access{}, need: core.NeedRuns, checks: true},
		{name: "classic before GitHub said", access: core.Access{Kind: core.TokenClassic}, need: core.NeedRuns, checks: true},
		{name: "classic with the scope", access: classic("gist", "repo"), need: core.NeedRuns, checks: true},
		{name: "classic without it", access: classic("gist"), need: core.NeedRuns, checks: true, want: core.ErrUnauthorized, grant: "repo"},
		{name: "notifications without repo", access: classic("gist"), need: core.NeedNotifications, checks: true, want: core.ErrUnauthorized, grant: "notifications"},
		{name: "checks off", access: classic("gist"), need: core.NeedRuns},
		{name: "fine-grained notifications", access: core.Access{Kind: core.TokenFineGrained}, need: core.NeedNotifications, checks: true, want: core.ErrUnauthorized},
		{name: "fine-grained runs", access: core.Access{Kind: core.TokenFineGrained}, need: core.NeedRuns, checks: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New("github.com", Token{}, WithChecks(tt.checks))
			s.Set(tt.access)
			err := s.Check(tt.need)
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) {
				t.Fatalf("Check = %v, want %v", err, tt.want)
			}
			if err == nil {
				return
			}
			if got := core.Explain("act", err).Grant; got != tt.grant {
				t.Errorf("grant = %q, want %q", got, tt.grant)
			}
			if _, kind := errors.AsType[*core.KindError](err); kind != (tt.grant == "") {
				t.Errorf("Check = %T, want a KindError only when no scope would do", err)
			}
		})
	}
}

// A fine-grained token is known by its prefix alone, since GitHub never
// lists what it may do, so the service starts from what the client knows.
func TestBindStartsFromTheClient(t *testing.T) {
	s := New("github.com", Token{})
	s.Bind(&fakeClient{svc: s, initial: core.Access{Kind: core.TokenFineGrained}})
	if err := s.Check(core.NeedNotifications); !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("Check of notifications with a fine-grained token = %v, want it refused", err)
	}
}

// Set never waits for a reader, which is sent only the latest change.
func TestChangesKeepLatest(t *testing.T) {
	s := New("github.com", Token{})
	slow, idle := s.Changes(), s.Changes()
	const n = 1000
	var wg sync.WaitGroup
	wg.Go(func() {
		for a := range slow {
			if a.Scopes[0] == strconv.Itoa(n-1) {
				return
			}
			time.Sleep(time.Millisecond)
		}
	})
	for i := range n {
		s.Set(classic(strconv.Itoa(i)))
	}
	wg.Wait()
	select {
	case a := <-idle:
		if a.Scopes[0] != strconv.Itoa(n-1) {
			t.Errorf("an idle reader got %v, want the latest", a.Scopes)
		}
	default:
		t.Error("an idle reader got nothing")
	}
	s.Set(classic(strconv.Itoa(n - 1)))
	select {
	case a := <-idle:
		t.Errorf("the same access was sent again: %v", a)
	default:
	}
}

func TestStartProbesWhenSilent(t *testing.T) {
	tests := []struct {
		name   string
		answer time.Duration // when an answer tells, or 0 for never
		checks bool
		probes int
	}{
		{name: "silent", checks: true, probes: 1},
		{name: "answered", answer: time.Second, checks: true},
		{name: "checks off", checks: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, c := bound(Token{}, WithChecks(tt.checks))
				c.learned = classic("repo")
				s.Start(t.Context())
				if tt.answer > 0 {
					time.Sleep(tt.answer)
					s.Set(classic("gist"))
				}
				time.Sleep(quiet - time.Millisecond)
				synctest.Wait()
				if _, probes := c.counts(); probes != 0 {
					t.Fatalf("probed %d times before %v", probes, quiet)
				}
				time.Sleep(time.Minute)
				synctest.Wait()
				if _, probes := c.counts(); probes != tt.probes {
					t.Errorf("probed %d times, want %d", probes, tt.probes)
				}
			})
		})
	}
}
