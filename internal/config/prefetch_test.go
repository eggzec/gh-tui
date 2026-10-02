package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// resolve returns what cfg resolves kind on page to, failing t if it can't.
func resolve(t *testing.T, cfg Config, page, kind string) Resolved {
	t.Helper()
	r, err := cfg.Prefetch.Resolve(page, kind)
	if err != nil {
		t.Fatalf("Resolve(%s, %s): %v", page, kind, err)
	}
	return r
}

func TestResolveDefaults(t *testing.T) {
	cfg := Default()
	tests := []struct {
		page, kind    string
		enabled       bool
		before, after int
		rest          time.Duration
		fromEnabled   string
		fromBefore    string
		fromAfter     string
		fromRest      string
		windowless    bool
	}{
		{"pulls", "details", true, 1, 4, 150 * time.Millisecond,
			"prefetch.enabled", "prefetch.window.before", "prefetch.window.after", "prefetch.rest", false},
		{"pulls", "checks", false, 0, 2, 150 * time.Millisecond,
			"prefetch.pulls.checks.enabled", "prefetch.pulls.checks.window.before", "prefetch.pulls.checks.window.after", "prefetch.rest", false},
		{"dashboard", "waiting_on_you", true, 1, 2, 150 * time.Millisecond,
			"prefetch.enabled", "prefetch.dashboard.waiting_on_you.window.before", "prefetch.dashboard.waiting_on_you.window.after", "prefetch.rest", false},
		{"dashboard", "repositories", false, 1, 1, 150 * time.Millisecond,
			"prefetch.dashboard.repositories.enabled", "prefetch.dashboard.repositories.window.before", "prefetch.dashboard.repositories.window.after", "prefetch.rest", false},
		{"dashboard", "pinned", false, 1, 1, 150 * time.Millisecond,
			"prefetch.dashboard.pinned.enabled", "prefetch.dashboard.pinned.window.before", "prefetch.dashboard.pinned.window.after", "prefetch.rest", false},
		{"search", "details", true, 0, 0, 150 * time.Millisecond,
			"prefetch.enabled", "prefetch.search.window.before", "prefetch.search.window.after", "prefetch.rest", false},
		{"search", "other_kinds", true, 0, 0, 700 * time.Millisecond,
			"prefetch.enabled", "", "", "prefetch.search.other_kinds.rest", true},
		{"files", "preview", true, 0, 32, 150 * time.Millisecond,
			"prefetch.enabled", "prefetch.files.preview.window.before", "prefetch.files.preview.window.after", "prefetch.rest", false},
		{"finder", "preview", true, 0, 0, 100 * time.Millisecond,
			"prefetch.enabled", "prefetch.finder.window.before", "prefetch.finder.window.after", "prefetch.finder.rest", false},
		{"history", "commits", true, 3, 3, 150 * time.Millisecond,
			"prefetch.enabled", "prefetch.history.window.before", "prefetch.history.window.after", "prefetch.rest", false},
		{"history", "branches", true, 0, 0, 150 * time.Millisecond,
			"prefetch.enabled", "prefetch.history.branches.window.before", "prefetch.history.branches.window.after", "prefetch.rest", false},
		{"actions", "jobs", true, 1, 2, 150 * time.Millisecond,
			"prefetch.enabled", "prefetch.actions.window.before", "prefetch.actions.window.after", "prefetch.rest", false},
		{"actions", "logs", false, 0, 2, 150 * time.Millisecond,
			"prefetch.actions.logs.enabled", "prefetch.actions.logs.window.before", "prefetch.actions.logs.window.after", "prefetch.rest", false},
	}
	for _, tt := range tests {
		r := resolve(t, cfg, tt.page, tt.kind)
		if r.Enabled != tt.enabled || r.Window != (Window{tt.before, tt.after}) || r.Rest != tt.rest {
			t.Errorf("%s.%s = %v %v %v, want %v {%d %d} %v", tt.page, tt.kind, r.Enabled, r.Window, r.Rest,
				tt.enabled, tt.before, tt.after, tt.rest)
		}
		if r.From.Enabled != tt.fromEnabled || r.From.Before != tt.fromBefore || r.From.After != tt.fromAfter || r.From.Rest != tt.fromRest {
			t.Errorf("%s.%s from %+v, want %s, %s, %s, %s", tt.page, tt.kind, r.From,
				tt.fromEnabled, tt.fromBefore, tt.fromAfter, tt.fromRest)
		}
	}
}

