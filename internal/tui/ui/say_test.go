package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// testLog is the log file as the tests show it.
const testLog = "~/.local/state/gh-tui/gh-tui.log"

// testVoice returns a voice with the default keys, the test log, a zone
// two hours east of UTC, and a clock ten minutes before reset.
func testVoice() Voice {
	keys := config.Default().Keys
	return Voice{
		Retry: Binding(keys, config.ActionRefresh, "retry"),
		Open:  Binding(keys, config.ActionOpen, "open"),
		Log:   testLog,
		Loc:   time.FixedZone("test", 2*60*60),
		Now:   func() time.Time { return reset.Add(-10 * time.Minute) },
	}
}

// reset is when the rate limit of the tests lifts: 14:05 in the zone of
// testVoice.
var reset = time.Date(2026, 9, 27, 12, 5, 0, 0, time.UTC)

// invisible are format characters that could make a reason read other
// than it is: bidi embeddings, overrides, isolates and marks, zero-width
// characters, the byte order mark and a tag character.
const invisible = "\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069\u200e\u200f\u061c\u200b\u200c\u200d\u2060\ufeff\U000E0041"

// within returns what fits a toast of width cells on one line.
func within(width int) func(string) bool {
	return func(s string) bool { return ansi.StringWidth(s) <= width }
}

// sayCase is a problem, with what Say and SayToast make of it under
// testVoice, in a toast of 80 cells. An empty toast isn't checked.
type sayCase struct {
	name       string
	p          *core.Problem
	text, hint string
	toast      string
}

