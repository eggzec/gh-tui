package ui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestSubjectMark(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		typ   core.SubjectType
		state core.SubjectState
		known bool
		want  Mark
	}{
		{"open pull", core.SubjectPullRequest, core.SubjectState{State: core.StateOpen}, true, Mark(PullOpen)},
		{"draft pull", core.SubjectPullRequest, core.SubjectState{State: core.StateOpen, Draft: true}, true, Mark(PullDraft)},
		{"merged pull", core.SubjectPullRequest, core.SubjectState{State: core.StateMerged}, true, Mark(PullMerged)},
		{"closed pull", core.SubjectPullRequest, core.SubjectState{State: core.StateClosed}, true, Mark(PullClosed)},
		{"pull of unknown state", core.SubjectPullRequest, core.SubjectState{State: core.StateMerged}, false, MarkPullUnknown},
		{"open issue", core.SubjectIssue, core.SubjectState{State: core.StateOpen}, true, Mark(IssueOpen)},
		{"closed issue", core.SubjectIssue, core.SubjectState{State: core.StateClosed}, true, Mark(IssueClosed)},
		{"issue not planned", core.SubjectIssue, core.SubjectState{State: core.StateClosed, Reason: core.ReasonNotPlanned}, true, Mark(IssueNotPlanned)},
		{"issue of unknown state", core.SubjectIssue, core.SubjectState{}, false, MarkIssueUnknown},
		{"open discussion", core.SubjectDiscussion, core.SubjectState{State: core.StateOpen}, true, MarkDiscussionOpen},
		{"answered discussion", core.SubjectDiscussion, core.SubjectState{State: core.StateOpen, Answered: true}, true, MarkDiscussionAnswered},
		{"closed discussion", core.SubjectDiscussion, core.SubjectState{State: core.StateClosed}, true, MarkDiscussionClosed},
		{"discussion of unknown state", core.SubjectDiscussion, core.SubjectState{Answered: true}, false, MarkDiscussionUnknown},
		{"release", core.SubjectRelease, core.SubjectState{}, false, MarkRelease},
		{"commit", core.SubjectCommit, core.SubjectState{}, false, MarkCommit},
		{"check suite", core.SubjectCheckSuite, core.SubjectState{}, false, MarkCheckSuite},
		{"security alert", core.SubjectType("RepositoryVulnerabilityAlert"), core.SubjectState{}, false, MarkAlert},
		{"other kind", core.SubjectType("RepositoryInvitation"), core.SubjectState{State: core.StateOpen}, true, MarkOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SubjectMark(tt.typ, tt.state, tt.known); got != tt.want {
				t.Errorf("SubjectMark = %d, want %d", got, tt.want)
			}
		})
	}
}

// The states of pull requests and issues keep the glyphs and colors the
// lists give them, in the sets that have shapes.
func TestMarkReusesStates(t *testing.T) {
	t.Parallel()
	pal, _ := config.Default().Palette(true)
	th := NewTheme(pal, true)
	for _, set := range []string{config.IconsNerd, config.IconsUnicode} {
		ic := NewIcons(set)
		for s := range NumStates {
			if got, want := ic.Mark(Mark(s)), ic.State(s); got != want {
				t.Errorf("%s: Mark(%d) = %q, want the state's %q", set, s, got, want)
			}
			if got, want := th.Mark(Mark(s)).GetForeground(), th.State(s).GetForeground(); got != want {
				t.Errorf("%s: Mark(%d) color = %v, want the state's %v", set, s, got, want)
			}
		}
		if got, want := ic.Mark(MarkPullUnknown), ic.State(PullOpen); got != want {
			t.Errorf("%s: unknown pull = %q, want the kind's %q", set, got, want)
		}
		if got, want := ic.Mark(MarkIssueUnknown), ic.State(IssueOpen); got != want {
			t.Errorf("%s: unknown issue = %q, want the kind's %q", set, got, want)
		}
	}
}

func TestMarkGlyphs(t *testing.T) {
	t.Parallel()
	want := map[string]map[Mark]string{
		config.IconsNerd: {
			MarkDiscussionOpen: "", MarkDiscussionAnswered: "", MarkDiscussionClosed: "",
			MarkDiscussionUnknown: "", MarkRelease: "", MarkCommit: "",
			MarkCheckSuite: "", MarkAlert: "", MarkOther: "",
		},
		config.IconsUnicode: {
			MarkDiscussionOpen: "◇", MarkDiscussionAnswered: "◈", MarkDiscussionClosed: "◆",
			MarkDiscussionUnknown: "◇", MarkRelease: "▲", MarkCommit: "•",
			MarkCheckSuite: "▶", MarkAlert: "⚠", MarkOther: "◦",
		},
		config.IconsASCII: {
			Mark(PullOpen): "pr", Mark(PullDraft): "pr", Mark(PullMerged): "pr", Mark(PullClosed): "pr", MarkPullUnknown: "pr",
			Mark(IssueOpen): "iss", Mark(IssueClosed): "iss", Mark(IssueNotPlanned): "iss", MarkIssueUnknown: "iss",
			MarkDiscussionOpen: "dsc", MarkDiscussionAnswered: "dsc", MarkDiscussionClosed: "dsc", MarkDiscussionUnknown: "dsc",
			MarkRelease: "rel", MarkCommit: "cmt", MarkCheckSuite: "ci", MarkAlert: "sec",
			// No tag of its own: the row names the type.
			MarkOther: "",
		},
	}
	for set, marks := range want {
		ic := NewIcons(set)
		for m, glyph := range marks {
			if got := ic.Mark(m); got != glyph {
				t.Errorf("%s: Mark(%d) = %q, want %q", set, m, got, glyph)
			}
		}
		if got := ic.Mark(NumMarks); got != "" {
			t.Errorf("%s: Mark(NumMarks) = %q, want none", set, got)
		}
	}
}

// A discussion is colored as GitHub does, and the kinds with no state, and
// those whose state isn't known, are muted.
func TestMarkStyles(t *testing.T) {
	t.Parallel()
	pal, _ := config.Default().Palette(true)
	for _, dark := range []bool{false, true} {
		th := NewTheme(pal, dark)
		fg := func(m Mark) any { return th.Mark(m).GetForeground() }
		tests := []struct {
			m    Mark
			want any
		}{
			{MarkDiscussionOpen, th.State(IssueOpen).GetForeground()},
			{MarkDiscussionAnswered, th.State(PullMerged).GetForeground()},
			{MarkDiscussionClosed, th.State(IssueNotPlanned).GetForeground()},
			{MarkPullUnknown, th.Muted.GetForeground()},
			{MarkIssueUnknown, th.Muted.GetForeground()},
			{MarkDiscussionUnknown, th.Muted.GetForeground()},
			{MarkRelease, th.Accent.GetForeground()},
			{MarkCommit, th.Muted.GetForeground()},
			{MarkCheckSuite, th.Muted.GetForeground()},
			{MarkAlert, th.Warning.GetForeground()},
			{MarkOther, th.Muted.GetForeground()},
		}
		for _, tt := range tests {
			if got := fg(tt.m); got != tt.want {
				t.Errorf("dark %v: Mark(%d) color = %v, want %v", dark, tt.m, got, tt.want)
			}
		}
	}
}
