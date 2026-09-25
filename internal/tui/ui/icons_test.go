package ui

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

var iconSets = []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII}

func TestIconsAreOneCellWide(t *testing.T) {
	for _, set := range iconSets {
		ic := NewIcons(set)
		glyphs := []string{ic.Fork, ic.Private, ic.Archived, ic.Template, ic.Mirror, ic.Here, ic.Star, ic.Language("")}
		for s := range NumStates {
			glyphs = append(glyphs, ic.State(s))
		}
		for _, g := range ic.langs {
			glyphs = append(glyphs, g)
		}
		for _, g := range glyphs {
			if w := ansi.StringWidth(g); w != 1 || len([]rune(g)) != 1 {
				t.Errorf("%s: glyph %q is %d cells wide, want 1", set, g, w)
			}
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
		{core.RunWaiting, "", RunQueued},
		{core.RunPending, "", RunQueued},
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
