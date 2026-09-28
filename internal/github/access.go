package github

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
)

// WithOnAccess sets a function that is told what the token may do each
// time an answer of GitHub changes that, and once the token is replaced
// (Client.SetToken). It is called from the goroutine of the request, one
// call at a time, and must not block. It may call SetToken, whose change
// it is told of once it returns.
func WithOnAccess(f func(core.Access)) Option {
	return func(o *options) { o.onAccess = f }
}

// Access returns what the token may do, as GitHub last said, or as its
// prefix says before GitHub did.
func (c *Client) Access() core.Access {
	return c.access.get()
}

// SetToken replaces the token that requests are sent with, such as one
// that gh refreshed with more scopes. Requests sent already keep the old
// one. What the token may do is unknown again until GitHub answers, and
// Account stays the same, so that what is kept for the account stays
// where it is.
func (c *Client) SetToken(token string) {
	c.token.Store(&token)
	c.access.reset(token)
}

// ProbeAccess asks GitHub for the rate limits, which costs nothing
// against them, so that its answer tells what the token may do. Any
// answer will do, as a 404 from a server that doesn't limit the rate; it
// fails only when GitHub doesn't answer, or rejects the token.
func (c *Client) ProbeAccess(ctx context.Context) error {
	_, err := c.rateLimits(ctx)
	if _, answered := errors.AsType[*Error](err); answered && !errors.Is(err, core.ErrUnauthorized) {
		return nil
	}
	return err
}

// tokenAccess keeps what the token may do from the headers of GitHub's
// answers. It is safe for concurrent use.
type tokenAccess struct {
	// host is the API host, with its port if it has one: only its
	// answers say what the token may do.
	host     string
	onAccess func(core.Access)

	mu  sync.Mutex
	cur core.Access
	// token is the token that requests are sent with, since an answer to
	// one sent with the token before says nothing of it, and kind is the
	// kind its prefix says.
	token string
	kind  core.TokenKind
	// scopes and sso are the headers cur was made of, so that an answer
	// with the same ones, as nearly every answer is, is not parsed.
	scopes, sso string
	seen        bool
	// version counts the changes of cur, delivered is the one onAccess
	// was last called with, and telling is set while it is called.
	version, delivered uint64
	telling            bool
}

func newTokenAccess(host, token string, onAccess func(core.Access)) *tokenAccess {
	a := &tokenAccess{host: host, onAccess: onAccess}
	a.forget(token)
	return a
}

// forget sets what the token may do to what its prefix says. a.mu must
// be held, unless a is new.
func (a *tokenAccess) forget(token string) {
	a.token, a.kind = token, tokenKind(token)
	a.cur = core.Access{Kind: a.kind}
	a.scopes, a.sso, a.seen = "", "", false
}

func (a *tokenAccess) get() core.Access {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cur
}

// reset forgets what the token may do, for a new token.
func (a *tokenAccess) reset(token string) {
	a.mu.Lock()
	a.forget(token)
	a.version++
	a.mu.Unlock()
	a.tell()
}

// observe learns what the token may do from the headers h of the answer
// to req. An answer without X-OAuth-Scopes, as a fine-grained token gets,
// one from another host, as a download from storage is, or one to a
// request sent with a token SetToken replaced, changes nothing. Only a
// classic token is told its scopes, and when they are there, the token
// is classic whatever its prefix says, unless they are empty and its
// prefix is of another kind, which may be how a proxy passes a header on.
func (a *tokenAccess) observe(req *http.Request, h http.Header) {
	if a == nil || req.URL.Host != a.host {
		return
	}
	if c, _ := req.Context().Value(callKey{}).(*call); c != nil && c.external {
		return
	}
	granted, ok := h[http.CanonicalHeaderKey("X-OAuth-Scopes")]
	sso := partialSSO(h.Values("X-GitHub-SSO"))
	if !ok && sso == "" {
		return
	}
	scopes := strings.Join(granted, ",")
	sent, _ := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	a.mu.Lock()
	if sent != a.token {
		a.mu.Unlock()
		return
	}
	if scopes == "" && a.kind != core.TokenUnknown && a.kind != core.TokenClassic {
		ok = false
	}
	next := a.cur
	if ok && (!a.seen || scopes != a.scopes) {
		next.Kind, next.Known, next.Scopes = core.TokenClassic, true, core.ParseScopes(scopes)
		a.scopes, a.seen = scopes, true
	}
	if sso != "" && sso != a.sso {
		next.SSO = orgIDs(sso)
		a.sso = sso
	}
	changed := !next.Equal(a.cur)
	if changed {
		a.cur = next
		a.version++
	}
	a.mu.Unlock()
	if changed {
		a.tell()
	}
}

// tell calls onAccess with what the token may do, unless it was called
// with that already. Answers arrive at once, so each call tells the
// latest, and never one older than it told before. While onAccess is
// being called, tell leaves the change to that call, which tells it once
// onAccess returns, so onAccess may itself change what it is told.
func (a *tokenAccess) tell() {
	if a.onAccess == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.telling {
		return
	}
	a.telling = true
	for a.delivered != a.version {
		cur := a.cur
		a.delivered = a.version
		a.mu.Unlock()
		a.onAccess(cur)
		a.mu.Lock()
	}
	a.telling = false
}

// partialSSO returns the organizations of an X-GitHub-SSO of
// partial-results, such as "partial-results; organizations=21955855,
// 20582480", which a list sends when some organizations left their
// results out, or "".
func partialSSO(values []string) string {
	for _, v := range values {
		kind, params, _ := strings.Cut(v, ";")
		if strings.TrimSpace(kind) != "partial-results" {
			continue
		}
		if orgs, ok := ssoParam(params, "organizations"); ok {
			return orgs
		}
	}
	return ""
}

// ssoParam returns the value of the parameter name of an X-GitHub-SSO,
// whose params follow its kind. Each kind has one parameter, and a URL may
// hold a semicolon, so params are not split.
func ssoParam(params, name string) (string, bool) {
	params = strings.TrimSpace(params)
	v, ok := strings.CutPrefix(params, name+"=")
	return strings.TrimSpace(v), ok && strings.TrimSpace(v) != ""
}

// orgIDs splits a list of organization ids, sorted.
func orgIDs(list string) []string {
	var ids []string
	for id := range strings.SplitSeq(list, ",") {
		if id = strings.TrimSpace(id); id != "" && strings.Trim(id, "0123456789") == "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// tokenKind returns the kind of token its prefix says it is. gh's OAuth
// tokens (gho_), classic personal access tokens (ghp_) and the 40 hex
// digits of older ones, as older Enterprise Servers issue, are classic.
func tokenKind(token string) core.TokenKind {
	switch {
	case strings.HasPrefix(token, "gho_"), strings.HasPrefix(token, "ghp_"):
		return core.TokenClassic
	case strings.HasPrefix(token, "github_pat_"):
		return core.TokenFineGrained
	case strings.HasPrefix(token, "ghu_"), strings.HasPrefix(token, "ghs_"):
		return core.TokenApp
	case len(token) == 40 && strings.Trim(strings.ToLower(token), "0123456789abcdef") == "":
		return core.TokenClassic
	}
	return core.TokenUnknown
}
