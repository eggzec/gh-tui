package repos

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// A row reads, in fixed-width columns:
//
//	▪● ★ owner/name          description…        archived Go          1.2k   3d
//
// The square marks a pinned repository, and its column is there only when
// some are pinned. The dot marks the current repository. The description
// gives way first, then the columns on its right.

const (
	glyphStarred   = "★"
	glyphUnstarred = "☆"
	glyphCurrent   = "●"
	glyphPinned    = "▪"

	// feedGutter is the width the list keeps left of every row for its
	// cursor.
	feedGutter = 2
	// leadWidth holds the current marker and the star, each followed by a
	// space. The pin column adds one cell before them.
	leadWidth = 4

	tagWidth   = 8 // "archived"
	langWidth  = 10
	starsWidth = 5 // "12.3k"
	ageWidth   = 4 // "11mo"

	minName = 12
	maxName = 32
	// minDesc is the narrowest description worth showing. Below it the
	// columns on the right go first, then the description.
	minDesc = 12
)

// styles are built once per theme, with the fixed fragments rendered.
type styles struct {
	owner, name, desc, lang, stars, age lipgloss.Style

	starred, unstarred, current, pinned string
	// tags are padded to tagWidth.
	private, fork, archived, noTag string
}

func newStyles(t ui.Theme) styles {
	tag := func(s string) string {
		return t.Muted.Render(s + strings.Repeat(" ", tagWidth-len(s)))
	}
	return styles{
		owner:     t.Muted,
		name:      t.Title,
		desc:      t.Subtle,
		lang:      t.Muted,
		stars:     t.Muted,
		age:       t.Subtle,
		starred:   t.Accent.Render(glyphStarred),
		unstarred: t.Subtle.Render(glyphUnstarred),
		current:   t.Subtle.Render(glyphCurrent),
		pinned:    t.Subtle.Render(glyphPinned),
		private:   tag("private"),
		fork:      tag("fork"),
		archived:  tag("archived"),
		noTag:     strings.Repeat(" ", tagWidth),
	}
}

// layout is the width of each column for a row width. Columns on the right
// that don't fit are dropped, least important first.
type layout struct {
	width int
	// lead is the width left of the name.
	lead int
	name int
	// desc is 0 when the description is dropped.
	desc                   int
	tag, lang, stars, ages bool
}

func newLayout(width, lead int) layout {
	l := layout{width: width, lead: lead, tag: true, lang: true, stars: true, ages: true}
	l.name = min(max(width*2/7, minName), maxName)
	// The description is what gives: language goes first, then age, then
	// the tag.
	drops := []*bool{&l.lang, &l.ages, &l.tag}
	for {
		l.desc = width - l.lead - l.name - 2 - l.right()
		if l.desc >= minDesc || len(drops) == 0 {
			break
		}
		*drops[0] = false
		drops = drops[1:]
	}
	if l.desc < minDesc {
		// Too narrow for a description: the name takes the room left,
		// and the star count goes too when even the name won't fit.
		l.desc = 0
		l.name = width - l.lead - l.right()
		if l.name < minName {
			l.stars = false
			l.name = width - l.lead
		}
		l.name = max(l.name, 0)
	}
	return l
}

// right returns the width of the enabled columns right of the description,
// each with the gap before it.
func (l layout) right() int {
	w := 0
	if l.tag {
		w += 2 + tagWidth
	}
	if l.lang {
		w += 1 + langWidth
	}
	if l.stars {
		w += 1 + starsWidth
	}
	if l.ages {
		w += 1 + ageWidth
	}
	return w
}

// lead returns the width left of the names.
func (s *Section) lead() int {
	if len(s.pinned) > 0 {
		return leadWidth + 1
	}
	return leadWidth
}

// rowWidth is the width the list gives a row in a section width wide.
func rowWidth(width int) int {
	return max(width-feedGutter, 0)
}

// maxCachedRows bounds the rendered rows kept, a few pages' worth.
const maxCachedRows = 512

// cachedRow is a rendered row and what it was rendered from.
type cachedRow struct {
	repo    core.Repo
	width   int
	current bool
	age     string
	line    string
}

// render draws one repository on one line of width cells. Styling a row
// costs far more than comparing it, so rows are drawn again only when they
// change; SetTheme forgets them.
func (s *Section) render(r core.Repo, _ bool, width int) string {
	age := ""
	if !r.UpdatedAt.IsZero() {
		age = ui.Ago(r.UpdatedAt, s.now())
	}
	current := sameRef(r.Ref, s.current)
	if c, ok := s.rows[r.Ref]; ok && c.width == width && c.current == current && c.age == age && c.repo == r {
		return c.line
	}
	line := s.draw(r, width, current, age)
	if len(s.rows) >= maxCachedRows {
		clear(s.rows)
	}
	s.rows[r.Ref] = cachedRow{repo: r, width: width, current: current, age: age, line: line}
	return line
}

