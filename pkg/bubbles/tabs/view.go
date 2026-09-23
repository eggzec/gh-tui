package tabs

import (
	"cmp"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Spacing of the bar. When the titles don't fit, the bar drops to the compact
// spacing first and shortens inactive titles only after that.
const (
	margin     = 1
	gap        = 4
	compactGap = 2
)

// View returns the bar and the rule beneath it, two lines that are exactly
// as wide as the width set with [Model.SetWidth].
func (m Model) View() string { return m.view }

// Height returns the number of lines View renders.
func (Model) Height() int { return 2 }

// label is a whole title and its badge, rendered as an inactive and as the
// active tab.
type label struct {
	width  int
	tab    string
	active string
	// title is the width of the title alone, and badge the rendered
	// inactive badge with its leading space, for shortening the title.
	title int
	badge string
}

// prepare renders the whole titles and their badges. It runs whenever the
// tabs, the badges or the styles change.
func (m *Model) prepare() {
	m.badges = m.badges[:min(len(m.badges), len(m.tabs))]
	// A new slice, since copies of a Model share the old one.
	m.labels = make([]label, 0, len(m.tabs))
	for i, t := range m.tabs {
		l := label{
			title:  ansi.StringWidth(t),
			tab:    m.styles.Tab.Render(t),
			active: m.styles.Active.Render(t),
		}
		l.width = l.title
		if b := m.Badge(i); b != "" {
			l.badge = " " + m.styles.Badge.Render(b)
			l.width += 1 + ansi.StringWidth(b)
			l.tab += l.badge
			l.active += " " + m.styles.ActiveBadge.Render(b)
		}
		m.labels = append(m.labels, l)
	}
}

// render rebuilds the cached view. It runs whenever the tabs, the active tab,
// the width or the styles change.
func (m *Model) render() {
	labels := m.labels
	lead, sep := margin, gap
	if m.width > 0 && m.naturalWidth(lead, sep) > m.width {
		lead, sep = 0, compactGap
		if m.naturalWidth(lead, sep) > m.width {
			labels = m.shorten(m.width - sep*max(len(m.tabs)-1, 0))
		}
	}

	var b strings.Builder
	b.Grow(len(m.view))
	writeSpaces(&b, lead)
	x, ax, aw := lead, 0, 0
	for i, l := range labels {
		if i > 0 {
			writeSpaces(&b, sep)
			x += sep
		}
		if i == m.active {
			ax, aw = x, l.width
			b.WriteString(l.active)
		} else {
			b.WriteString(l.tab)
		}
		x += l.width
	}

	total := m.width
	if total <= 0 {
		total = x
	}
	if x > total {
		line := ansi.Truncate(b.String(), total, m.styles.Ellipsis)
		b.Reset()
		b.WriteString(line)
	} else {
		writeSpaces(&b, total-x)
	}
	b.WriteByte('\n')

	ax = min(ax, total)
	aw = min(aw, total-ax)
	writeRule(&b, m.styles.Rule, m.styles.RuleChar, ax)
	writeRule(&b, m.styles.Indicator, m.styles.IndicatorChar, aw)
	writeRule(&b, m.styles.Rule, m.styles.RuleChar, total-ax-aw)
	m.view = b.String()
}

func writeSpaces(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

func writeRule(b *strings.Builder, s lipgloss.Style, char string, n int) {
	if n > 0 {
		b.WriteString(s.Render(strings.Repeat(char, n)))
	}
}

// naturalWidth is the width of the bar with whole titles.
func (m *Model) naturalWidth(lead, sep int) int {
	w := lead + sep*max(len(m.tabs)-1, 0)
	for _, l := range m.labels {
		w += l.width
	}
	return w
}

// shorten fits the labels into avail columns. The active title and every
// badge stay whole, short titles stay whole, and the longer ones share what
// is left evenly.
func (m *Model) shorten(avail int) []label {
	out := slices.Clone(m.labels)
	rest := avail - m.labels[m.active].width
	idx := make([]int, 0, len(m.tabs)-1)
	for i, l := range m.labels {
		if i != m.active {
			idx = append(idx, i)
			rest -= l.width - l.title
		}
	}
	width := func(i int) int { return m.labels[i].title }
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(width(a), width(b)) })
	for j, i := range idx {
		if width(i) <= rest/(len(idx)-j) {
			rest -= width(i)
			continue
		}
		long := idx[j:]
		slices.Sort(long)
		base, extra := rest/len(long), rest%len(long)
		for k, i := range long {
			n := base
			if k < extra {
				n++
			}
			l := m.labels[i]
			t := m.ellipsize(m.tabs[i], n)
			tw := ansi.StringWidth(t)
			out[i] = label{width: l.width - l.title + tw, title: tw, tab: m.styles.Tab.Render(t) + l.badge}
		}
		break
	}
	return out
}

// ellipsize shortens title to at most n columns, ending it with the ellipsis
// rather than a space.
func (m *Model) ellipsize(title string, n int) string {
	keep := n - ansi.StringWidth(m.styles.Ellipsis)
	return strings.TrimRight(ansi.Truncate(title, max(keep, 0), ""), " ") + m.styles.Ellipsis
}
