package issues

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// View implements ui.Section. It renders exactly the section's size.
func (s *Section) View() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	if !s.hasRepo {
		return s.empty
	}
	if s.height == 1 {
		return s.bar
	}
	return s.bar + "\n" + s.list.View()
}

// renderChrome renders what surrounds the bubbles: the bar over the list,
// with the repository and the tabs, and the empty state.
func (s *Section) renderChrome() {
	s.cols, s.colsWidth = layout(max(s.width-2, 0)), max(s.width-2, 0)
	s.renderBar()
	s.renderEmpty()
}

func (s *Section) renderBar() {
	t := s.theme
	left := "  " + t.Muted.Render(s.repo.String())
	var right strings.Builder
	for i, tb := range tabs {
		if i > 0 {
			right.WriteString(t.Subtle.Render(" · "))
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
	s.bar = spread(left, right.String(), s.width)
}

// spread puts left and right at the two ends of a line width cells wide,
// cutting left if both don't fit.
func spread(left, right string, width int) string {
	rw := ansi.StringWidth(right)
	if rw+1 > width {
		return fitStyled(left, width)
	}
	left = ansi.Truncate(left, width-rw-1, "…")
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-rw) + right
}

// fitStyled truncates or pads styled text to exactly width cells.
func fitStyled(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

// renderEmpty renders the state shown before a repository is selected,
// wrapped to fit a narrow pane.
func (s *Section) renderEmpty() {
	if s.width <= 0 || s.height <= 0 {
		s.empty = ""
		return
	}
	t := s.theme
	center := lipgloss.NewStyle().Width(s.width).Align(lipgloss.Center)
	text := lipgloss.JoinVertical(lipgloss.Left,
		center.Inherit(t.Title).Render("No repository selected"),
		"",
		center.Inherit(t.Muted).Render(s.hint),
	)
	lines := strings.Split(lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, text), "\n")
	lines = lines[:min(len(lines), s.height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, s.width, "")
	}
	s.empty = strings.Join(lines, "\n")
}

// emptyText is what the list says when no issue is in the tab, with the
// key that shows more.
func (s *Section) emptyText() string {
	kind := "No " + strings.ToLower(tabLabel(s.tab)) + " issues"
	if s.tab == core.FilterAll {
		kind = "No issues"
	}
	if s.query != "" {
		text := kind + " match the filters."
		if k := s.keys.ClearFilter; k.Enabled() {
			text += " Press " + k.Help().Key + " to clear them."
		}
		return text
	}
	if s.tab == core.FilterAll {
		return "No issues yet."
	}
	text := kind + "."
	if k := s.keys.NextTab; k.Enabled() {
		text += " Press " + k.Help().Key + " to see " + strings.ToLower(tabLabel(nextTab(s.tab, 1))) + " issues."
	}
	return text
}
