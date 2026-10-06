package ui

import (
	"cmp"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// The most cells that Say's text and the action in a toast take, and the
// fewest of a cause a toast cuts to.
const (
	SayWidth    = 300
	actionWidth = 60
	minCause    = 12
)

// restart ends the sentences that ask the user to fix the token with gh
// when the app has no command that reads the token again.
const restart = ", then restart gh-tui"

// Voice is what Say needs besides the problem: the keys a hint names, from
// the live keymap, the log file it points to, and the clock and zone it
// tells times by.
type Voice struct {
	// Retry reads again, and Open opens what failed on GitHub. A hint
	// names a key only while its binding is enabled.
	Retry, Open key.Binding
	// Log is the log file as ShortPath shows it, or "" when nothing is
	// logged.
	Log string
	// Loc is the zone times are told in; nil means time.Local.
	Loc *time.Location
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Token is what the token may do, and names the command that grants
	// it more, which the words for a token problem point to. Without it
	// they ask the user to fix the token with gh and restart the app.
	Token *Token
	// Icons give the words their separators and ellipses, and name their
	// keys. They are read each time the words are, so a voice copied into
	// a bubble follows a switch of the icons it points to. Nil words them
	// as the Unicode set does.
	Icons *Icons
}

// icons returns the icons of v, with the Unicode set's words where it has
// none.
func (v Voice) icons() Icons {
	if v.Icons == nil {
		return Icons{}.OrUnicode()
	}
	return v.Icons.OrUnicode()
}

// NewVoice returns the voice of the configured keys, whose refresh key
// retries and whose open key opens on GitHub, pointing to the log file at
// log, or to none if log is "".
func NewVoice(keys config.Keymap, log string) Voice {
	v := Voice{
		Retry: Binding(keys, config.ActionRefresh, "retry"),
		Open:  Binding(keys, config.ActionOpen, "open"),
	}
	if log != "" {
		v.Log = ShortPath(log)
	}
	return v
}

func (v Voice) now() time.Time {
	if v.Now == nil {
		return time.Now()
	}
	return v.Now()
}

// Say returns what the user should read about p: a sentence that says what
// went wrong, in SayWidth cells or fewer, and a hint that says what to do,
// such as "r to retry", or "". Both are empty for a problem the user
// needn't hear of, such as a read that was canceled. Nothing of the error
// shows but GitHub's own reason, cleaned, so no package, request or status
// code reaches the screen.
func Say(p *core.Problem, v Voice) (text, hint string) {
	text, hint, _ = say(p, v)
	return text, hint
}

// SayKept returns what a view says while it shows what an earlier read
// kept, because of a problem of kind: GitHub out of reach, or a rate
// limit, with the separator of ic. It is "" for any other kind, which
// serves nothing kept.
func SayKept(kind core.ProblemKind, ic Icons) string {
	switch kind {
	case core.Offline:
		return "offline" + ic.Separator + "showing the last visit"
	case core.RateLimited:
		return "rate limited" + ic.Separator + "showing the last visit"
	default:
		return ""
	}
}

// SayLine returns what the user should read about err, which stopped
// action, on one line: Say's text and, after the separator, its hint.
func SayLine(action string, err error, v Voice) string {
	text, hint := Say(core.Explain(action, err), v)
	if hint == "" {
		return text
	}
	return text + v.icons().Separator + hint
}

// ErrorText returns what a bubble shows for an error that stopped action
// on subject, such as the repository "eggzec/x": Say's text and hint, in
// the form its WithErrorText takes. subject is "" when there is nothing to
// name, and a subject the error names itself, such as the number of an
// issue, wins. A bubble that retries with a key of its own needs v with
// that key as Retry.
func ErrorText(action, subject string, v Voice) func(error) (text, hint string) {
	return func(err error) (text, hint string) {
		p := core.Explain(action, err)
		if p != nil && p.Subject == "" {
			// Explain may return a problem the error holds, which stays
			// as it is.
			named := *p
			named.Subject = subject
			p = &named
		}
		return Say(p, v)
	}
}

// say is Say, which also reports whether the text starts with a name, such
// as a repository or GitHub's reason, whose case a toast must keep.
func say(p *core.Problem, v Voice) (text, hint string, named bool) {
	if p == nil {
		return "", "", false
	}
	text, hint, named = words(p, v)
	if ansi.StringWidth(text) > SayWidth {
		text = cutWords(text, SayWidth, v.icons().Ellipsis)
	}
	return text, hint, named
}

// words says p, at any length.
func words(p *core.Problem, v Voice) (text, hint string, named bool) {
	retry := keyHint(v.icons(), v.Retry, "retry")
	open := keyHint(v.icons(), v.Open, "open on GitHub")
	subject := clean(p.Subject)
	switch p.Kind {
	case core.Canceled:
		return "", "", false
	case core.Offline:
		return "Can't reach GitHub", retry, false
	case core.Unavailable:
		return "GitHub isn't responding", retry, false
	case core.RateLimited:
		now := v.now()
		if !p.Reset.After(now) {
			// A limit that has lifted, or that GitHub didn't say when
			// it lifts, may pass on a retry.
			return "Rate limited by GitHub", retry, false
		}
		return "Rate limited until " + Clock(p.Reset.In(cmp.Or(v.Loc, time.Local)), now), "loads again then", false
	case core.Auth:
		return authWords(p, v.Token.Hint())
	case core.Forbidden:
		if sso(p) {
			return ssoWords(subject, v.Token.Hint())
		}
		// Access is granted per repository, so what is refused is named
		// by its repository, such as "eggzec/x" for "eggzec/x#5".
		repo, _, _ := strings.Cut(subject, "#")
		return "You don't have access to " + cmp.Or(repo, "this"), open, false
	case core.NotFound:
		text := "This doesn't exist or is private."
		if subject != "" {
			text = subject + " doesn't exist or is private."
		}
		// Without repo, GitHub hides what is private as if it weren't
		// there.
		if h := v.Token.Hint(); h != "" && lacksRepo(v.Token) {
			return text + " If it's private, the token needs the repo scope.", h + " to grant it", subject != ""
		}
		return text, "", subject != ""
	case core.Rejected:
		if reason := clean(p.Reason); reason != "" {
			return reason, open, true
		}
		return "GitHub refused this", open, false
	case core.Unsupported:
		return "This GitHub Enterprise version doesn't support this", open, false
	default:
		if v.Log == "" {
			return "Something went wrong", retry, false
		}
		return "Something went wrong. Details are in the log (" + v.Log + ")", retry, false
	}
}

// authWords says an Auth problem p, pointing to the command hint, such as
// ":auth", that grants the token what it lacks, or to gh and a restart
// when hint is "".
func authWords(p *core.Problem, hint string) (text, do string, named bool) {
	if e, ok := errors.AsType[*core.KindError](p.Err); ok {
		text = "This needs a classic token, not " + kindArticle(e.Kind)
		if hint == "" {
			return text + ". Run gh auth login to use one" + restart + ".", "", false
		}
		return text, hint + " to see how", false
	}
	s := grant(p)
	switch {
	case s != "" && hint != "":
		return "The token lacks the " + s + " scope", hint + " to grant it", false
	case s != "":
		return "The token lacks the " + s + " scope. Run gh auth refresh -s " + s + restart + ".", "", false
	case hint != "":
		return "GitHub rejected the token. Run gh auth login, then " + hint + ".", "", false
	}
	return "GitHub rejected the token. Run gh auth login" + restart + ".", "", false
}

// ssoWords says that the organization of subject, such as "eggzec/x",
// requires SSO, pointing to the command hint that authorizes the token,
// or to gh and a restart when hint is "".
func ssoWords(subject, hint string) (text, do string, named bool) {
	org, _, _ := strings.Cut(subject, "/")
	text, named = "The organization requires SSO", false
	if org != "" {
		text, named = org+" requires SSO", true
	}
	if hint != "" {
		return text, hint + " to authorize the token", named
	}
	return text + ". Run gh auth refresh" + restart + ".", "", named
}

// lacksRepo reports whether t is known to lack the repo scope.
func lacksRepo(t *Token) bool {
	_, ok := errors.AsType[*core.ScopeError](t.Check(core.NeedRuns))
	return ok
}

// SayToast returns the toast that tells the user p stopped its action,
// such as "Couldn't merge #5: can't reach GitHub.", saying as much as fits
// reports a toast shows whole. To fit, it first cuts the tail of the
// action, keeping its first word, then says the cause more briefly,
// keeping the command a call to action names, and then cuts the action
// further; only a cause that fits in no way is cut. A toast names no key,
// since what it tells of may no longer be on screen. It is "" when Say has
// nothing to say.
func SayToast(p *core.Problem, v Voice, fits func(string) bool) string {
	text, _, named := say(p, v)
	if text == "" {
		return ""
	}
	ell := v.icons().Ellipsis
	action := cmp.Or(clean(p.Action), "do that")
	if ansi.StringWidth(action) > actionWidth {
		action = cutWords(action, actionWidth, ell)
	}
	if !named {
		text = lowerFirst(text)
	}
	causes := toastCauses(p, v, text)
	actions := actionCuts(action, ell)
	// A cut that keeps the first word, such as "merge", still says what
	// failed.
	first, _, _ := strings.Cut(action, " ")
	kept := 1
	for kept < len(actions) && strings.HasPrefix(actions[kept], first) {
		kept++
	}
	for _, c := range causes {
		for _, a := range actions[:kept] {
			if s := "Couldn't " + a + ": " + sentence(c); fits(s) {
				return s
			}
		}
	}
	shortest := causes[len(causes)-1]
	for _, a := range actions[kept:] {
		if s := "Couldn't " + a + ": " + sentence(shortest); fits(s) {
			return s
		}
	}
	// No cause fits whole, so it is cut after the longest action that
	// leaves room for a few words of it.
	least := min(ansi.StringWidth(shortest), minCause)
	for _, a := range actions {
		head := "Couldn't " + a + ": "
		if !fits(head + cutWords(shortest, least, ell)) {
			continue
		}
		return head + longestCut(shortest, least, ell, func(c string) bool { return fits(head + c) })
	}
	return "Couldn't " + actions[len(actions)-1] + ": " + ell
}

// longestCut returns the longest cut of s, by cutWords to end in tail,
// that fits, and
// the cut to least cells when none longer does. A longer cut fits no
// better, so it is found by halves rather than by trying each width. If
// fits breaks that, and the cut found doesn't fit, each width is tried
// from the longest, so the cut is never worse than that search's.
func longestCut(s string, least int, tail string, fits func(string) bool) string {
	n := ansi.StringWidth(s) - least
	w := least + sort.Search(max(n, 0), func(i int) bool { return !fits(cutWords(s, least+1+i, tail)) })
	if c := cutWords(s, w, tail); w == least || fits(c) {
		return c
	}
	for w := ansi.StringWidth(s); w > least; w-- {
		if c := cutWords(s, w, tail); fits(c) {
			return c
		}
	}
	return cutWords(s, least, tail)
}

// actionCuts returns action and ever shorter cuts of it, each ending in
// tail, down to tail alone.
func actionCuts(action, tail string) []string {
	cuts := []string{action}
	for w := ansi.StringWidth(action); w > 0; w-- {
		if c := cutWords(action, w, tail); c != cuts[len(cuts)-1] {
			cuts = append(cuts, c)
		}
	}
	return cuts
}

// toastCauses returns the ways a toast may say the cause of p, whose
// sentence is text, from the fullest to the shortest. A toast has no room
// for the longer sentences, so the shorter ones keep what the user needs
// from them: the command to run, or where the log is.
func toastCauses(p *core.Problem, v Voice, text string) []string {
	if h := v.Token.Hint(); h != "" {
		return hintedCauses(p, v, text, h)
	}
	var run string
	switch scope := grant(p); {
	case p.Kind == core.Auth && scope != "":
		run = "run gh auth refresh -s " + scope
	case p.Kind == core.Auth:
		run = "run gh auth login"
	case p.Kind == core.Forbidden && sso(p):
		run = "run gh auth refresh"
	case p.Kind == core.Internal && v.Log != "":
		return []string{"something went wrong, see " + v.Log, "something went wrong, see the log"}
	default:
		return []string{text}
	}
	// Without the restart the command seems not to work, so the restart
	// outlasts what went wrong.
	return []string{text, run + restart, run}
}

// hintedCauses are toastCauses when the app has a command, hint, that
// fixes the token: a toast names it, though it names no key, since the
// command works from anywhere.
func hintedCauses(p *core.Problem, v Voice, text, hint string) []string {
	_, do, _ := words(p, v)
	sep := v.icons().Separator
	switch {
	case p.Kind == core.Internal && v.Log != "":
		return []string{"something went wrong, see " + v.Log, "something went wrong, see the log"}
	case p.Kind == core.Auth && do == "":
		// The text names the command already.
		return []string{text, "run gh auth login, then " + hint}
	case p.Kind == core.NotFound && do != "":
		first, _, _ := strings.Cut(text, ". ")
		return []string{text + sep + do, first + sep + hint + " grants the repo scope", first}
	case p.Kind == core.Auth || p.Kind == core.Forbidden && sso(p):
		return []string{text + sep + do, text + sep + hint}
	}
	return []string{text}
}

// sentence ends s with a full stop, unless it ends a sentence already.
func sentence(s string) string {
	if strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?") {
		return s
	}
	return s + "."
}