// sayCases are problems of every kind.
var sayCases = []sayCase{
	{
		name: "canceled", p: &core.Problem{Kind: core.Canceled, Action: "load your profile"},
	},
	{
		name: "offline", p: &core.Problem{Kind: core.Offline, Action: "load your profile"},
		text: "Can't reach GitHub", hint: "r to retry",
		toast: "Couldn't load your profile: can't reach GitHub.",
	},
	{
		name: "unavailable", p: &core.Problem{Kind: core.Unavailable, Action: "load your profile"},
		text: "GitHub isn't responding", hint: "r to retry",
		toast: "Couldn't load your profile: GitHub isn't responding.",
	},
	{
		name: "rate limited", p: &core.Problem{Kind: core.RateLimited, Action: "merge #5", Reset: reset},
		text: "Rate limited until 14:05", hint: "loads again then",
		toast: "Couldn't merge #5: rate limited until 14:05.",
	},
	{
		name: "rate limited without a reset", p: &core.Problem{Kind: core.RateLimited, Action: "merge #5"},
		text: "Rate limited by GitHub", hint: "r to retry",
		toast: "Couldn't merge #5: rate limited by GitHub.",
	},
	{
		name: "rate limited until a time past", p: &core.Problem{Kind: core.RateLimited, Action: "merge #5", Reset: reset.Add(-time.Hour)},
		text: "Rate limited by GitHub", hint: "r to retry",
	},
	{
		name: "rate limited until another day", p: &core.Problem{Kind: core.RateLimited, Action: "merge #5", Reset: reset.Add(30 * time.Hour)},
		text: "Rate limited until Sep 28 20:05", hint: "loads again then",
		toast: "Couldn't merge #5: rate limited until Sep 28 20:05.",
	},
	{
		name: "auth", p: &core.Problem{Kind: core.Auth, Action: "load your profile", Reason: "Bad credentials"},
		text:  "GitHub rejected the token. Run gh auth login, then restart gh-tui.",
		toast: "Couldn't load your profile: run gh auth login, then restart gh-tui.",
	},
	{
		name: "auth missing a scope",
		p: &core.Problem{Kind: core.Auth, Action: "load your teams", Reason: "Your token has not been granted the required scopes to execute this query. " +
			"The 'teams' field requires one of the following scopes: ['read:org'], but your token has only been granted the: ['repo'] scopes."},
		text:  "The token lacks the read:org scope. Run gh auth refresh -s read:org, then restart gh-tui.",
		toast: "Couldn't load your teams: run gh auth refresh -s read:org, then restart gh-tui.",
	},
	{
		name: "auth missing a scope REST names",
		p:    &core.Problem{Kind: core.Auth, Action: "list the members", Reason: `This API operation needs the "admin:org" scope.`},
		text: "The token lacks the admin:org scope. Run gh auth refresh -s admin:org, then restart gh-tui.",
	},
	{
		name: "forbidden", p: &core.Problem{Kind: core.Forbidden, Action: "load the files", Subject: "eggzec/x", Reason: "Resource not accessible by integration"},
		text: "You don't have access to eggzec/x", hint: "o to open on GitHub",
		toast: "Couldn't load the files: you don't have access to eggzec/x.",
	},
	{
		name: "forbidden without a subject", p: &core.Problem{Kind: core.Forbidden, Action: "load the files"},
		text: "You don't have access to this", hint: "o to open on GitHub",
	},
	{
		name: "forbidden by SSO",
		p: &core.Problem{Kind: core.Forbidden, Action: "load the files", Subject: "eggzec/x", Reason: "Resource protected by organization SAML enforcement. " +
			"You must grant your Personal Access token access to this organization."},
		text:  "eggzec requires SSO. Run gh auth refresh, then restart gh-tui.",
		toast: "Couldn't load…: eggzec requires SSO. Run gh auth refresh, then restart gh-tui.",
	},
	{
		name: "forbidden by SSO keeps the case of the organization", p: &core.Problem{Kind: core.Forbidden, Action: "sync", Subject: "Acme/x", Reason: "SAML enforcement"},
		text:  "Acme requires SSO. Run gh auth refresh, then restart gh-tui.",
		toast: "Couldn't sync: Acme requires SSO. Run gh auth refresh, then restart gh-tui.",
	},
	{
		name: "forbidden by SSO without a subject", p: &core.Problem{Kind: core.Forbidden, Action: "load the files", Reason: "SSO required"},
		text:  "The organization requires SSO. Run gh auth refresh, then restart gh-tui.",
		toast: "Couldn't load the files: run gh auth refresh, then restart gh-tui.",
	},
	{
		name: "not found", p: &core.Problem{Kind: core.NotFound, Action: "load #5", Subject: "eggzec/x#5"},
		text:  "eggzec/x#5 doesn't exist or is private.",
		toast: "Couldn't load #5: eggzec/x#5 doesn't exist or is private.",
	},
	{
		name: "not found keeps the case of the subject", p: &core.Problem{Kind: core.NotFound, Action: "load #5", Subject: "Microsoft/vscode#5"},
		text:  "Microsoft/vscode#5 doesn't exist or is private.",
		toast: "Couldn't load #5: Microsoft/vscode#5 doesn't exist or is private.",
	},
	{
		name: "not found without a subject", p: &core.Problem{Kind: core.NotFound, Action: "load #5"},
		text:  "This doesn't exist or is private.",
		toast: "Couldn't load #5: this doesn't exist or is private.",
	},
	{
		name: "rejected", p: &core.Problem{Kind: core.Rejected, Action: "merge #5", Reason: "Pull Request is not mergeable"},
		text: "Pull Request is not mergeable", hint: "o to open on GitHub",
		toast: "Couldn't merge #5: Pull Request is not mergeable.",
	},
	{
		name: "rejected with a dirty reason", p: &core.Problem{Kind: core.Rejected, Action: "merge #5", Reason: "\x1b[31mBase\u200b branch\x1b[0m was\n\tmod\u202eified\u202c.\r\n  Review\u2066 and\u2069 try\x07 again" + invisible + "."},
		text: "Base branch was modified. Review and try again.", hint: "o to open on GitHub",
		toast: "Couldn't merge #5: Base branch was modified. Review and try again.",
	},
	{
		name: "rejected without a reason", p: &core.Problem{Kind: core.Rejected, Action: "merge #5"},
		text: "GitHub refused this", hint: "o to open on GitHub",
		toast: "Couldn't merge #5: GitHub refused this.",
	},
	{
		name: "unsupported", p: &core.Problem{Kind: core.Unsupported, Action: "open o/r", Reason: "Field 'x' doesn't exist on type 'Repository'"},
		text: "This GitHub Enterprise version doesn't support this", hint: "o to open on GitHub",
		toast: "Couldn't open o/r: this GitHub Enterprise version doesn't support this.",
	},
	{
		name: "internal", p: &core.Problem{Kind: core.Internal, Action: "merge #5", Err: errors.New("graphql: decode: unexpected EOF")},
		text: "Something went wrong. Details are in the log (" + testLog + ")", hint: "r to retry",
		toast: "Couldn't merge #5: something went wrong, see " + testLog + ".",
	},
}

