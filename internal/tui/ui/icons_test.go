package ui

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

var iconSets = []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII}

// marks returns the glyphs of ic that mark something, each one cell wide.
func marks(ic Icons) []string {
	return slices.Concat([]string{
		ic.Fork, ic.Private, ic.Archived, ic.Template, ic.Mirror, ic.Here, ic.Star, ic.Error, ic.Language(""),
		ic.Yes, ic.No, ic.Info, ic.Cursor, ic.Folded, ic.Unfolded, ic.Marked,
		ic.ChangesRequested, ic.ReviewRequired, ic.Dot, ic.Ring, ic.Crumb, ic.Before, ic.Cell, ic.Comment, ic.Recent,
	}, ic.states[:], ic.runs[:], slices.Collect(maps.Values(ic.langs)))
}

func TestIconsAreOneCellWide(t *testing.T) {
	for _, set := range iconSets {
		for _, g := range marks(NewIcons(set)) {
			if w := ansi.StringWidth(g); w != 1 || len([]rune(g)) != 1 {
				t.Errorf("%s: glyph %q is %d cells wide, want 1", set, g, w)
			}
		}
	}
}

// The ASCII set is ASCII alone, for terminals and fonts that draw nothing
// else.
func TestIconsASCII(t *testing.T) {
	ic := NewIcons(config.IconsASCII)
	b := ic.Border
	drawn := []string{
		ic.Separator, ic.Ellipsis, ic.Arrow, ic.Up, ic.Down, ic.Times, ic.Minus,
		b.Top, b.Bottom, b.Left, b.Right, b.TopLeft, b.TopRight, b.BottomLeft, b.BottomRight,
		b.MiddleLeft, b.MiddleRight, b.Middle, b.MiddleTop, b.MiddleBottom,
		ic.Edge, ic.InputEdge, ic.Remove, ic.Warning, ic.Below, ic.OpenQuote, ic.CloseQuote,
		ic.Key("↑/k ↓/j ←/h →/l ↵"),
	}
	for _, g := range append(marks(ic), drawn...) {
		for _, r := range g {
			if r > unicode.MaxASCII {
				t.Errorf("glyph %q is not ASCII", g)
			}
		}
	}
}

// Markdown draws its own glyphs in ASCII in the ASCII set, and as the
// renderer always did in the others.
func TestIconsMarkdown(t *testing.T) {
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	th := NewTheme(p, true)
	for _, set := range iconSets {
		want := markdown.Glyphs{}
		if set == config.IconsASCII {
			want = markdown.ASCIIGlyphs()
		}
		if got := th.Thread(NewIcons(set)).MarkdownGlyphs; got != want {
			t.Errorf("%s: the thread's markdown glyphs are %+v, want %+v", set, got, want)
		}
	}
}

func TestIconsKey(t *testing.T) {
	for label, want := range map[string]string{"↑/k ↵": "up/k enter", "↑↓": "up/down", "←→": "left/right", "½ page down": "half page down"} {
		if got := NewIcons(config.IconsASCII).Key(label); got != want {
			t.Errorf("ASCII Key(%q) = %q, want %q", label, got, want)
		}
	}
	for _, set := range []string{config.IconsUnicode, config.IconsNerd} {
		if got := NewIcons(set).Key("↑/k ↵"); got != "↑/k ↵" {
			t.Errorf("%s Key = %q, want it unchanged", set, got)
		}
	}
}

func TestIconsTellStatesApart(t *testing.T) {
	for _, set := range iconSets {
		ic := NewIcons(set)
		seen := map[string]State{}
		for s := range NumStates {
			if prev, ok := seen[ic.State(s)]; ok {
				t.Errorf("%s: states %d and %d share %q", set, prev, s, ic.State(s))
			}
			seen[ic.State(s)] = s
		}
	}
}

