package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// Icons are the glyphs that mark repositories, their languages, and the
// states of issues and pull requests, from one of the sets that
// config.UI.Icons names. The Nerd Font set marks files and directories
// too. Every glyph is one cell wide, so rows that use
// them stay aligned. Rows should follow a glyph with a space: the glyphs of
// a Nerd Font often spill over into the next cell.
type Icons struct {
	// Fork, Private, Archived, Template and Mirror mark repositories, and
	// Here marks the repository of the current directory.
	Fork, Private, Archived, Template, Mirror, Here string
	// Star marks a count of stars.
	Star string
	// Error marks what went wrong, as ErrorLine shows it.
	Error string
	// Separator goes between the text of an error and its hint, and
	// Ellipsis ends the text where it is cut: " · " and "…", or ASCII in
	// the ASCII set.
	Separator, Ellipsis string
	// Yes and No mark what holds and what doesn't, such as a signature
	// GitHub verified and one it couldn't.
	Yes, No string

	// langs holds the glyphs of languages that have one; the others get
	// lang.
	langs  map[string]string
	lang   string
	states [NumStates]string
	runs   [NumRunStates]string
	// files holds the glyphs of files and directories, in the sets that
	// have them.
	files *fileIcons
}

// State is the state of an issue or pull request as a glyph shows it.
type State int

// States of issues and pull requests. An issue closed as not planned, or
// as a duplicate, is shown apart from one that was completed, and a draft
// apart from a pull request that is ready.
const (
	IssueOpen State = iota
	IssueClosed
	IssueNotPlanned
	PullOpen
	PullDraft
	PullMerged
	PullClosed
	// NumStates counts the states, for arrays indexed by them.
	NumStates
)

// NewIcons returns the icon set named set, one of config.IconsNerd,
// config.IconsUnicode and config.IconsASCII. An unknown name, which a
// validated config never has, gets the Nerd Font set.
func NewIcons(set string) Icons {
	ic := newIcons(set)
	ic.runs = runGlyphs(set)
	return ic
}

func newIcons(set string) Icons {
	switch set {
	case config.IconsUnicode:
		return Icons{
			Fork: "⑂", Private: "⊘", Archived: "⊟", Template: "⧉", Mirror: "⇄", Here: "⌂",
			Star: "★", Error: "✗", Separator: " · ", Ellipsis: "…",
			Yes: "✓", No: "✗",
			lang: "◉",
			states: [NumStates]string{
				IssueOpen: "⦾", IssueClosed: "⦿", IssueNotPlanned: "⊘",
				PullOpen: "↣", PullDraft: "⇢", PullMerged: "⋈", PullClosed: "⊗",
			},
		}
	case config.IconsASCII:
		return Icons{
			Fork: "F", Private: "P", Archived: "A", Template: "T", Mirror: "M", Here: "~",
			Star: "*", Error: "x", Separator: " - ", Ellipsis: "...",
			Yes: "+", No: "x",
			// A colored dot, as the other sets have, since the star takes *.
			lang: "o",
			states: [NumStates]string{
				IssueOpen: "o", IssueClosed: "x", IssueNotPlanned: "-",
				PullOpen: "O", PullDraft: "D", PullMerged: "M", PullClosed: "X",
			},
		}
	default:
		// Octicons, which GitHub draws these with, and the language glyphs
		// of Devicons and Seti.
		return Icons{
			Fork: "\uf402", Private: "\uf456", Archived: "\uf411", Template: "\uf509", Mirror: "\uf41a", Here: "\uf46d",
			Star:  "\uf41e",
			Error: "\uf530", Separator: " · ", Ellipsis: "…",
			Yes: "\uf42e", No: "\uf467", // oct-check, oct-x
			langs: nerdLanguages,
			lang:  "\uf44f",
			files: nerdFiles,
			states: [NumStates]string{
				IssueOpen: "\uf41b", IssueClosed: "\uf41d", IssueNotPlanned: "\uf517",
				PullOpen: "\uf407", PullDraft: "\uf4dd", PullMerged: "\uf419", PullClosed: "\uf4dc",
			},
		}
	}
}

// Language returns the glyph of the language named name, as GitHub names
// it, such as "Go" or "Jupyter Notebook". Languages without a glyph of
// their own share one.
func (ic Icons) Language(name string) string {
	if g, ok := ic.langs[name]; ok {
		return g
	}
	return ic.lang
}

// State returns the glyph of state s.
func (ic Icons) State(s State) string {
	if s < 0 || s >= NumStates {
		return ic.states[IssueOpen]
	}
	return ic.states[s]
}

// Flags returns the glyphs that mark r, in the order fork, private,
// archived, template, mirror.
func (ic Icons) Flags(r core.Repo) []string {
	var flags []string
	for _, f := range []struct {
		on    bool
		glyph string
	}{
		{r.Fork, ic.Fork}, {r.Private, ic.Private}, {r.Archived, ic.Archived},
		{r.Template, ic.Template}, {r.Mirror, ic.Mirror},
	} {
		if f.on {
			flags = append(flags, f.glyph)
		}
	}
	return flags
}

// IssueState returns the state of issue is.
func IssueState(is core.Issue) State {
	switch {
	case is.State == core.StateOpen:
		return IssueOpen
	case is.Reason == core.ReasonNotPlanned || is.Reason == core.ReasonDuplicate:
		return IssueNotPlanned
	default:
		return IssueClosed
	}
}

// PullState returns the state of a pull request in state, a draft or not.
func PullState(state core.State, draft bool) State {
	switch state {
	case core.StateMerged:
		return PullMerged
	case core.StateClosed:
		return PullClosed
	default:
		if draft {
			return PullDraft
		}
		return PullOpen
	}
}

// HitState returns the state of the issue or pull request of hit.
func HitState(hit core.SearchHit) State {
	if hit.Kind == core.SearchPulls {
		return PullState(hit.Issue.State, hit.Draft)
	}
	return IssueState(hit.Issue)
}

// The colors GitHub gives the states, on light and on dark backgrounds.
var stateColors = [NumStates][2]string{
	IssueOpen:       {"#1a7f37", "#3fb950"},
	IssueClosed:     {"#8250df", "#a371f7"},
	IssueNotPlanned: {"#59636e", "#9198a1"},
	PullOpen:        {"#1a7f37", "#3fb950"},
	PullDraft:       {"#59636e", "#9198a1"},
	PullMerged:      {"#8250df", "#a371f7"},
	PullClosed:      {"#cf222e", "#f85149"},
}

// State returns the style of the glyph of state s: green while open,
// purple once merged or completed, red once closed without merging, and
// grey for drafts and issues not planned, as on GitHub.
func (t Theme) State(s State) lipgloss.Style {
	if s < 0 || s >= NumStates {
		s = IssueOpen
	}
	c := stateColors[s][0]
	if t.Dark {
		c = stateColors[s][1]
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
}

// Language returns the style of the glyph of a language: the color GitHub
// gives it, which color names when the read carried it, or the palette's
// muted color when the language has none or it would be hard to see.
func (t Theme) Language(name, color string) lipgloss.Style {
	if c := LanguageColor(name, color, t.Dark); c != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
	}
	return t.Muted
}
