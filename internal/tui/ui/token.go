package ui

import (
	"errors"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// AuthCommand is the name of the command that shows what the token may do
// and grants it what it lacks. The app's command table and the hints that
// point to it share it.
const AuthCommand = "auth"

// Checker knows what the token may do, as the access service does.
type Checker interface {
	// Access returns what the token may do, as far as GitHub said.
	Access() core.Access
	// Check returns a *core.ScopeError or a *core.KindError when the
	// token is known not to be allowed what needs n, else nil.
	Check(n core.Need) error
}

// Token tells the gates and the words of the app what the token may do,
// and how the user grants it more. A nil *Token knows nothing, and allows
// everything, as GitHub has the last word.
type Token struct {
	src  Checker
	hint string
}

// NewToken returns the token that src knows of, whose hints name the
// command AuthCommand on the command key of keys.
func NewToken(src Checker, keys map[string][]string) *Token {
	return &Token{src: src, hint: AuthHint(keys)}
}

// AuthHint returns how the user types the command AuthCommand with the
// command key of keys, such as ":auth", or "" while no key opens the
// command line.
func AuthHint(keys map[string][]string) string {
	k := Binding(keys, config.ActionCommand, "").Help().Key
	if k == "" {
		return ""
	}
	return k + AuthCommand
}

// Access returns what the token may do, or the zero Access, which allows
// everything, when t is nil.
func (t *Token) Access() core.Access {
	if t == nil || t.src == nil {
		return core.Access{}
	}
	return t.src.Access()
}

// Check returns an error when the token is known not to be allowed what
// needs n: a *core.ScopeError when it lacks a scope, and a *core.KindError
// when no token of its kind may. It returns nil when t is nil.
func (t *Token) Check(n core.Need) error {
	if t == nil || t.src == nil {
		return nil
	}
	return t.src.Check(n)
}

// Hint returns how the user types the command that grants the token what
// it lacks, such as ":auth", or "" when t is nil or no key opens the
// command line.
func (t *Token) Hint() string {
	if t == nil {
		return ""
	}
	return t.hint
}

// refusal says why the token may not do what gerund names, such as
// "Merging", which err, from Check, refused: "Merging needs the repo scope
// · :auth to grant it".
func (t *Token) refusal(gerund string, err error, ic Icons) string {
	var why, do string
	if e, ok := errors.AsType[*core.KindError](err); ok {
		why, do = gerund+" needs a classic token, not "+kindArticle(e.Kind), " to see how"
	} else {
		why, do = gerund+" needs the "+scopeOf(err)+" scope", " to grant it"
	}
	if h := t.Hint(); h != "" {
		return why + ic.OrUnicode().Separator + h + do
	}
	return why + "."
}

// scopeOf returns the scope a *core.ScopeError in err asks to grant.
func scopeOf(err error) string {
	if e, ok := errors.AsType[*core.ScopeError](err); ok && e.Grant() != "" {
		return e.Grant()
	}
	return "right"
}

// kindArticle names a token of kind k after an article, as in "not a
// fine-grained one".
func kindArticle(k core.TokenKind) string {
	switch k {
	case core.TokenFineGrained:
		return "a fine-grained one"
	case core.TokenApp:
		return "a GitHub App's"
	default:
		return "this one"
	}
}

// AccessMsg reports what the token may do, each time that changes and
// after the user asked the app to read the token again. The sections read
// again what the token was refused, and the gates follow it.
type AccessMsg struct {
	Access core.Access
}

// Unreadable returns what a pane shows in place of data that needs n,
// which action reads, such as "load the notifications", when the token is
// known not to be allowed it: Say's words for why and what to do, and ok
// set. It reports ok unset when the token may, or that isn't known.
func Unreadable(n core.Need, action string, v Voice) (text, hint string, ok bool) {
	err := v.Token.Check(n)
	if err == nil {
		return "", "", false
	}
	text, hint = Say(core.Explain(action, err), v)
	return text, hint, true
}