func TestSay(t *testing.T) {
	for _, tt := range sayCases {
		t.Run(tt.name, func(t *testing.T) {
			text, hint := Say(tt.p, testVoice())
			if text != tt.text || hint != tt.hint {
				t.Errorf("Say = %q, %q; want %q, %q", text, hint, tt.text, tt.hint)
			}
			if tt.toast == "" {
				return
			}
			if got := SayToast(tt.p, testVoice(), within(80)); got != tt.toast {
				t.Errorf("SayToast = %q, want %q", got, tt.toast)
			}
		})
	}
}

// TestSayFields checks that Say goes by the scope and the SSO that the
// github package found, rather than by GitHub's reason.
func TestSayFields(t *testing.T) {
	tests := []struct {
		name        string
		p           *core.Problem
		text, toast string
	}{
		{
			name:  "the scope GitHub named",
			p:     &core.Problem{Kind: core.Auth, Action: "merge #5", Reason: "refusing to allow an OAuth App to create or update workflow without `workflow` scope", Grant: "workflow"},
			text:  "The token lacks the workflow scope. Run gh auth refresh -s workflow, then restart gh-tui.",
			toast: "Couldn't merge #5: run gh auth refresh -s workflow, then restart gh-tui.",
		},
		{
			name: "the scope over the reason",
			p:    &core.Problem{Kind: core.Auth, Action: "list the members", Reason: `This API operation needs the "admin:org" scope.`, Grant: "read:org"},
			text: "The token lacks the read:org scope. Run gh auth refresh -s read:org, then restart gh-tui.",
		},
		{
			name:  "the SSO GitHub named",
			p:     &core.Problem{Kind: core.Forbidden, Action: "sync", Subject: "eggzec/x", Reason: "Resource protected", SSO: true},
			text:  "eggzec requires SSO. Run gh auth refresh, then restart gh-tui.",
			toast: "Couldn't sync: eggzec requires SSO. Run gh auth refresh, then restart gh-tui.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if text, _ := Say(tt.p, testVoice()); text != tt.text {
				t.Errorf("Say = %q, want %q", text, tt.text)
			}
			if tt.toast == "" {
				return
			}
			if got := SayToast(tt.p, testVoice(), within(80)); got != tt.toast {
				t.Errorf("SayToast = %q, want %q", got, tt.toast)
			}
		})
	}
}

// TestSayNeverLeaks explains errors as the github package makes them and
// checks that nothing of the chain but GitHub's reason shows.
func TestSayNeverLeaks(t *testing.T) {
	chain := fmt.Errorf("merge pull: graphql: %w", &core.RefusedError{Action: "merge", Reason: "\x1b]8;;https://evil\x07Head\x1b]8;;\x07 is out of date"})
	for _, err := range []error{
		fmt.Errorf("list pulls: GET /repos/eggzec/x/pulls: github: 502 Bad Gateway: %w", core.ErrUnavailable),
		fmt.Errorf("dial tcp 140.82.112.6:443: %w", core.ErrOffline),
		fmt.Errorf("github: 401 Bad credentials: %w", core.ErrUnauthorized),
		fmt.Errorf("github: 404 Not Found: %w", core.ErrNotFound),
		errors.New("json: cannot unmarshal string into Go value of type github.restPull"),
		chain,
		&core.RefusedError{Action: "merge", Reason: "Head \u202eelif.exe\u202c is out of date" + invisible},
		&core.Problem{Kind: core.NotFound, Subject: "egg" + invisible + "zec/x#5", Err: core.ErrNotFound},
		&core.Problem{Kind: core.Forbidden, Subject: "egg\u202ezec/x", Reason: "SAML", Err: core.ErrForbidden},
	} {
		p := core.Explain("merge #5"+invisible, err)
		text, hint := Say(p, testVoice())
		all := text + " " + hint + " " + SayToast(p, testVoice(), within(80))
		for _, leak := range []string{"github:", "graphql", "GET", "/repos", "401", "404", "502", "json", "tcp", "\x1b", "evil"} {
			if strings.Contains(all, leak) {
				t.Errorf("Say(%v) = %q, which shows %q", err, all, leak)
			}
		}
		if strings.ContainsFunc(all, isFormat) {
			t.Errorf("Say(%v) = %q, which holds invisible format characters", err, all)
		}
	}
}

