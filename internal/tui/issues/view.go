package issues

import (
	"strconv"
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
	if s.inDetail {
		return s.bar + "\n" + s.detail.View()
	}
	return s.bar + "\n" + s.list.View()
}

// filters are the states the filter cycles through, in order.
var filters = []core.StateFilter{core.FilterOpen, core.FilterClosed, core.FilterAll}

func nextFilter(f core.StateFilter) core.StateFilter {
	for i, g := range filters {
		if g == f {
			return filters[(i+1)%len(filters)]
		}
	}
	return filters[0]
}

// renderChrome renders what surrounds the bubbles: the bar over the list,
// with the repository and the filter, and the empty state.
func (s *Section) renderChrome() {
	s.cols, s.colsWidth = layout(max(s.width-2, 0)), max(s.width-2, 0)
	s.renderBar()
	s.renderEmpty()
}

func (s *Section) renderBar() {
	t := s.theme
	left := "  " + t.Muted.Render(s.repo.String())
	if s.inDetail {
		left += t.Subtle.Render(" › ") + t.Muted.Render("#"+strconv.Itoa(s.issue.Number))
		s.bar = fitStyled(left, s.width)
		return
	}
	var right strings.Builder
	for i, f := range filters {
		if i > 0 {
			right.WriteString(t.Subtle.Render(" · "))
		}
		st := t.Subtle
		if f == s.filter {
			st = t.Accent
		}
		right.WriteString(st.Render(string(f)))
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

// renderEmpty renders the state shown before a repository is selected.
func (s *Section) renderEmpty() {
	if s.width <= 0 || s.height <= 0 {
		s.empty = ""
		return
	}
	t := s.theme
	text := lipgloss.JoinVertical(lipgloss.Center,
		ansi.Truncate(t.Title.Render("No repository selected"), s.width, "…"),
		"",
		ansi.Truncate(t.Muted.Render("Pick a repository in Repositories (tab 4)."), s.width, "…"),
	)
	s.empty = lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, text)
}

// emptyText is what the list says when no issue matches the filter, with
// the key that changes the filter.
func (s *Section) emptyText() string {
	hint := ""
	if k := s.keys.Filter.Help().Key; k != "" {
		hint = " Press " + k + " to see " + string(nextFilter(s.filter)) + " issues."
	}
	switch s.filter {
	case core.FilterClosed:
		return "No closed issues." + hint
	case core.FilterAll:
		return "No issues yet."
	default:
		return "No open issues." + hint
	}
}
