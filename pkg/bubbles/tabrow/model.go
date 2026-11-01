// Package tabrow draws a row of tabs on one line, with the tab on view
// accented. Where the tabs don't fit the width it gets, it steps down a
// ladder of shorter forms, or, once asked to, scrolls to the tab on view with
// markers. It draws the row and nothing else: where the row goes, what stands
// beside it and how wide it may be are the caller's.
package tabrow

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// DefaultSeparator goes between two tabs unless WithSeparator says another.
const DefaultSeparator = "  "

// Item is one tab.
type Item struct {
	// Label names the tab in full.
	Label string
	// Short names it where Label doesn't fit. Empty means Label.
	Short string
	// Count is shown after the name when it isn't empty. The narrowest form
	// leaves it out.
	Count string
}

// Model is a tab row.
type Model struct {
	items  []Item
	cur    int
	width  int
	styles Styles
	sep    string
	// before and more mark the tabs scrolled out of view on either side; the
	// row scrolls instead of stepping down the ladder when both are set.
	before, more string
	scroll       bool
}

// Option configures a Model.
type Option func(*Model)

// WithStyles sets the styles.
func WithStyles(s Styles) Option { return func(m *Model) { m.styles = s } }

// WithSeparator sets the text between two tabs.
func WithSeparator(sep string) Option { return func(m *Model) { m.sep = sep } }

// WithMarkers makes the row scroll to the tab on view where the tabs don't
// fit, instead of stepping down the ladder. before marks tabs hidden on the
// left and more tabs hidden on the right.
func WithMarkers(before, more string) Option {
	return func(m *Model) { m.before, m.more, m.scroll = before, more, true }
}

// New returns a tab row.
func New(opts ...Option) Model {
	m := Model{styles: DefaultStyles(true), sep: DefaultSeparator}
	for _, o := range opts {
		o(&m)
	}
	return m
}

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) { m.styles = s }

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetItems sets the tabs, keeping a copy, and the one on view.
func (m *Model) SetItems(items []Item, cur int) {
	m.items = append(m.items[:0:0], items...)
	m.SetCurrent(cur)
}

// SetCurrent sets the index of the tab on view.
func (m *Model) SetCurrent(cur int) { m.cur = cur }

// SetWidth sets the width the row may take.
func (m *Model) SetWidth(w int) { m.width = max(w, 0) }

// Width returns the width the row may take.
func (m Model) Width() int { return m.width }

// View returns the row: the first form of the ladder that fits the width
// (names, short names, short names without separators, then digits with the
// current in brackets), or the row scrolled to the tab on view when markers
// are set. It is never wider than the width, and is not padded to it.
func (m Model) View() string {
	if m.width == 0 || len(m.items) == 0 {
		return ""
	}
	if m.scroll {
		return m.scrolled()
	}
	forms := []func() string{
		func() string { return m.join(m.names(false), m.sep) },
		func() string { return m.join(m.names(true), m.sep) },
		func() string { return m.join(m.names(true), " ") },
		func() string { return m.join(m.digits(), " ") },
	}
	var last string
	for _, f := range forms {
		last = f()
		if ansi.StringWidth(last) <= m.width {
			return last
		}
	}
	return ansi.Truncate(last, m.width, "")
}

// name returns the text of tab i for the form.
func (m Model) name(i int, short bool) string {
	it := m.items[i]
	label := it.Label
	if short && it.Short != "" {
		label = it.Short
	}
	if it.Count != "" {
		label += " " + it.Count
	}
	return label
}

func (m Model) names(short bool) []string {
	out := make([]string, len(m.items))
	for i := range m.items {
		out[i] = m.name(i, short)
	}
	return out
}

func (m Model) digits() []string {
	out := make([]string, len(m.items))
	for i := range m.items {
		if i == m.cur {
			out[i] = fmt.Sprintf("[%d]", i+1)
		} else {
			out[i] = strconv.Itoa(i + 1)
		}
	}
	return out
}

// paint styles text as tab i.
func (m Model) paint(i int, text string) string {
	if i == m.cur {
		return m.styles.Current.Render(text)
	}
	return m.styles.Other.Render(text)
}

// mark styles a separator or marker, leaving blank text unstyled, since
// there is nothing in it to colour.
func (m Model) mark(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return m.styles.Marker.Render(text)
}

func (m Model) join(texts []string, sep string) string {
	var b strings.Builder
	for i, t := range texts {
		if i > 0 {
			b.WriteString(m.mark(sep))
		}
		b.WriteString(m.paint(i, t))
	}
	return b.String()
}

// scrolled draws the tabs from the first that lets the current show, with a
// marker where tabs are hidden on either side.
func (m Model) scrolled() string {
	sepW := ansi.StringWidth(m.sep)
	beforeW := ansi.StringWidth(m.before) + sepW
	moreW := sepW + ansi.StringWidth(m.more)
	n := len(m.items)
	w := make([]int, n)
	for i := range w {
		w[i] = ansi.StringWidth(m.name(i, false))
	}
	cur := min(max(m.cur, 0), n-1)

	// Scroll right until the tabs from start to the current fit, with room
	// for the marker of the tabs hidden before them.
	start := 0
	span := func(from int) int {
		t := 0
		for i := from; i <= cur; i++ {
			t += w[i]
			if i > from {
				t += sepW
			}
		}
		return t
	}
	for start < cur {
		room := m.width
		if start > 0 {
			room -= beforeW
		}
		if cur < n-1 {
			room -= moreW
		}
		if span(start) <= room {
			break
		}
		start++
	}

	var b strings.Builder
	used := 0
	if start > 0 {
		b.WriteString(m.styles.Marker.Render(m.before + m.sep))
		used = beforeW
	}
	end := start
	for end < n {
		add := w[end]
		if end > start {
			add += sepW
		}
		// Tabs hidden after this one need room for their marker, unless
		// this is the last.
		reserve := 0
		if end < n-1 {
			reserve = moreW
		}
		if used+add+reserve > m.width && end > start && end > cur {
			break
		}
		if end > start {
			b.WriteString(m.mark(m.sep))
		}
		b.WriteString(m.paint(end, m.name(end, false)))
		used += add
		end++
	}
	if end < n {
		b.WriteString(m.styles.Marker.Render(m.sep + m.more))
	}
	return ansi.Truncate(b.String(), m.width, "")
}
