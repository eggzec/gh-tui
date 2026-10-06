package issues

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View implements ui.Section. It renders exactly the section's size.
func (s *Section) View() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	if !s.hasRepo {
		return s.empty
	}
	if s.issuesOff() {
		return s.off
	}
	if s.height == 1 {
		return s.bar
	}
	return s.bar + "\n" + s.list.View()
}

// renderChrome renders what surrounds the bubbles: the bar over the list,
// with the repository and the tabs, and the empty state.
func (s *Section) renderChrome() {
	s.cols, s.colsWidth = layout(max(s.width-2, 0), s.room, s.dates.Width()), max(s.width-2, 0)
	s.renderBar()
	s.renderEmpty()
	s.renderOff()
}

func (s *Section) renderBar() {
	t := s.theme
	left := "  " + t.Muted.Render(s.repo.String())
	var right strings.Builder
	for i, tb := range tabs {
		if i > 0 {
			right.WriteString(t.Subtle.Render(s.icons.Separator))
		}
		st := t.Subtle
		if tb.state == s.tab {
			st = t.Accent
		}
		right.WriteString(st.Render(tb.label))
	}
	// Narrow panes name only the tab shown, and the narrowest only the
	// repository.
	if rw := ansi.StringWidth(right.String()); ansi.StringWidth(left)+rw+2 > s.width {
		right.Reset()
		right.WriteString(t.Accent.Render(tabLabel(s.tab)))
	}
	s.bar = spread(left, right.String(), s.width, s.icons.Ellipsis)
}

// spread puts left and right at the two ends of a line width cells wide,
// cutting left to end in tail if both don't fit.
func spread(left, right string, width int, tail string) string {
	rw := ansi.StringWidth(right)
	if rw+1 > width {
		return fitStyled(left, width, tail)
	}
	left = termtext.Truncate(left, width-rw-1, tail)
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-rw) + right
}

// fitStyled truncates styled text to exactly width cells, ending in tail,
// or pads it.
func fitStyled(s string, width int, tail string) string {
	s = termtext.Truncate(s, width, tail)
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

// renderEmpty renders the state shown before a repository is selected,
// wrapped to fit a narrow pane.
func (s *Section) renderEmpty() {
	s.empty = s.placard("No repository selected", s.hint)
}

// renderOff renders the state shown in place of the list when the
// repository turned its issues off, which only an admin can undo.
func (s *Section) renderOff() {
	if !s.issuesOff() {
		s.off = ""
		return
	}
	hint := "The repository doesn't use GitHub's issues."
	if s.caps.Permission == core.PermissionAdmin {
		hint = "You can turn them on in its settings on GitHub."
	}
	s.off = s.placard("Issues are turned off for "+s.repo.String(), hint)
}

// placard renders title over hint in the middle of the section, wrapped
// to fit a narrow pane.
func (s *Section) placard(title, hint string) string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	t := s.theme
	center := lipgloss.NewStyle().Width(s.width).Align(lipgloss.Center)
	text := lipgloss.JoinVertical(lipgloss.Left,
		center.Inherit(t.Title).Render(title),
		"",
		center.Inherit(t.Muted).Render(hint),
	)
	lines := strings.Split(lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, text), "\n")
	lines = lines[:min(len(lines), s.height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, s.width, "")
	}
	return strings.Join(lines, "\n")
}

// emptyText is what the list says when no issue is in the tab, with the
// key that shows more.
func (s *Section) emptyText() string {
	kind := strings.ToLower(tabLabel(s.tab)) + " issues"
	if s.tab == core.FilterAll {
		kind = "issues"
	}
	if s.query != "" {
		return ui.NoMatch(kind, ui.KeyOf(s.icons, s.keys.ClearFilter))
	}
	text := ui.None(kind)
	switch next := nextTab(s.tab, 1); {
	case s.tab == core.FilterAll:
		return text
	case next == core.FilterAll:
		return ui.Press(text, ui.KeyOf(s.icons, s.keys.NextTab), "show all of them")
	default:
		return ui.Press(text, ui.KeyOf(s.icons, s.keys.NextTab), "show "+strings.ToLower(tabLabel(next))+" ones")
	}
}