func TestIconsLanguage(t *testing.T) {
	nerd := NewIcons(config.IconsNerd)
	if nerd.Language("Go") == nerd.Language("Rust") || nerd.Language("Go") == nerd.Language("COBOL") {
		t.Error("Go, Rust and a language without a glyph should differ")
	}
	if got, want := nerd.Language("COBOL"), nerd.Language("Brainfuck"); got != want {
		t.Errorf("languages without a glyph get %q and %q, want the same", got, want)
	}
	uni := NewIcons(config.IconsUnicode)
	if uni.Language("Go") != uni.Language("Rust") {
		t.Error("the unicode set should mark every language alike, by color")
	}
	if NewIcons("bogus").Language("Go") != nerd.Language("Go") {
		t.Error("an unknown set should get the Nerd Font one")
	}
}

func TestIconsFlags(t *testing.T) {
	ic := NewIcons(config.IconsASCII)
	r := core.Repo{Fork: true, Private: true, Archived: true, Template: true, Mirror: true}
	if got := ic.Flags(r); !slices.Equal(got, []string{"F", "P", "A", "T", "M"}) {
		t.Errorf("Flags = %v, want F P A T M", got)
	}
	if got := ic.Flags(core.Repo{Private: true}); !slices.Equal(got, []string{"P"}) {
		t.Errorf("Flags = %v, want P", got)
	}
	if got := ic.Flags(core.Repo{}); got != nil {
		t.Errorf("Flags = %v, want none", got)
	}
}

func TestStates(t *testing.T) {
	tests := []struct {
		hit  core.SearchHit
		want State
	}{
		{core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{State: core.StateOpen}}, IssueOpen},
		{core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{State: core.StateClosed}}, IssueClosed},
		{core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{State: core.StateClosed, Reason: core.ReasonCompleted}}, IssueClosed},
		{core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{State: core.StateClosed, Reason: core.ReasonNotPlanned}}, IssueNotPlanned},
		{core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{State: core.StateClosed, Reason: core.ReasonDuplicate}}, IssueNotPlanned},
		{core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{State: core.StateOpen, Reason: core.ReasonReopened}}, IssueOpen},
		{core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{State: core.StateOpen}}, PullOpen},
		{core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{State: core.StateOpen}, Draft: true}, PullDraft},
		{core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{State: core.StateMerged}}, PullMerged},
		{core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{State: core.StateClosed}, Draft: true}, PullClosed},
	}
	for _, tt := range tests {
		if got := HitState(tt.hit); got != tt.want {
			t.Errorf("HitState(%s %s %s draft %v) = %d, want %d", tt.hit.Kind, tt.hit.Issue.State, tt.hit.Issue.Reason, tt.hit.Draft, got, tt.want)
		}
	}
}

func TestLanguageColor(t *testing.T) {
	tests := []struct {
		name, color string
		dark        bool
		want        string
	}{
		{"Go", "", true, "#00ADD8"},
		{"Go", "#123456", false, "#123456"},
		// Navy vanishes on a dark background, and yellow on a light one.
		{"Lua", "", true, ""},
		{"Lua", "", false, "#000080"},
		{"JavaScript", "", false, ""},
		{"JavaScript", "", true, "#f1e05a"},
		{"COBOL", "", true, ""},
		{"Go", "not a color", true, ""},
	}
	for _, tt := range tests {
		if got := LanguageColor(tt.name, tt.color, tt.dark); got != tt.want {
			t.Errorf("LanguageColor(%q, %q, %v) = %q, want %q", tt.name, tt.color, tt.dark, got, tt.want)
		}
	}
}

func TestRunIcons(t *testing.T) {
	for _, set := range iconSets {
		ic := NewIcons(set)
		seen := map[string]RunState{}
		for s := range NumRunStates {
			g := ic.Run(s)
			if w := ansi.StringWidth(g); w != 1 || len([]rune(g)) != 1 {
				t.Errorf("%s: run glyph %q is %d cells wide, want 1", set, g, w)
			}
			if prev, ok := seen[g]; ok {
				t.Errorf("%s: run states %d and %d share %q", set, prev, s, g)
			}
			seen[g] = s
		}
	}
	if NewIcons("bogus").Run(RunFailure) != NewIcons(config.IconsNerd).Run(RunFailure) {
		t.Error("an unknown set should get the Nerd Font glyphs")
	}
}