// cutWords cuts s to width cells, between words unless that loses most of
// what fits, and ends it in tail, an ellipsis.
func cutWords(s string, width int, tail string) string {
	cut := ansi.Truncate(s, max(width-ansi.StringWidth(tail), 0), "")
	if len(cut) < len(s) && s[len(cut)] != ' ' {
		if i := strings.LastIndexByte(cut, ' '); i > len(cut)/2 {
			cut = cut[:i]
		}
	}
	return strings.TrimRight(cut, " ,;:.") + tail
}

// ShortPath returns path with the user's home directory as ~, so that a
// path shown on screen doesn't spell out where the home directory is.
func ShortPath(path string) string {
	return obs.ShortHome(path)
}

// keyHint returns "<key> to <do>" with the key of b, named as ic names
// it, or "" while b is unbound or disabled.
func keyHint(ic Icons, b key.Binding, do string) string {
	k := b.Help().Key
	if !b.Enabled() || k == "" {
		return ""
	}
	return ic.Key(k) + " to " + do
}

// clean puts text from elsewhere, such as GitHub's reason, on one line
// without escape sequences, control characters or runs of spaces. It
// drops the invisible format characters too, such as bidi overrides and
// zero-width spaces, which could make the text read other than it is.
// OneLine keeps those, since names such as emoji need the zero-width
// joiner, but an error needs none of them.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, ansi.Strip(s))
	return strings.Join(strings.Fields(OneLine(s)), " ")
}