// isFormat reports whether r is an invisible format character.
func isFormat(r rune) bool {
	return unicode.Is(unicode.Cf, r)
}

func TestSayLine(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("list pulls: %w", core.ErrOffline), "Can't reach GitHub · r to retry"},
		{&core.NoNumberError{Repo: core.RepoRef{Owner: "eggzec", Name: "x"}, Number: 5, Err: core.ErrNotFound}, "eggzec/x#5 doesn't exist or is private."},
		{context.Canceled, ""},
	}
	for _, tt := range tests {
		if got := SayLine("load #5", tt.err, testVoice()); got != tt.want {
			t.Errorf("SayLine(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestErrorText(t *testing.T) {
	x := core.RepoRef{Owner: "eggzec", Name: "x"}
	tests := []struct {
		name, subject string
		err           error
		text, hint    string
	}{
		{"offline", "eggzec/x", fmt.Errorf("list files: %w", core.ErrOffline), "Can't reach GitHub", "r to retry"},
		{"forbidden names the subject", "eggzec/x", fmt.Errorf("list files: %w", core.ErrForbidden), "You don't have access to eggzec/x", "o to open on GitHub"},
		{"forbidden names the repository of an item", "eggzec/x#5", fmt.Errorf("merge #5: %w", core.ErrForbidden), "You don't have access to eggzec/x", "o to open on GitHub"},
		{"forbidden without a subject", "", fmt.Errorf("list files: %w", core.ErrForbidden), "You don't have access to this", "o to open on GitHub"},
		{"not found names the subject", "eggzec/x", fmt.Errorf("list files: %w", core.ErrNotFound), "eggzec/x doesn't exist or is private.", ""},
		{"the error's own subject wins", "eggzec/x", &core.NoNumberError{Repo: x, Number: 5, Err: core.ErrNotFound}, "eggzec/x#5 doesn't exist or is private.", ""},
		{"canceled", "eggzec/x", context.Canceled, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, hint := ErrorText("load the files", tt.subject, testVoice())(tt.err)
			if text != tt.text || hint != tt.hint {
				t.Errorf("ErrorText = %q, %q; want %q, %q", text, hint, tt.text, tt.hint)
			}
		})
	}
}

// A problem the error holds keeps its own subject, however it is said.
func TestErrorTextLeavesTheProblemAlone(t *testing.T) {
	p := &core.Problem{Kind: core.Forbidden, Action: "merge #5", Err: core.ErrForbidden}
	if text, _ := ErrorText("load", "eggzec/x", testVoice())(p); text != "You don't have access to eggzec/x" || p.Subject != "" {
		t.Errorf("ErrorText = %q, and the problem's subject became %q", text, p.Subject)
	}
}

func TestSayCapsTheText(t *testing.T) {
	p := &core.Problem{Kind: core.Rejected, Reason: strings.Repeat("GitHub says no. ", 200)}
	text, _ := Say(p, testVoice())
	if w := ansi.StringWidth(text); w > SayWidth || !strings.HasSuffix(text, "…") {
		t.Errorf("Say of a long reason is %d cells, ending %q; want at most %d, ending in …", w, text[len(text)-10:], SayWidth)
	}
}

func TestNewVoice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	v := NewVoice(config.Default().Keys, filepath.Join(home, "gh-tui.log"))
	if v.Retry.Help().Key != "r" || v.Open.Help().Key != "o" || v.Log != filepath.Join("~", "gh-tui.log") {
		t.Errorf("NewVoice = retry %q, open %q, log %q", v.Retry.Help().Key, v.Open.Help().Key, v.Log)
	}
	if v := NewVoice(nil, ""); v.Log != "" || v.Retry.Enabled() || v.Open.Enabled() {
		t.Errorf("NewVoice without keys or a log = %+v", v)
	}
}

