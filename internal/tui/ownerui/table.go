package ownerui

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// The headers of the columns of the repositories. The flags have none:
// their glyphs speak for themselves.
const (
	nameHeader = "Name"
	descHeader = "Description"
	langHeader = "Lang"
	ageHeader  = "Updated"
)

// Bounds of the columns of the repositories: the gap between two, the
// widest a name gets while there is a description, and the narrowest a
// description or a name may be before the columns give way.
const (
	colGap  = 2
	maxName = 30
	minDesc = 12
	minName = 12
)

// Cols are the widths of the columns of the repositories, which every row
// of a table shares so that they line up. A width of 0 drops a column:
// narrow tables drop the description first, then the language.
type Cols struct {
	name, flags, desc, lang, stars, age int
	// full names the repositories by owner and name.
	full bool
}

// Measure is what the repositories read so far need: the widest name, the
// most flags and the widest count of stars. It only grows, so the columns
// don't jump as the list scrolls.
type Measure struct {
	name, flags, stars int
	// seen is how many repositories were measured.
	seen int
}

// Add measures r too.
func (m *Measure) Add(r core.Repo, icons ui.Icons) { m.add(r, icons, false) }

// add measures r too, named by owner and name if full is set.
func (m *Measure) add(r core.Repo, icons ui.Icons, full bool) {
	m.name = max(m.name, ansi.StringWidth(repoName(r, full)))
	m.flags = max(m.flags, len(icons.Flags(r)))
	m.stars = max(m.stars, len(Count(r.Stars)))
}

// LayoutCols fits the columns of repositories measured by m in width
// cells, with dates of age cells at most.
func LayoutCols(width int, m Measure, star string, age int) Cols {
	c := Cols{
		lang:  len(langHeader),
		stars: max(m.stars, ansi.StringWidth(star)),
		age:   max(len(ageHeader), age),
	}
	if m.flags > 0 {
		c.flags = 2*m.flags - 1
	}
	name := max(m.name, len(nameHeader))
	c.name = min(name, max(width*2/5, minName), maxName)
	if d := width - c.Width() - colGap; d >= minDesc {
		c.desc = d
		return c
	}
	// Without a description the name takes the rest, so the counts stay at
	// the right edge.
	c.name = 0
	if n := width - c.Width() - colGap; n >= min(name, minName) {
		c.name = n
		return c
	}
	c.lang = 0
	c.name = max(width-c.Width()-colGap, 0)
	return c
}

// Width is how many cells the columns take with their gaps.
func (c Cols) Width() int {
	n, cols := 0, 0
	for _, w := range []int{c.name, c.flags, c.desc, c.lang, c.stars, c.age} {
		if w > 0 {
			n += w
			cols++
		}
	}
	return n + colGap*max(cols-1, 0)
}

// Header renders the headers of the columns, the stars' headed by star and
// those cut short ending in tail.
func (c Cols) Header(star, tail string) string {
	var b strings.Builder
	cell := func(text string, w int, right bool) {
		if w <= 0 {
			return
		}
		if b.Len() > 0 {
			b.WriteString(strings.Repeat(" ", colGap))
		}
		text = Truncate(text, w, tail)
		pad := strings.Repeat(" ", w-ansi.StringWidth(text))
		if right {
			b.WriteString(pad + text)
		} else {
			b.WriteString(text + pad)
		}
	}
	cell(nameHeader, c.name, false)
	cell("", c.flags, false)
	cell(descHeader, c.desc, false)
	cell(langHeader, c.lang, false)
	cell(star, c.stars, true)
	cell(ageHeader, c.age, true)
	return b.String()
}