// grant returns the scope to grant the token for p: the one GitHub named
// in its headers or its error type, or else the one its reason names, or
// "".
func grant(p *core.Problem) string {
	if p.Grant != "" {
		return p.Grant
	}
	return missingScope(p.Reason)
}

// The ways GitHub names a scope a token lacks: GraphQL's "requires one of
// the following scopes: ['read:org']", and REST's "needs the "admin:org"
// scope".
var scopeReasons = []*regexp.Regexp{
	regexp.MustCompile(`following scopes: \[\s*['"]([a-z_:]+)['"]`),
	regexp.MustCompile("(?:needs|requires) the ['\"`]([a-z_:]+)['\"`] scope"),
}

// missingScope returns the scope that GitHub's reason says the token
// lacks, or "" if it names none. Only names made of the letters of scopes
// come back, since the name goes into a command the user runs.
func missingScope(reason string) string {
	for _, re := range scopeReasons {
		if m := re.FindStringSubmatch(reason); m != nil {
			return m[1]
		}
	}
	return ""
}

// ssoReason matches GitHub's reason for refusing a token that isn't
// authorized for an organization's SAML single sign-on.
var ssoReason = regexp.MustCompile(`\b(?:SAML|SSO)\b`)

// SSO reports whether err is an organization refusing the token until it
// is authorized for the organization's single sign-on, rather than any
// other refusal.
func SSO(err error) bool {
	p := core.Explain("", err)
	return p != nil && p.Kind == core.Forbidden && sso(p)
}

// sso reports whether p is an organization's SSO: GitHub said so in its
// headers, or its reason names SSO, as a GraphQL error's does.
func sso(p *core.Problem) bool {
	return p.SSO || ssoReason.MatchString(p.Reason)
}

// lowerFirst lowers the first letter of s, a sentence of the app's own,
// to follow a colon, unless its first word is a name such as GitHub.
func lowerFirst(s string) string {
	word, _, _ := strings.Cut(s, " ")
	r, n := utf8.DecodeRuneInString(word)
	if !unicode.IsUpper(r) || strings.ContainsFunc(word[n:], unicode.IsUpper) {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}
