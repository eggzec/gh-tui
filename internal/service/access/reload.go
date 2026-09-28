package access

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// ErrOtherAccount is a token of another account than the session started
// with, which Reload refuses: what is kept is kept per account.
var ErrOtherAccount = errors.New("the token is another account's")

// Reload reads the token from where it came from again, such as after gh
// refreshed it, and has the client send it from now on, unless it is
// another account's. Then it asks GitHub what the token may do, and
// returns that. When the token can't be read again, or is refused, the
// client keeps the one it had.
func (s *Service) Reload(ctx context.Context) (core.Access, error) {
	s.reloading.Lock()
	defer s.reloading.Unlock()
	s.mu.Lock()
	c, cur := s.client, s.token
	s.mu.Unlock()
	if c == nil || s.lookup == nil {
		return s.Access(), errors.New("reload the token: nothing to read it from")
	}
	tok := s.lookup(s.host)
	if tok.Value == "" {
		return s.Access(), fmt.Errorf("reload the token: none for %s: %w", s.host, core.ErrUnauthorized)
	}
	if tok.Value != cur.Value {
		// A token without a login is named by itself, so another one is
		// another account.
		if tok.Login == "" || cur.Login == "" || !strings.EqualFold(tok.Login, cur.Login) {
			return s.Access(), &core.RefusedError{Action: "use the new token", Reason: otherAccount(cur, tok), Err: ErrOtherAccount}
		}
		c.SetToken(tok.Value)
		s.mu.Lock()
		s.token = tok
		s.mu.Unlock()
	}
	if err := c.ProbeAccess(ctx); err != nil {
		return s.Access(), fmt.Errorf("reload the token: %w", err)
	}
	return s.Access(), nil
}

// otherAccount says why the token of next can't replace that of cur.
func otherAccount(cur, next Token) string {
	if next.Login == "" || cur.Login == "" {
		return "the token changed and may be another account's; restart gh-tui to use it"
	}
	return "gh is logged in as " + next.Login + " now, not " + cur.Login + "; restart gh-tui to switch accounts"
}