func TestSayHintsNameTheLiveKeys(t *testing.T) {
	p := &core.Problem{Kind: core.Offline}
	keys := map[string][]string{config.ActionRefresh: {"ctrl+r"}}
	v := Voice{Retry: Binding(keys, config.ActionRefresh, "retry")}
	if _, hint := Say(p, v); hint != "^r to retry" {
		t.Errorf("hint = %q, want ^r to retry", hint)
	}
	v.Retry.SetEnabled(false)
	if _, hint := Say(p, v); hint != "" {
		t.Errorf("hint of a disabled key = %q, want none", hint)
	}
	v.Retry = key.NewBinding(key.WithDisabled())
	if _, hint := Say(p, v); hint != "" {
		t.Errorf("hint of an unbound key = %q, want none", hint)
	}
	if text, _ := Say(&core.Problem{Kind: core.Internal}, Voice{}); text != "Something went wrong" {
		t.Errorf("Internal without a log = %q", text)
	}
	if text, hint := Say(nil, v); text != "" || hint != "" {
		t.Errorf("Say(nil) = %q, %q; want nothing", text, hint)
	}
}

func TestSayToastCutsTheCause(t *testing.T) {
	long := strings.Repeat("GitHub says no ", 20)
	tests := []struct {
		action, want string
	}{
		{"merge #5", "Couldn't merge #5: GitHub says no GitHub says no GitHub says no GitHub says no…"},
		{strings.Repeat("a", 75), "Couldn't " + strings.Repeat("a", 56) + "…: GitHub says…"},
		{strings.Repeat("merge it ", 30), "Couldn't " + strings.Repeat("merge it ", 5) + "merge it…: GitHub says no…"},
		{"", "Couldn't do that: GitHub says no GitHub says no GitHub says no GitHub says no…"},
	}
	for _, tt := range tests {
		got := SayToast(&core.Problem{Kind: core.Rejected, Action: tt.action, Reason: long}, testVoice(), within(80))
		if got != tt.want {
			t.Errorf("SayToast(%q) = %q, want %q", tt.action, got, tt.want)
		}
		if w := ansi.StringWidth(got); w > 80 {
			t.Errorf("SayToast(%q) is %d cells, want at most 80", tt.action, w)
		}
	}
}

// TestSayToastKeepsWhatMatters checks that a toast too short for the
// whole of it cuts the action first, and says the cause more briefly only
// then, still naming the command to run, or the log.
func TestSayToastKeepsWhatMatters(t *testing.T) {
	action := "load the members of the core team"
	scope := &core.Problem{Kind: core.Auth, Action: action, Reason: `This API operation needs the "admin:org" scope.`}
	login := &core.Problem{Kind: core.Auth, Action: action, Reason: "Bad credentials"}
	sso := &core.Problem{Kind: core.Forbidden, Action: action, Subject: "eggzec/x", Reason: "Resource protected by organization SAML enforcement."}
	internal := &core.Problem{Kind: core.Internal, Action: action}
	tests := []struct {
		p     *core.Problem
		width int
		want  string
	}{
		{scope, 120, "Couldn't load the members…: the token lacks the admin:org scope. Run gh auth refresh -s admin:org, then restart gh-tui."},
		{scope, 100, "Couldn't " + action + ": run gh auth refresh -s admin:org, then restart gh-tui."},
		{scope, 80, "Couldn't load the…: run gh auth refresh -s admin:org, then restart gh-tui."},
		{scope, 50, "Couldn't load…: run gh auth refresh -s admin:org."},
		{login, 80, "Couldn't load the members of the core…: run gh auth login, then restart gh-tui."},
		{login, 30, "Couldn't …: run gh auth login."},
		{sso, 120, "Couldn't " + action + ": eggzec requires SSO. Run gh auth refresh, then restart gh-tui."},
		{sso, 60, "Couldn't load…: run gh auth refresh, then restart gh-tui."},
		{internal, 80, "Couldn't load the…: something went wrong, see " + testLog + "."},
		{internal, 60, "Couldn't load the…: something went wrong, see the log."},
		// Past what any cause needs, the cause is cut too.
		{internal, 30, "Couldn't load the…: something…"},
	}
	for _, tt := range tests {
		if got := SayToast(tt.p, testVoice(), within(tt.width)); got != tt.want {
			t.Errorf("SayToast(%s) in %d cells = %q, want %q", tt.p.Kind, tt.width, got, tt.want)
		}
	}
}

