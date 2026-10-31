package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Titles of the sections, which name their panes. Sections refer to each
// other by title, for example to show pull requests once a repository is
// chosen.
const (
	PullsTitle         = "Pull requests"
	IssuesTitle        = "Issues"
	NotificationsTitle = "Notifications"
	FilesTitle         = "Files"
	DashboardTitle     = "Dashboard"
	SearchTitle        = "Search"
	OwnerTitle         = "Profile"
)

// TitleSpec says what a pane or modal title shows: [k] Name, the chips, and
// the count at the right.
type TitleSpec struct {
	// Key is the key that jumps to the pane, or "" where none does.
	Key  string
	Name string
	// Chips follow the name, each after Sep.
	Chips []string
	// Count is put against the right edge of Width, or after the chips,
	// after Sep, when Width is 0.
	Count string
	// Width is the cells of the line, which Title fills exactly; 0 leaves
	// the title as long as it is.
	Width int
	// Sep goes between the name and the chips, and Ellipsis ends a cut name.
	Sep, Ellipsis string
	// NameStyle, ChipStyle and CountStyle style the parts; nil leaves a
	// part plain; a lipgloss style's Render fits. The key is styled with
	// the name.
	NameStyle, ChipStyle, CountStyle func(...string) string
	// SepStyle styles each Sep written between parts; nil leaves it plain.
	SepStyle func(...string) string
}

// Title builds a pane or modal title. When it does not fit Width it drops
// the chips from the last, then the key, then cuts the name, so that the
// count stays whole while it fits.
func Title(s TitleSpec) string {
	style := func(f func(...string) string, t string) string {
		if f == nil {
			return t
		}
		return f(t)
	}
	count := ""
	if s.Count != "" {
		count = style(s.CountStyle, s.Count)
	}
	sep := style(s.SepStyle, s.Sep)
	build := func(chips int, key bool) string {
		name := s.Name
		if key && s.Key != "" {
			name = "[" + s.Key + "] " + name
		}
		var line strings.Builder
		line.WriteString(style(s.NameStyle, name))
		for _, c := range s.Chips[:chips] {
			line.WriteString(sep)
			line.WriteString(style(s.ChipStyle, c))
		}
		return line.String()
	}
	if s.Width <= 0 {
		line := build(len(s.Chips), true)
		if count != "" {
			line += sep + count
		}
		return line
	}
	cw := ansi.StringWidth(count)
	fits := func(l string) bool {
		w := ansi.StringWidth(l)
		if cw > 0 {
			w += 1 + cw
		}
		return w <= s.Width
	}
	for n := len(s.Chips); n >= 0; n-- {
		if l := build(n, true); fits(l) {
			return Spread(l, count, s.Width, s.Ellipsis)
		}
	}
	l := build(0, false)
	if cw == 0 {
		return Fit(termtext.Truncate(l, s.Width, s.Ellipsis), s.Width)
	}
	return Spread(l, count, s.Width, s.Ellipsis)
}