// Everything reads 3 rows around the cursor, the pull request list 5, and
// it never reads checks ahead.
func TestResolvePrefetchLayers(t *testing.T) {
	t.Setenv(EnvLog, "")
	cfg, _, err := loadBase(writeConfig(t, `prefetch:
  window: {before: 3, after: 3}
  pulls:
    window: {before: 5, after: 5}
    checks: {enabled: false}
  issues:
    window: {after: 8}
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		page, kind    string
		before, after int
	}{
		{"pulls", "details", 5, 5},
		{"pulls", "comments", 5, 5},
		{"notifications", "details", 3, 3},
		// before and after resolve apart.
		{"issues", "details", 3, 8},
		// default.yaml sets it at the kind.
		{"dashboard", "waiting_on_you", 1, 2},
	} {
		if r := resolve(t, cfg, tt.page, tt.kind); r.Window != (Window{tt.before, tt.after}) {
			t.Errorf("%s.%s window = %v, want {%d %d}", tt.page, tt.kind, r.Window, tt.before, tt.after)
		}
	}
	if resolve(t, cfg, "pulls", "checks").Enabled {
		t.Error("pulls.checks is enabled")
	}
}

// enabled resolves like every knob: off globally turns off whatever
// doesn't turn itself on.
func TestResolveEnabled(t *testing.T) {
	cfg, err := Default().Set("prefetch.enabled", "false")
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err = cfg.Set("prefetch.history.commits.enabled", "true"); err != nil {
		t.Fatal(err)
	}
	for _, pk := range cfg.Prefetch.Kinds() {
		want := pk.String() == "history.commits"
		if got := resolve(t, cfg, pk.Page, pk.Kind).Enabled; got != want {
			t.Errorf("%s enabled = %v, want %v", pk, got, want)
		}
	}
}

// A window wider than a kind may read is cut to its bound.
func TestResolveCutsToBound(t *testing.T) {
	cfg, err := Default().Set("prefetch.window.after", "30")
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err = cfg.Set("prefetch.history.window.after", "10"); err != nil {
		t.Fatal(err)
	}
	cfg.Prefetch.History.Window.After = nil
	if r := resolve(t, cfg, "history", "commits"); r.Window.After != 10 {
		t.Errorf("history.commits after = %d, want the bound, 10", r.Window.After)
	}
	if r := resolve(t, cfg, "pulls", "details"); r.Window.After != 30 {
		t.Errorf("pulls.details after = %d, want 30", r.Window.After)
	}
}

func TestResolveUnknown(t *testing.T) {
	for _, pk := range [][2]string{{"nope", "details"}, {"pulls", "nope"}, {"pulls", "window"}, {"window", "before"}} {
		if _, err := Default().Prefetch.Resolve(pk[0], pk[1]); !errors.Is(err, ErrUnknownKey) {
			t.Errorf("Resolve(%s, %s) err = %v, want ErrUnknownKey", pk[0], pk[1], err)
		}
	}
}

func TestPrefetchKinds(t *testing.T) {
	kinds := Default().Prefetch.Kinds()
	if len(kinds) != 23 {
		t.Errorf("%d kinds, want 23: %v", len(kinds), kinds)
	}
	for _, want := range []string{"pulls.details", "pulls.other_tabs", "dashboard.pinned", "search.other_kinds", "files.preview", "actions.logs"} {
		if !slices.ContainsFunc(kinds, func(pk PageKind) bool { return pk.String() == want }) {
			t.Errorf("no kind %s", want)
		}
	}
}

func TestValidatePrefetch(t *testing.T) {
	tests := []struct {
		key, value, want string
	}{
		{"prefetch.window.after", "31", "prefetch.window.after: must be between 0 and 30, got 31"},
		{"prefetch.window.before", "-1", "prefetch.window.before: must be between 0 and 30"},
		{"prefetch.rest", "3s", "prefetch.rest: must be between 0 and 2s, got 3s"},
		{"prefetch.parallel", "0", "prefetch.parallel: must be between 1 and 10, got 0"},
		{"prefetch.pulls.window.after", "31", "prefetch.pulls.window.after: must be between 0 and 30"},
		{"prefetch.pulls.details.rest", "-1ms", "prefetch.pulls.details.rest: must be between 0 and 2s"},
		{"prefetch.history.window.before", "11", "prefetch.history.window.before: must be between 0 and 10"},
		{"prefetch.history.commits.window.after", "11", "prefetch.history.commits.window.after: must be between 0 and 10"},
		{"prefetch.files.preview.window.after", "65", "prefetch.files.preview.window.after: must be between 0 and 64"},
		{"prefetch.files.tree.window.after", "31", "prefetch.files.tree.window.after: must be between 0 and 30"},
		{"prefetch.pulls.other_tabs.window.after", "1",
			"prefetch.pulls.other_tabs.window: other tabs are read a page at a time, not around the cursor"},
		{"prefetch.search.other_kinds.window.before", "0",
			"prefetch.search.other_kinds.window: other kinds of results are read a page at a time"},
	}
	for _, tt := range tests {
		_, err := Default().Set(tt.key, tt.value)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Set(%s, %s) err = %v, want %q", tt.key, tt.value, err, tt.want)
		}
	}
	for key, value := range map[string]string{
		"prefetch.files.preview.window.after": "64",
		"prefetch.history.window.after":       "10",
		"prefetch.pulls.other_tabs.enabled":   "false",
		"prefetch.window.after":               "0",
		"prefetch.rest":                       "2s",
	} {
		if _, err := Default().Set(key, value); err != nil {
			t.Errorf("Set(%s, %s): %v", key, value, err)
		}
	}
	// A typo fails as in any other setting.
	t.Setenv(EnvLog, "")
	if _, _, err := loadBase(writeConfig(t, "prefetch:\n  pulls:\n    detail: {enabled: false}\n")); err == nil {
		t.Error("Load of prefetch.pulls.detail = nil error")
	}
}

func TestGetInherited(t *testing.T) {
	cfg := Default()
	for key, want := range map[string]string{
		"prefetch.window.after":                  "4",
		"prefetch.pulls.window.after":            "4 (from prefetch.window.after)",
		"prefetch.pulls.enabled":                 "true (from prefetch.enabled)",
		"prefetch.pulls.details.window.before":   "1 (from prefetch.window.before)",
		"prefetch.pulls.checks.enabled":          "false",
		"prefetch.search.details.window.after":   "0 (from prefetch.search.window.after)",
		"prefetch.search.other_kinds.rest":       "700ms",
		"prefetch.finder.preview.rest":           "100ms (from prefetch.finder.rest)",
		"prefetch.pulls.other_tabs.window.after": "none: other tabs are read a page at a time, not around the cursor",
	} {
		got, err := cfg.Get(key)
		if err != nil || got != want {
			t.Errorf("Get(%s) = %q, %v; want %q", key, got, err, want)
		}
	}
	if !Inherits("prefetch.pulls.details.enabled") || Inherits("prefetch.enabled") || Inherits("ui.icons") {
		t.Error("Inherits is wrong about pulls.details.enabled, prefetch.enabled or ui.icons")
	}
	if vs := cfg.Values("prefetch.pulls.enabled"); !slices.Equal(vs, []string{"true", "false"}) {
		t.Errorf("Values(prefetch.pulls.enabled) = %v", vs)
	}
}

// Setting a knob of a kind sets only that kind, in only that config.
func TestSetKnob(t *testing.T) {
	d := Default()
	cfg, err := d.Set("prefetch.pulls.details.window.after", "7")
	if err != nil {
		t.Fatal(err)
	}
	if r := resolve(t, cfg, "pulls", "details"); r.Window.After != 7 || r.From.After != "prefetch.pulls.details.window.after" {
		t.Errorf("pulls.details after = %d from %s, want 7 from its own", r.Window.After, r.From.After)
	}
	if r := resolve(t, cfg, "pulls", "comments"); r.Window.After != 4 {
		t.Errorf("pulls.comments after = %d, want 4", r.Window.After)
	}
	if r := resolve(t, d, "pulls", "details"); r.Window.After != 4 {
		t.Errorf("the config Set was called on changed: after = %d", r.Window.After)
	}
	if got, _ := cfg.Get("prefetch.pulls.details.window.after"); got != "7" {
		t.Errorf("Get = %q, want 7", got)
	}
}

func TestPrefetchTable(t *testing.T) {
	lines := Default().Prefetch.Table()
	if len(lines) != len(Default().Prefetch.Kinds()) {
		t.Fatalf("%d lines, want one a kind", len(lines))
	}
	for _, want := range []string{
		"pulls.details           enabled true  (prefetch.enabled)  window 1 (prefetch.window.before) / 4 (prefetch.window.after)  rest 150ms (prefetch.rest)",
		"pulls.checks            enabled false (prefetch.pulls.checks.enabled)",
		"search.other_kinds      enabled true  (prefetch.enabled)  rest 700ms (prefetch.search.other_kinds.rest)",
	} {
		// Only the words: the columns are as wide as the longest kind.
		if !slices.ContainsFunc(lines, func(l string) bool {
			return strings.HasPrefix(strings.Join(strings.Fields(l), " "), strings.Join(strings.Fields(want), " "))
		}) {
			t.Errorf("no line starts %q in\n%s", want, strings.Join(lines, "\n"))
		}
	}
}

// TestGetInheritedCut checks that a page's inherited window is shown as
// the page reads it, cut to its bound.
func TestGetInheritedCut(t *testing.T) {
	cfg := Default()
	cfg.Prefetch.Window.After = 20
	cfg.Prefetch.History.Window = Span{}
	if got, _ := cfg.Get("prefetch.history.window.after"); got != "10 (from prefetch.window.after)" {
		t.Errorf("Get = %q, want the global 20 cut to the history's 10", got)
	}
}

// TestNullHandsKnobBack checks that null on a knob that inherits drops
// what default.yaml sets for it, so it follows the layer above, and that
// null is still refused elsewhere.
func TestNullHandsKnobBack(t *testing.T) {
	t.Setenv(EnvLog, "")
	cfg, _, err := loadBase(writeConfig(t, "prefetch:\n  window: {before: 2, after: 6}\n  search:\n    window: {before: null, after: null}\n"))
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if r := resolve(t, cfg, "search", "details"); r.Window != (Window{2, 6}) || r.From.After != "prefetch.window.after" {
		t.Errorf("search.details = %v from %s, want the global window", r.Window, r.From.After)
	}
	// Under a host, whose name holds dots, and under a profile.
	for _, tt := range []struct{ file, login string }{
		{"prefetch:\n  window: {before: 2, after: 6}\nhosts:\n  github.com:\n    prefetch:\n      search:\n        window: {before: null, after: null}\n", ""},
		{"prefetch:\n  window: {before: 2, after: 6}\nprofiles:\n  work:\n    accounts: [mona@github.com]\n    prefetch:\n      search:\n        window: {before: null, after: null}\n", "mona"},
	} {
		cfg, _ := resolveFile(t, tt.file, "github.com", tt.login)
		if r := resolve(t, cfg, "search", "details"); r.Window != (Window{2, 6}) {
			t.Errorf("search.details = %v under %q, want the global window", r.Window, tt.file)
		}
	}
	for _, file := range []string{"prefetch:\n  rest: null\n", "prefetch:\n  search:\n    window: null\n", "hosts:\n  github.com:\n    prefetch:\n      rest: null\n"} {
		if _, _, err := loadBase(writeConfig(t, file)); err == nil || !strings.Contains(err.Error(), "is empty") {
			t.Errorf("Load(%q) error = %v, want null refused", file, err)
		}
	}
}