// TestSayToastShowsTheWayOut checks that a toast of every kind, at every
// width from 40 columns, shows whole in the toast stack, so what it tells
// the user to do is never cut.
func TestSayToastShowsTheWayOut(t *testing.T) {
	action := "load the members of the core team"
	scopes := "Your token has not been granted the required scopes to execute this query. " +
		"The 'teams' field requires one of the following scopes: ['read:org']."
	tests := []struct {
		p    *core.Problem
		want []string
	}{
		{&core.Problem{Kind: core.Offline}, []string{"can't reach GitHub"}},
		{&core.Problem{Kind: core.Unavailable}, []string{"GitHub isn't responding"}},
		{&core.Problem{Kind: core.RateLimited, Reset: reset}, []string{"rate limited until 14:05"}},
		{&core.Problem{Kind: core.Auth, Reason: scopes}, []string{"run gh auth refresh -s read:org", "then restart gh-tui"}},
		{&core.Problem{Kind: core.Auth, Reason: "Bad credentials"}, []string{"run gh auth login", "then restart gh-tui"}},
		{&core.Problem{Kind: core.Forbidden, Subject: "eggzec/x"}, []string{"you don't have access to eggzec/x"}},
		{&core.Problem{Kind: core.Forbidden, Subject: "eggzec/x", Reason: "SAML enforcement"}, []string{"run gh auth refresh", "then restart gh-tui"}},
		{&core.Problem{Kind: core.NotFound, Subject: "eggzec/x#5"}, []string{"eggzec/x#5 doesn't exist or is private"}},
		{&core.Problem{Kind: core.Rejected, Reason: "Pull Request is not mergeable"}, []string{"Pull Request is not mergeable"}},
		{&core.Problem{Kind: core.Internal}, []string{"something went wrong, see"}},
	}
	squeeze := func(s string) string {
		return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
			return unicode.IsSpace(r) || r == '▌' || r == '✗'
		}), "")
	}
	for _, tt := range tests {
		tt.p.Action = action
		for width := 40; width <= 200; width++ {
			m := toast.New(toast.WithSize(width, 24))
			text := SayToast(tt.p, testVoice(), func(s string) bool { return m.Fits(toast.Error, s) })
			m.Push(toast.Error, text)
			shown := squeeze(ansi.Strip(m.View()))
			if !strings.Contains(shown, squeeze(text)) {
				t.Errorf("%s at %d columns: the toast cuts %q", tt.p.Kind, width, text)
			}
			for _, w := range tt.want {
				// The fuller cause says "Run" where the briefer says "run".
				if !strings.Contains(strings.ToLower(shown), strings.ToLower(squeeze(w))) {
					t.Errorf("%s at %d columns: %q doesn't say %q", tt.p.Kind, width, text, w)
				}
			}
		}
	}
}

func TestShortPath(t *testing.T) {
	home := t.TempDir()
	log := filepath.Join(home, ".local", "state", "gh-tui", "gh-tui.log")
	elsewhere := filepath.Join(string(filepath.Separator), "var", "log", "gh-tui.log")
	tests := []struct{ home, path, want string }{
		{home, log, filepath.Join("~", ".local", "state", "gh-tui", "gh-tui.log")},
		{home + string(filepath.Separator), log, filepath.Join("~", ".local", "state", "gh-tui", "gh-tui.log")},
		{home, home, "~"},
		{home, home + "x", home + "x"},
		{home, elsewhere, elsewhere},
		{string(filepath.Separator), elsewhere, elsewhere},
	}
	for _, tt := range tests {
		t.Setenv("HOME", tt.home)
		if got := ShortPath(tt.path); got != tt.want {
			t.Errorf("ShortPath(%q) with home %q = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}
