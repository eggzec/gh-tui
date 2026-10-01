package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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

// repoCols are the widths of the columns of the repositories, which every
// row of a tab shares so that they line up. A width of 0 drops a column:
// narrow panes drop the description first, then the language.
type repoCols struct {
	name, flags, desc, lang, stars, age int
}

// repoMeasure is what the repositories read so far need: the widest name,
// the most flags and the widest count of stars. It only grows, so the
// columns don't jump as the list scrolls.
type repoMeasure struct {
	name, flags, stars int
	// seen is how many repositories were measured.
	seen int
}

func (m *repoMeasure) add(r core.Repo, icons ui.Icons) {
	m.name = max(m.name, ansi.StringWidth(r.Ref.Name))
	m.flags = max(m.flags, len(icons.Flags(r)))
	m.stars = max(m.stars, len(count(r.Stars)))
}

// layoutCols fits the columns of repositories measured by m in width
// cells, with dates of age cells at most.
func layoutCols(width int, m repoMeasure, star string, age int) repoCols {
	c := repoCols{
		lang:  len(langHeader),
		stars: max(m.stars, ansi.StringWidth(star)),
		age:   max(len(ageHeader), age),
	}
	if m.flags > 0 {
		c.flags = 2*m.flags - 1
	}
	name := max(m.name, len(nameHeader))
	c.name = min(name, max(width*2/5, minName), maxName)
	if d := width - c.width() - colGap; d >= minDesc {
		c.desc = d
		return c
	}
	// Without a description the name takes the rest, so the counts stay at
	// the right edge.
	c.name = 0
	if n := width - c.width() - colGap; n >= min(name, minName) {
		c.name = n
		return c
	}
	c.lang = 0
	c.name = max(width-c.width()-colGap, 0)
	return c
}

// width is how many cells the columns take with their gaps.
func (c repoCols) width() int {
	n, cols := 0, 0
	for _, w := range []int{c.name, c.flags, c.desc, c.lang, c.stars, c.age} {
		if w > 0 {
			n += w
			cols++
		}
	}
	return n + colGap*max(cols-1, 0)
}

// header renders the headers of the columns.
func (c repoCols) header(star string) string {
	var b strings.Builder
	cell := func(text string, w int, right bool) {
		if w <= 0 {
			return
		}
		if b.Len() > 0 {
			b.WriteString(strings.Repeat(" ", colGap))
		}
		text = truncate(text, w)
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

// renderRepo renders a repository of the list in the columns of the tab:
// its name, flags, description, language, stars and age.
func (s *Section) renderRepo(c repoCols, r core.Repo, selected bool) string {
	st := &s.st
	var b strings.Builder
	b.Grow(c.width() + 96)
	first := true
	gap := func() {
		if !first {
			b.WriteString(strings.Repeat(" ", colGap))
		}
		first = false
	}
	if c.name > 0 {
		gap()
		name := truncate(r.Ref.Name, c.name)
		nameStyle := st.text
		if selected {
			nameStyle = st.selected
		}
		b.WriteString(s.links.Link(s.repoURL(r), nameStyle.render(name)))
		b.WriteString(strings.Repeat(" ", c.name-ansi.StringWidth(name)))
	}
	if c.flags > 0 {
		gap()
		used := 0
		for i, f := range s.icons.Flags(r) {
			if i > 0 {
				b.WriteByte(' ')
				used++
			}
			st.subtle.write(&b, f)
			used += ansi.StringWidth(f)
		}
		b.WriteString(strings.Repeat(" ", max(c.flags-used, 0)))
	}
	if c.desc > 0 {
		gap()
		d := truncate(cleanLine(r.Description), c.desc)
		st.muted.write(&b, d)
		b.WriteString(strings.Repeat(" ", c.desc-ansi.StringWidth(d)))
	}
	if c.lang > 0 {
		gap()
		used := 0
		if r.Language != "" {
			g := s.icons.Language(r.Language)
			s.langPaint(r).write(&b, g)
			used = ansi.StringWidth(g)
		}
		b.WriteString(strings.Repeat(" ", max(c.lang-used, 0)))
	}
	if c.stars > 0 {
		gap()
		n := count(r.Stars)
		b.WriteString(strings.Repeat(" ", max(c.stars-len(n), 0)))
		st.muted.write(&b, n)
	}
	if c.age > 0 {
		gap()
		age := ""
		if !r.UpdatedAt.IsZero() {
			age = s.dates.Short(r.UpdatedAt, s.now())
		}
		b.WriteString(strings.Repeat(" ", max(c.age-ansi.StringWidth(age), 0)))
		st.subtle.write(&b, age)
	}
	return b.String()
}

// langPaint returns the paint of the language glyph of r, built once per
// language and theme.
func (s *Section) langPaint(r core.Repo) paint {
	k := r.Language + "\x00" + r.LanguageColor
	if p, ok := s.st.langs[k]; ok {
		return p
	}
	p := newPaint(s.theme.Language(r.Language, r.LanguageColor))
	if s.st.langs == nil {
		s.st.langs = map[string]paint{}
	}
	s.st.langs[k] = p
	return p
}