func (s *Section) draw(r core.Repo, width int, current bool, age string) string {
	l := s.cols
	if l.width != width {
		l = newLayout(width, s.lead())
	}
	st := &s.styles

	var b strings.Builder
	b.Grow(width + 128)
	switch {
	case s.isPinned(r.Ref):
		b.WriteString(st.pinned)
	case len(s.pinned) > 0:
		b.WriteByte(' ')
	}
	if current {
		b.WriteString(st.current)
	} else {
		b.WriteByte(' ')
	}
	b.WriteByte(' ')
	if r.Starred {
		b.WriteString(st.starred)
	} else {
		b.WriteString(st.unstarred)
	}
	b.WriteByte(' ')
	s.writeName(&b, r.Ref, l.name)

	if l.desc > 0 {
		b.WriteString("  ")
		writeCell(&b, st.desc, oneLine(r.Description), l.desc, false)
	}
	if l.tag {
		b.WriteString("  ")
		switch {
		case r.Archived:
			b.WriteString(st.archived)
		case r.Private:
			b.WriteString(st.private)
		case r.Fork:
			b.WriteString(st.fork)
		default:
			b.WriteString(st.noTag)
		}
	}
	if l.lang {
		b.WriteByte(' ')
		writeCell(&b, st.lang, r.Language, langWidth, false)
	}
	if l.stars {
		b.WriteByte(' ')
		count := ""
		if r.Stars > 0 {
			count = compact(r.Stars)
		}
		writeCell(&b, st.stars, count, starsWidth, true)
	}
	if l.ages {
		b.WriteByte(' ')
		writeCell(&b, st.age, age, ageWidth, true)
	}
	return b.String()
}

// writeName writes owner/name in width cells. A long owner is shortened
// before the name, and dropped when the name alone barely fits.
func (s *Section) writeName(b *strings.Builder, ref core.RepoRef, width int) {
	owner, name := ref.Owner+"/", ref.Name
	ow, nw := ansi.StringWidth(owner), ansi.StringWidth(name)
	if ow+nw > width {
		switch room := width - nw; {
		case room >= 4:
			owner = ansi.Truncate(owner, room-1, "…") + "/"
		default:
			owner = ""
			name = ansi.Truncate(name, width, "…")
		}
		ow, nw = ansi.StringWidth(owner), ansi.StringWidth(name)
	}
	if owner != "" {
		b.WriteString(s.styles.owner.Render(owner))
	}
	b.WriteString(s.styles.name.Render(name))
	pad(b, width-ow-nw)
}

// writeCell writes text in style, truncated or padded to width, on the
// right when alignRight is set. Blank cells are written without escapes.
func writeCell(b *strings.Builder, st lipgloss.Style, text string, width int, alignRight bool) {
	w := ansi.StringWidth(text)
	if w > width {
		text = ansi.Truncate(text, width, "…")
		w = ansi.StringWidth(text)
	}
	if w == 0 {
		pad(b, width)
		return
	}
	if alignRight {
		pad(b, width-w)
	}
	b.WriteString(st.Render(text))
	if !alignRight {
		pad(b, width-w)
	}
}

func pad(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

// oneLine keeps a description with line breaks or tabs on one line.
func oneLine(s string) string {
	if !strings.ContainsAny(s, "\n\r\t") {
		return s
	}
	return strings.Join(strings.Fields(s), " ")
}

// compact shortens a count to at most five cells: 999, 1.2k, 12k, 1.2m.
// It rounds down, so a count never reads higher than it is.
func compact(n int) string {
	switch {
	case n < 1_000:
		return strconv.Itoa(max(n, 0))
	case n < 10_000:
		return tenths(n/100) + "k"
	case n < 1_000_000:
		return strconv.Itoa(n/1_000) + "k"
	case n < 10_000_000:
		return tenths(n/100_000) + "m"
	}
	return strconv.Itoa(n/1_000_000) + "m"
}

// tenths formats n tenths as "1.2", or "1" when it is whole.
func tenths(n int) string {
	if n%10 == 0 {
		return strconv.Itoa(n / 10)
	}
	return strconv.Itoa(n/10) + "." + strconv.Itoa(n%10)
}
