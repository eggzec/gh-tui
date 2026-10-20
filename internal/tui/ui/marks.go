package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// Mark is what the glyph of a notification shows: the kind of its subject
// and, where it has one, the state. The first marks are the States of pull
// requests and issues, so a State is a Mark; the others follow.
type Mark int

// Marks other than the States. The Unknown ones are the kind of a subject
// whose state isn't known yet, or can't be known.
const (
	// MarkDiscussionOpen, MarkDiscussionAnswered and MarkDiscussionClosed
	// mark a discussion that is open, one that has an answer, and one that
	// was closed.
	MarkDiscussionOpen Mark = Mark(NumStates) + iota
	MarkDiscussionAnswered
	MarkDiscussionClosed
	// MarkPullUnknown, MarkIssueUnknown and MarkDiscussionUnknown mark a
	// pull request, an issue and a discussion of an unknown state.
	MarkPullUnknown
	MarkIssueUnknown
	MarkDiscussionUnknown
	// MarkRelease, MarkCommit, MarkCheckSuite and MarkAlert mark a release,
	// a commit, a run of checks and a security alert.
	MarkRelease
	MarkCommit
	MarkCheckSuite
	MarkAlert
	// MarkOther marks any other kind of subject.
	MarkOther
	// NumMarks counts the marks, for arrays indexed by them.
	NumMarks
)

// SubjectMark returns the mark of a subject of type t in state st. known is
// whether st was read: a pull request, an issue or a discussion without it
// gets the mark of its kind alone.
func SubjectMark(t core.SubjectType, st core.SubjectState, known bool) Mark {
	switch t {
	case core.SubjectPullRequest:
		if !known {
			return MarkPullUnknown
		}
		return Mark(PullState(st.State, st.Draft))
	case core.SubjectIssue:
		if !known {
			return MarkIssueUnknown
		}
		return Mark(IssueState(core.Issue{State: st.State, Reason: st.Reason}))
	case core.SubjectDiscussion:
		switch {
		case !known:
			return MarkDiscussionUnknown
		case st.Answered:
			return MarkDiscussionAnswered
		case st.State == core.StateClosed:
			return MarkDiscussionClosed
		}
		return MarkDiscussionOpen
	case core.SubjectRelease:
		return MarkRelease
	case core.SubjectCommit:
		return MarkCommit
	case core.SubjectCheckSuite:
		return MarkCheckSuite
	case core.SubjectVulnerabilityAlert:
		return MarkAlert
	}
	return MarkOther
}

// markGlyphs returns the glyphs of the marks other than the States in the
// icon set named set, which fills those in from states. The ASCII set has
// no shapes, so it draws the short text tags of the kinds, and none for
// MarkOther, which has no tag of its own.
func markGlyphs(set string, states [NumStates]string) [NumMarks]string {
	var marks [NumMarks]string
	switch set {
	case config.IconsUnicode:
		marks = [NumMarks]string{
			MarkDiscussionOpen: "◇", MarkDiscussionAnswered: "◈", MarkDiscussionClosed: "◆",
			MarkDiscussionUnknown: "◇",
			MarkRelease:           "▲", MarkCommit: "•", MarkCheckSuite: "▶", MarkAlert: "⚠", MarkOther: "◦",
		}
	case config.IconsASCII:
		for _, m := range []Mark{Mark(PullOpen), Mark(PullDraft), Mark(PullMerged), Mark(PullClosed), MarkPullUnknown} {
			marks[m] = "pr"
		}
		for _, m := range []Mark{Mark(IssueOpen), Mark(IssueClosed), Mark(IssueNotPlanned), MarkIssueUnknown} {
			marks[m] = "iss"
		}
		for _, m := range []Mark{MarkDiscussionOpen, MarkDiscussionAnswered, MarkDiscussionClosed, MarkDiscussionUnknown} {
			marks[m] = "dsc"
		}
		marks[MarkRelease], marks[MarkCommit], marks[MarkCheckSuite], marks[MarkAlert] = "rel", "cmt", "ci", "sec"
		return marks
	default:
		// Octicons: comment-discussion, check-circle-fill, discussion-closed,
		// tag, git-commit, workflow, shield and bell.
		marks = [NumMarks]string{
			MarkDiscussionOpen: "", MarkDiscussionAnswered: "", MarkDiscussionClosed: "",
			MarkDiscussionUnknown: "",
			MarkRelease:           "", MarkCommit: "", MarkCheckSuite: "", MarkAlert: "", MarkOther: "",
		}
	}
	copy(marks[:NumStates], states[:])
	// A subject whose state is unknown shows the glyph of its kind in the
	// state it starts in.
	marks[MarkPullUnknown] = states[PullOpen]
	marks[MarkIssueUnknown] = states[IssueOpen]
	return marks
}

// Mark returns the glyph of mark m: a shape in the Nerd Font and Unicode
// sets, and a short tag such as "pr" in the ASCII set, where MarkOther has
// none ("").
func (ic Icons) Mark(m Mark) string {
	if m < 0 || m >= NumMarks {
		return ""
	}
	return ic.marks[m]
}

// Mark returns the style of the glyph of mark m. States are styled as State
// does, a discussion as GitHub shows it (open as an open issue, answered as
// a merged pull request, closed as an issue not planned), a security alert
// as a warning and a release in the accent; the rest, including the kinds
// whose state isn't known, are muted.
func (t Theme) Mark(m Mark) lipgloss.Style {
	switch {
	case m >= 0 && m < Mark(NumStates):
		return t.State(State(m))
	case m == MarkDiscussionOpen:
		return t.State(IssueOpen)
	case m == MarkDiscussionAnswered:
		return t.State(PullMerged)
	case m == MarkDiscussionClosed:
		return t.State(IssueNotPlanned)
	case m == MarkRelease:
		return t.Accent
	case m == MarkAlert:
		return t.Warning
	}
	return t.Muted
}
