// Package access knows what the token may do, from GitHub's answers, so
// that what the token lacks a scope for is refused before it is sent, and
// tells the user how to grant it.
package access

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

const span = "service.access"

// quiet is how long the client may go without an answer that tells what
// the token may do, from the start, before Start asks GitHub.
const quiet = 3 * time.Second

// Token is a token and where it came from.
type Token struct {
	Value string
	// Source is where it was found, as auth.TokenForHost names it: an
	// environment variable such as GH_TOKEN, oauth_token for gh's hosts
	// file, or gh for gh's keyring.
	Source string
	// Login is the account gh stores the token for, or "" for a token
	// from elsewhere, which may be anyone's.
	Login string
}

// Lookup finds the token of host again, such as after gh refreshed it.
type Lookup func(host string) Token

// Client is the client whose token the service knows of.
type Client interface {
	Access() core.Access
	SetToken(token string)
	ProbeAccess(ctx context.Context) error
}

// Service holds what the token may do, learns it from GitHub's answers,
// and reads the token again after a refresh. It is safe for concurrent
// use.
type Service struct {
	host   string
	lookup Lookup
	checks bool
	// ghPath finds gh, or returns "" when it isn't installed.
	ghPath func() string

	mu     sync.Mutex
	client Client
	token  Token
	cur    core.Access
	subs   []chan core.Access
	// heard is closed once GitHub answered, which the client tells of
	// whether or not the answer said what the token may do (Set).
	heard     chan struct{}
	heardOnce sync.Once

	// reloading makes Reload one at a time.
	reloading sync.Mutex
}

// Option configures a Service.
type Option func(*Service)

// WithLookup sets how Reload finds the token again.
func WithLookup(l Lookup) Option {
	return func(s *Service) { s.lookup = l }
}

// WithChecks sets whether Check refuses what the token is known to lack,
// and whether Start asks GitHub what it may do. They are on by default.
func WithChecks(on bool) Option {
	return func(s *Service) { s.checks = on }
}

// withGHPath sets how the service finds gh, for tests.
func withGHPath(f func() string) Option {
	return func(s *Service) { s.ghPath = f }
}

// New returns a service for token on host, the host gh names it by, such
// as github.com. Bind gives it the client that sends the token.
func New(host string, token Token, opts ...Option) *Service {
	s := &Service{host: host, token: token, checks: true, ghPath: GHPath, heard: make(chan struct{})}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Found sets the token of a service made while it was still being looked
// up, such as while gh read it from the system keyring, which New would
// have been given: Reload and Refresh go by it. Bind the client once it
// has the token too.
func (s *Service) Found(token Token) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
}

// Bind sets the client that sends the token, which is made after the
// service, since it tells the service what it learns (Set), and starts
// from what the client knows before any answer: the kind of token its
// prefix says, which GitHub may never say otherwise, as of a fine-grained
// token. The subscribers hear of it, since a client bound once the app
// runs, as one whose token gh read meanwhile, tells it what Access said
// nothing of before. It isn't an answer, so Start still asks GitHub what
// a classic token may do.
func (s *Service) Bind(c Client) {
	a := c.Access()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = c
	if s.cur.Equal(core.Access{}) && !a.Equal(core.Access{}) {
		s.cur = a
		s.publish(a)
	}
}

// Set records what the token may do, as the client learned it. It never
// blocks: a subscriber that hasn't read the last change is sent only the
// latest.
func (s *Service) Set(a core.Access) {
	s.heardOnce.Do(func() { close(s.heard) })
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Equal(s.cur) {
		return
	}
	s.cur = a
	s.publish(a)
	slog.Info("token access", "span", span, "kind", a.Kind.String(), "known", a.Known,
		"scopes", strings.Join(a.Scopes, ","), "sso", len(a.SSO))
}

// publish sends a to every subscriber, replacing what one hasn't read.
// s.mu must be held.
func (s *Service) publish(a core.Access) {
	for _, ch := range s.subs {
		select {
		case <-ch:
		default:
		}
		ch <- a
	}
}

// Access returns what the token may do, as far as GitHub said.
func (s *Service) Access() core.Access {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Check returns an error when the token is known not to be allowed what
// needs n: a *core.ScopeError when it lacks a scope, and a *core.KindError
// when no token of its kind may. What isn't known is allowed, since
// GitHub has the last word, and so is everything while checks are off.
func (s *Service) Check(n core.Need) error {
	if !s.checks {
		return nil
	}
	a := s.Access()
	if ok, known := a.Allows(n); ok || !known {
		return nil
	}
	if a.Missing(n) == "" {
		return &core.KindError{Kind: a.Kind}
	}
	return &core.ScopeError{Scopes: slices.Clone(n.Scopes)}
}

// Changes returns a channel that receives what the token may do each time
// that changes, starting with the next change. A reader that falls behind
// receives only the latest. Each call returns a channel of its own.
func (s *Service) Changes() <-chan core.Access {
	ch := make(chan core.Access, 1)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	return ch
}

// Start asks GitHub what the token may do if no answer said so within a
// few seconds, as when every read at the start came from the disk. It
// returns at once, and asks nothing while checks are off.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	c := s.client
	s.mu.Unlock()
	if !s.checks || c == nil {
		return
	}
	go func() {
		t := time.NewTimer(quiet)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return
		case <-s.heard:
			return
		case <-t.C:
		}
		if err := c.ProbeAccess(ctx); err != nil {
			slog.WarnContext(ctx, "access not probed", "span", span, "err", err.Error())
		}
	}()
}

// GHPath returns where gh is, as go-gh finds it: GH_PATH, else gh on the
// PATH, or "" when it isn't installed.
func GHPath() string {
	if p := os.Getenv("GH_PATH"); p != "" {
		return p
	}
	p, err := exec.LookPath("gh")
	if err != nil {
		return ""
	}
	return p
}
