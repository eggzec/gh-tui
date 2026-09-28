package uitest

import (
	"slices"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Checker knows what a token may do, and checks it as the access service
// does.
type Checker struct {
	A core.Access
}

// Access implements ui.Checker.
func (c *Checker) Access() core.Access { return c.A }

// Check implements ui.Checker.
func (c *Checker) Check(n core.Need) error {
	if ok, known := c.A.Allows(n); ok || !known {
		return nil
	}
	if c.A.Missing(n) == "" {
		return &core.KindError{Kind: c.A.Kind}
	}
	return &core.ScopeError{Scopes: slices.Clone(n.Scopes)}
}

// Classic returns what a classic token with scopes may do, as GitHub
// listed them.
func Classic(scopes ...string) core.Access {
	return core.Access{Kind: core.TokenClassic, Known: true, Scopes: scopes}
}

// Token returns the token that c knows of, whose hints name :auth.
func Token(c *Checker) *ui.Token {
	return ui.NewToken(c, config.Default().Keys)
}