// Row renders a repository of a table in its columns c: its name, flags,
// description, language, stars and age, measured against now in dates.
func (d Drawer) Row(c Cols, r core.Repo, selected bool, dates ui.Dates, now func() time.Time) string {
	st := d.Styles
	var b strings.Builder
	b.Grow(c.Width() + 96)
	first := true
	gap := func() {
		if !first {
			b.WriteString(strings.Repeat(" ", colGap))
		}
		first = false
	}
	if c.name > 0 {
		gap()
		name := Truncate(repoName(r, c.full), c.name, d.Icons.Ellipsis)
		nameStyle := st.Text
		if selected {
			nameStyle = st.Selected
		}
		b.WriteString(d.Links.Link(d.URL(r), nameStyle.Render(name)))
		b.WriteString(strings.Repeat(" ", c.name-ansi.StringWidth(name)))
	}
	if c.flags > 0 {
		gap()
		used := 0
		for i, f := range d.Icons.Flags(r) {
			if i > 0 {
				b.WriteByte(' ')
				used++
			}
			st.Subtle.Write(&b, f)
			used += ansi.StringWidth(f)
		}
		b.WriteString(strings.Repeat(" ", max(c.flags-used, 0)))
	}
	if c.desc > 0 {
		gap()
		desc := Truncate(CleanLine(r.Description), c.desc, d.Icons.Ellipsis)
		st.Muted.Write(&b, desc)
		b.WriteString(strings.Repeat(" ", c.desc-ansi.StringWidth(desc)))
	}
	if c.lang > 0 {
		gap()
		used := 0
		if r.Language != "" {
			g := d.Icons.Language(r.Language)
			d.Lang(r).Write(&b, g)
			used = ansi.StringWidth(g)
		}
		b.WriteString(strings.Repeat(" ", max(c.lang-used, 0)))
	}
	if c.stars > 0 {
		gap()
		n := Count(r.Stars)
		b.WriteString(strings.Repeat(" ", max(c.stars-len(n), 0)))
		st.Muted.Write(&b, n)
	}
	if c.age > 0 {
		gap()
		age := ""
		if !r.UpdatedAt.IsZero() {
			age = dates.Short(r.UpdatedAt, now())
		}
		b.WriteString(strings.Repeat(" ", max(c.age-ansi.StringWidth(age), 0)))
		st.Subtle.Write(&b, age)
	}
	return b.String()
}

// Table is a list of the repositories of an owner in columns, which a
// filter narrows and orders. Its page owns the feed, and builds it with a
// render that draws Cols and a read that consults Filter.
type Table struct {
	Feed feed.Model[core.Repo]

	// measure sizes cols, the columns of the list.
	measure Measure
	cols    Cols

	// filter is the filter of the table, which the feed's reads use in
	// their commands.
	filter atomic.Pointer[Filter]

	// FullNames names the repositories by owner and name, for a list of
	// repositories of several owners, such as those a user starred.
	FullNames bool
}

// repoName is the name of r in a table: its name, or its owner and name
// if full is set.
func repoName(r core.Repo, full bool) string {
	if full {
		return r.Ref.String()
	}
	return r.Ref.Name
}

// Cols returns the columns of the table as they are laid out.
func (t *Table) Cols() Cols { return t.cols }

// keepAll is what Filter returns when none was set. Nothing changes a
// filter once it is made, so every table may share it.
var keepAll = &Filter{}

// Filter returns the filter in force, which keeps everything when none
// was set.
func (t *Table) Filter() *Filter {
	if f := t.filter.Load(); f != nil {
		return f
	}
	return keepAll
}

// SetFilter puts f in force for the reads that follow.
func (t *Table) SetFilter(f *Filter) { t.filter.Store(f) }

// Remeasure measures the repositories read since it last did, and reports
// whether the columns need laying out again.
func (t *Table) Remeasure(icons ui.Icons) bool {
	n := t.Feed.Len()
	if n == t.measure.seen {
		return false
	}
	m := t.measure
	for i := range n {
		if r, ok := t.Feed.Item(i); ok {
			m.add(r, icons, t.FullNames)
		}
	}
	m.seen = n
	if m == t.measure {
		return false
	}
	t.measure = m
	return true
}

// Layout fits the columns in width cells, the stars' headed by star and
// the dates age cells at most.
func (t *Table) Layout(width int, star string, age int) {
	t.cols = LayoutCols(width, t.measure, star, age)
	t.cols.full = t.FullNames
}