func TestRunStateOf(t *testing.T) {
	tests := []struct {
		status core.RunStatus
		c      core.Conclusion
		want   RunState
	}{
		{core.RunQueued, "", RunQueued},
		{core.RunWaiting, "", RunWaiting},
		{core.RunPending, "", RunWaiting},
		{core.RunRequested, "", RunWaiting},
		{core.RunInProgress, "", RunInProgress},
		{core.RunCancelling, "", RunCancelled},
		{core.RunCompleted, core.ConclusionSuccess, RunSuccess},
		{core.RunCompleted, core.ConclusionFailure, RunFailure},
		{core.RunCompleted, core.ConclusionStartupFailure, RunFailure},
		{core.RunCompleted, core.ConclusionCancelled, RunCancelled},
		{core.RunCompleted, core.ConclusionSkipped, RunSkipped},
		{core.RunCompleted, core.ConclusionTimedOut, RunTimedOut},
		{core.RunCompleted, core.ConclusionActionRequired, RunActionRequired},
		{core.RunCompleted, core.ConclusionNeutral, RunNeutral},
		{core.RunCompleted, core.ConclusionStale, RunNeutral},
	}
	for _, tt := range tests {
		if got := RunStateOf(tt.status, tt.c); got != tt.want {
			t.Errorf("RunStateOf(%s, %s) = %d, want %d", tt.status, tt.c, got, tt.want)
		}
	}
}

func TestStatusText(t *testing.T) {
	for status, want := range map[core.RunStatus]string{
		core.RunWaiting:    "waiting for approval",
		core.RunPending:    "waiting",
		core.RunRequested:  "waiting",
		core.RunQueued:     "queued",
		core.RunInProgress: "in progress",
	} {
		if got := StatusText(status); got != want {
			t.Errorf("StatusText(%s) = %q, want %q", status, got, want)
		}
	}
}

// The ASCII set spins a line, spaced as the view's own spinner is, and
// the other sets leave each view its own.
func TestIconsSpinnerOr(t *testing.T) {
	ascii := NewIcons(config.IconsASCII)
	if got := ascii.SpinnerOr(spinner.Dot).Frames[0]; got != "| " {
		t.Errorf("ASCII spinner after a spaced one = %q, want %q", got, "| ")
	}
	if got := ascii.SpinnerOr(spinner.MiniDot).Frames[0]; got != "|" {
		t.Errorf("ASCII spinner after an unspaced one = %q, want %q", got, "|")
	}
	for _, def := range []spinner.Spinner{spinner.Dot, spinner.MiniDot} {
		if got := ascii.SpinnerOr(def); len(got.Frames) != len(def.Frames) || got.FPS != def.FPS {
			t.Errorf("ASCII spinner has %d frames at %v, want %d at %v", len(got.Frames), got.FPS, len(def.Frames), def.FPS)
		}
	}
	if got := NewIcons(config.IconsUnicode).SpinnerOr(spinner.Dot).Frames[0]; got != spinner.Dot.Frames[0] {
		t.Errorf("Unicode spinner = %q, want the view's own", got)
	}
}

// A spinner switched to the icon set's frames mid-spin, at its last
// frame, still draws one.
func TestIconsSpinnerSwitchMidSpin(t *testing.T) {
	for _, def := range []spinner.Spinner{spinner.Dot, spinner.MiniDot} {
		sp := spinner.New(spinner.WithSpinner(def))
		for range len(def.Frames) - 1 {
			sp, _ = sp.Update(sp.Tick())
		}
		sp.Spinner = NewIcons(config.IconsASCII).SpinnerOr(def)
		if v := sp.View(); v == "(error)" || strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("spinner after the switch draws %q", v)
		}
	}
}

// A list draws a marked row with the glyph of its icon set.
func TestFeedMarkGlyph(t *testing.T) {
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	th := NewTheme(p, true)
	for _, set := range iconSets {
		ic := NewIcons(set)
		if ic.Marked == "" {
			t.Errorf("%s: no glyph for a marked row", set)
		}
		if got := th.Feed(ic).MarkGlyph; got != ic.Marked {
			t.Errorf("%s: a list marks rows with %q, want %q", set, got, ic.Marked)
		}
	}
}
