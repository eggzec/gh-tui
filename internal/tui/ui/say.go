package ui

import (
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// The most cells that Say's text and the action in a toast take, and the
// fewest of a cause a toast cuts to.
const (
	SayWidth    = 300
	actionWidth = 60
	minCause    = 12
)

// restart ends the sentences that ask the user to fix the token with gh,
// since the app reads the token only as it starts.
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
}

// NewVoice returns the voice of the configured keys, whose refresh key
// retries and whose open key opens on GitHub, pointing to the log file at
// log, or to none if log is "".
func NewVoice(keys map[string][]string, log string) Voice {
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

// SayLine returns what the user should read about err, which stopped
// action, on one line: Say's text and, after " · ", its hint.
func SayLine(action string, err error, v Voice) string {
	text, hint := Say(core.Explain(action, err), v)
	if hint == "" {
		return text
	}
	return text + " · " + hint
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
		text = cutWords(text, SayWidth)
	}
	return text, hint, named
}

// words says p, at any length.
func words(p *core.Problem, v Voice) (text, hint string, named bool) {
	retry := keyHint(v.Retry, "retry")
	open := keyHint(v.Open, "open on GitHub")
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
		layout := "15:04"
		if p.Reset.Sub(now) >= 24*time.Hour {
			layout = "Jan 2, 15:04"
		}
		return "Rate limited until " + p.Reset.In(cmp.Or(v.Loc, time.Local)).Format(layout), "loads again then", false
	case core.Auth:
		// The app reads the token once, as it starts, so a new one from gh
		// reaches it only after a restart; the token-scopes work (A3, a
		// token Reload) will switch these to "then r to retry".
		if s := grant(p); s != "" {
			return "The token lacks the " + s + " scope. Run gh auth refresh -s " + s + restart + ".", "", false
		}
		return "GitHub rejected the token. Run gh auth login" + restart + ".", "", false
	case core.Forbidden:
		if sso(p) {
			if org, _, _ := strings.Cut(subject, "/"); org != "" {
				return org + " requires SSO. Run gh auth refresh" + restart + ".", "", true
			}
			return "The organization requires SSO. Run gh auth refresh" + restart + ".", "", false
		}
		return "You don't have access to " + cmp.Or(subject, "this"), open, false
	case core.NotFound:
		if subject != "" {
			return subject + " doesn't exist or is private.", "", true
		}
		return "This doesn't exist or is private.", "", false
	case core.Rejected:
		if reason := clean(p.Reason); reason != "" {
			return reason, open, true
		}
		return "GitHub refused this", open, false
	default:
		if v.Log == "" {
			return "Something went wrong", retry, false
		}
		return "Something went wrong. Details are in the log (" + v.Log + ")", retry, false
	}
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
	action := cmp.Or(clean(p.Action), "do that")
	if ansi.StringWidth(action) > actionWidth {
		action = cutWords(action, actionWidth)
	}
	if !named {
		text = lowerFirst(text)
	}
	causes := toastCauses(p, v, text)
	actions := actionCuts(action)
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
		if !fits(head + cutWords(shortest, least)) {
			continue
		}
		for w := ansi.StringWidth(shortest); w > least; w-- {
			if s := head + cutWords(shortest, w); fits(s) {
				return s
			}
		}
		return head + cutWords(shortest, least)
	}
	return "Couldn't " + actions[len(actions)-1] + ": …"
}

// actionCuts returns action and ever shorter cuts of it, down to "…".
func actionCuts(action string) []string {
	cuts := []string{action}
	for w := ansi.StringWidth(action); w > 0; w-- {
		if c := cutWords(action, w); c != cuts[len(cuts)-1] {
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

// sentence ends s with a full stop, unless it ends a sentence already.
func sentence(s string) string {
	if strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?") {
		return s
	}
	return s + "."
}

// cutWords cuts s to width cells, between words unless that loses most of
// what fits, and ends it in "…".
func cutWords(s string, width int) string {
	cut := ansi.Truncate(s, width-1, "")
	if len(cut) < len(s) && s[len(cut)] != ' ' {
		if i := strings.LastIndexByte(cut, ' '); i > len(cut)/2 {
			cut = cut[:i]
		}
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}

// ShortPath returns path with the user's home directory as ~, so that a
// path shown on screen doesn't spell out where the home directory is.
func ShortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	// A home at the root would make every path start with ~.
	if home = filepath.Clean(home); filepath.Dir(home) == home {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// keyHint returns "<key> to <do>" with the key of b, or "" while b is
// unbound or disabled.
func keyHint(b key.Binding, do string) string {
	k := b.Help().Key
	if !b.Enabled() || k == "" {
		return ""
	}
	return k + " to " + do
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
