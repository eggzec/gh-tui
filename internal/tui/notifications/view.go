package notifications

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// headerHeight is the lines above the list: the filter and the titles of the
// columns.
const headerHeight = 2

// Widths of the fixed columns and of the gaps between columns.
const (
	dotWidth    = 1
	reasonWidth = 7
	gap         = 2
)

// The kind column is one glyph wide, or three cells for the short tags of
// the ASCII set.
const (
	glyphWidth = 1
	tagWidth   = 3
)

// kindWidth returns the width of the column that tells the kind of a thread,
// in the icon set ic.
func kindWidth(ic ui.Icons) int {
	if ic.Set() == config.IconsASCII {
		return tagWidth
	}
	return glyphWidth
}

// Narrow rows give up columns, least important first, so the title keeps at
// least this many cells.
const (
	minTitleWithReason = 32
	minTitleWithTag    = 24
	minTitleWithRepo   = 20
)

// styles are the row styles, built once per theme.
type styles struct {
	// look holds the cells of a row in each of its looks, for a read thread
	// and for an unread one.
	look         [ui.NumLooks][2]cells
	filter, hint paint
}

// cells are the paints of the cells of a row in one look, and its dot.
type cells struct {
	tag, repo, title, reason, age paint
	dot                           string
	// marks holds the glyph of each mark, as wide as the column and painted,
	// or "" for a mark that has none, which is drawn from the type of the
	// thread.
	marks [ui.NumMarks]string
}

func newCells(t ui.Theme, ic ui.Icons, look int, unread bool) cells {
	selected := look != ui.LookPlain
	cell := func(s lipgloss.Style) lipgloss.Style { return t.Cell(s, selected) }
	c := cells{
		tag:    newPaint(cell(t.Muted)),
		repo:   newPaint(cell(t.Muted)),
		reason: newPaint(cell(t.Muted)),
		age:    newPaint(cell(t.Muted)),
		dot:    " ",
	}
	title := t.RowTitle(selected, look == ui.LookFocused)
	switch {
	case unread:
		c.dot = cell(t.Accent).Render(ic.Dot)
		if !selected {
			title = t.Title
		}
	case !selected:
		title = t.Muted
		c.tag, c.repo = newPaint(t.Subtle), newPaint(t.Subtle)
	default:
		c.tag, c.repo = newPaint(cell(t.Subtle)), newPaint(cell(t.Subtle))
	}
	c.title = newPaint(cell(title))
	width := kindWidth(ic)
	for m := range ui.NumMarks {
		g := ic.Mark(m)
		if g == "" {
			continue
		}
		style := t.Subtle
		if unread {
			style = t.Mark(m)
		}
		c.marks[m] = newPaint(cell(style)).render(fit(g, width, ic.Ellipsis))
	}
	return c
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	st := styles{
		filter: newPaint(t.Text),
		hint:   newPaint(t.Subtle),
	}
	for look := range st.look {
		st.look[look] = [2]cells{newCells(t, ic, look, false), newCells(t, ic, look, true)}
	}
	return st
}

// paint is a style rendered once into the sequences around its text, so
// rows are styled by concatenation. It suits styles of one line without
// padding or borders, which the row styles are.
type paint struct {
	pre, post string
}

func newPaint(st lipgloss.Style) paint {
	pre, post, _ := strings.Cut(st.Render("x"), "x")
	return paint{pre: pre, post: post}
}

func (p paint) write(b *strings.Builder, s string) {
	b.WriteString(p.pre)
	b.WriteString(s)
	b.WriteString(p.post)
}

func (p paint) render(s string) string {
	return p.pre + s + p.post
}

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}

// updatedTitle heads the column of ages.
const updatedTitle = "Updated"

// typeTitle heads the column of kinds, when it is wide enough to hold it.
const typeTitle = "Type"

// layout is where the columns of a row go at one width. A zero width drops
// the column.
type layout struct {
	tag, repo, title, reason, age int
}

// newLayout lays out a row of width cells, with dates of age cells at most
// and a kind column of tag cells.
func newLayout(width, age, tag int) layout {
	// The age column is as wide as its title, so the title isn't cut.
	age = max(age, len(updatedTitle))
	// The dot, the title and the age are always shown.
	rest := width - dotWidth - 1 - gap - age
	l := layout{
		age:    age,
		tag:    tag,
		repo:   min(max(width/5, 12), 32),
		reason: reasonWidth,
	}
	l.title = rest - l.tag - 1 - l.repo - gap - l.reason - gap
	if l.title < minTitleWithReason {
		l.title += l.reason + gap
		l.reason = 0
	}
	// A glyph costs less than a tag, so it stays at a narrower title.
	if l.title < minTitleWithTag-(tagWidth-tag) {
		l.title += l.tag + 1
		l.tag = 0
	}
	if l.title < minTitleWithRepo {
		l.title += l.repo + gap
		l.repo = 0
	}
	l.title = max(l.title, 0)
	return l
}

// render draws one notification in width cells:
//
//	●  owner/name      Title of the thread               mention  3h
//
// where the glyph after the dot tells the kind of the thread and its state,
// or is a tag such as "pr" in the ASCII set.
func (s *Section) render(n core.Notification, selected bool, width int) string {
	look := ui.Look(selected, s.feed.Focused())
	unread := 0
	if n.Unread {
		unread = 1
	}
	st := &s.styles.look[look][unread]
	l := newLayout(width, s.dates.Width(), kindWidth(s.icons))
	tag, repo, title, dot := st.tag, st.repo, st.title, st.dot

	ell := s.icons.Ellipsis
	var b strings.Builder
	b.Grow(width + 96)
	b.WriteString(dot)
	b.WriteByte(' ')
	if l.tag > 0 {
		sub, known := s.State(n)
		if g := st.marks[ui.SubjectMark(n.Subject.Type, sub, known)]; g != "" {
			b.WriteString(g)
		} else {
			tag.write(&b, fit(otherTag(n.Subject.Type), l.tag, ell))
		}
		b.WriteByte(' ')
	}
	if l.repo > 0 {
		repo.write(&b, fit(refLabel(n.Repo, n.Subject.Number, l.repo, ell), l.repo, ell))
		spaces(&b, gap)
	}
	// The title links to the thread's page.
	t := cut(ui.OneLine(n.Subject.Title), l.title, ell)
	b.WriteString(s.links.Link(n.Subject.WebURL, title.render(t)))
	spaces(&b, l.title-ansi.StringWidth(t)+gap)
	if l.reason > 0 {
		st.reason.write(&b, fit(shortReason(n.Reason), l.reason, ell))
		spaces(&b, gap)
	}
	age := ""
	if !n.UpdatedAt.IsZero() {
		age = s.dates.Short(n.UpdatedAt, s.now())
	}
	st.age.write(&b, fit(age, l.age, ""))
	return b.String()
}

// renderHeader draws the filter line: which threads are listed, what the
// filter keeps of them, and the keys that change it, such as
// "Unread · mention · f filter".
func (s *Section) renderHeader() {
	f := s.filter()
	name := "Unread"
	if f.all {
		name = "All"
	}
	h := strings.Repeat(" ", s.feed.Gutter()) + s.styles.filter.render(name)
	sep := s.icons.Separator
	kept, off := s.toggle.Kept()
	if off {
		kf := parseFilter(kept)
		h += s.styles.filter.render(sep + ui.OffChip(kf.chips(sep), sep))
	} else if chips := f.chips(sep); chips != "" {
		h += s.styles.filter.render(sep + chips)
	}
	if s.off && len(s.levels) > 0 {
		names := make([]string, len(s.levels))
		for i, c := range s.levels {
			names[i], _ = c.Value.(string)
		}
		h += s.styles.filter.render(sep + ui.OffChip("by "+strings.Join(names, ", "), sep))
	}
	if len(s.sorts) > 0 {
		chip := s.sortChip()
		if s.sortOff {
			chip = ui.OffChip(chip, sep)
		}
		h += s.styles.filter.render(sep + chip)
	}
	var hints []string
	if k := s.keys.Filter.Help().Key; k != "" {
		hints = append(hints, s.icons.Key(k)+" filter")
	}
	if k := s.keys.ToggleGroups.Help().Key; k != "" && len(s.levels) > 0 {
		hints = append(hints, s.icons.Key(k)+" group on/off")
	}
	if k := s.keys.ToggleSort.Help().Key; k != "" && len(s.sorts) > 0 {
		hints = append(hints, s.icons.Key(k)+" sort on/off")
	}
	switched := s.filtered() || off
	if k := s.keys.ToggleFilter.Help().Key; k != "" && switched {
		hints = append(hints, s.icons.Key(k)+" on/off")
	}
	if k := s.keys.ClearFilter.Help().Key; k != "" && switched {
		hints = append(hints, s.icons.Key(k)+" clear")
	}
	if len(hints) > 0 {
		h += s.styles.hint.render("  " + strings.Join(hints, sep))
	}
	s.header = fitANSI(h, s.width, s.icons.Ellipsis)
	l := newLayout(max(s.width-s.feed.Gutter(), 0), s.dates.Width(), kindWidth(s.icons))
	titles := strings.Repeat(" ", s.feed.Gutter()) + newPaint(s.theme.Muted).render(columnHeader(l, s.icons.Ellipsis))
	s.columns = fitANSI(titles, s.width, s.icons.Ellipsis)
	s.feed.SetEmptyText(s.emptyText())
}

func (s *Section) emptyText() string {
	switch f := s.filter(); {
	case f.local():
		return ui.NoMatch("notifications", ui.KeyOf(s.icons, s.keys.ClearFilter))
	case f.all:
		return ui.None("notifications")
	}
	return ui.Press(ui.None("unread notifications"), ui.KeyOf(s.icons, s.keys.Filter), "show read ones too")
}

// View renders the filter line and the list.
func (s *Section) View() string {
	body := s.unreadable
	if body == "" {
		body = s.feed.View()
	}
	if s.height < headerHeight+1 {
		return body
	}
	return s.header + "\n" + s.columns + "\n" + body
}

// refused reports whether the token may not read notifications.
func (s *Section) refused() bool {
	_, _, ok := ui.Unreadable(core.NeedNotifications, "load the notifications", s.voice)
	return ok
}

// renderUnreadable draws what the section shows in place of the list
// while the token may not read notifications, which is nothing while it
// may.
func (s *Section) renderUnreadable() {
	s.unreadable = ""
	text, hint, ok := ui.Unreadable(core.NeedNotifications, "load the notifications", s.voice)
	if !ok || s.width <= 0 {
		return
	}
	h := s.height
	if h >= headerHeight+1 {
		h -= headerHeight
	}
	// The text starts where the titles of the rows do.
	lines := ui.ErrorLine(s.theme.Empty(s.icons), text, hint, max(s.width-2, 1))
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	s.unreadable = strings.Join(ui.FitLines(lines, s.width, h), "\n")
}

// otherTag is the short tag of a type that has no mark with a tag of its
// own: the start of its name in lower case, as the other tags are.
func otherTag(t core.SubjectType) string {
	return ansi.Truncate(strings.ToLower(string(t)), tagWidth, "")
}

// shortReason names why the user got a notification in a word.
func shortReason(r string) string {
	switch r {
	case "review_requested":
		return "review"
	case "approval_requested":
		return "approve"
	case "team_mention":
		return "team"
	case "state_change":
		return "state"
	case "ci_activity":
		return "ci"
	case "security_alert":
		return "alert"
	case "security_advisory_credit":
		return "credit"
	case "subscribed":
		return "watch"
	case "invitation":
		return "invite"
	case "member_feature_requested":
		return "feature"
	case "assign", "author", "comment", "manual", "mention":
		return r
	}
	// GitHub may add reasons, which are shown as they come.
	return ui.OneLine(strings.ReplaceAll(r, "_", " "))
}

// refLabel names the thread of r in at most width cells, as "owner/name#7"
// or, without a number, "owner/name". The name tells repositories apart
// better than the owner, so the owner is shortened first, to end in tail;
// the number is always kept.
func refLabel(r core.RepoRef, number, width int, tail string) string {
	num := ""
	if number > 0 {
		num = "#" + strconv.Itoa(number)
	}
	width = max(width-len(num), 0)
	full := r.String()
	if ansi.StringWidth(full) <= width {
		return full + num
	}
	// Keep at least a letter of the owner, an ellipsis and the slash.
	room := width - ansi.StringWidth(r.Name) - 1
	if room < 2 {
		return termtext.Truncate(r.Name, width, tail) + num
	}
	return termtext.Truncate(r.Owner, room, tail) + "/" + r.Name + num
}

// columnHeader renders the titles of the columns of the rows of l, in the
// cells the rows give them.
func columnHeader(l layout, tail string) string {
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", dotWidth+1))
	if l.tag > 0 {
		title := typeTitle
		if l.tag < len(typeTitle) {
			title = ""
		}
		b.WriteString(ui.Header([]ui.Column{{Title: title, Width: l.tag}}, 1, tail))
		b.WriteByte(' ')
	}
	b.WriteString(ui.Header([]ui.Column{
		{Title: "Ref", Width: l.repo},
		{Title: "Title", Width: l.title},
		{Title: "Reason", Width: l.reason},
		{Title: updatedTitle, Width: l.age},
	}, gap, tail))
	return b.String()
}

// fit truncates plain s to width cells, ending in tail, and pads it on the
// right.
func fit(s string, width int, tail string) string {
	s = cut(s, width, tail)
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

// cut truncates plain s to width cells, ending in tail.
func cut(s string, width int, tail string) string {
	if ansi.StringWidth(s) > width {
		return termtext.Truncate(s, width, tail)
	}
	return s
}

// fitANSI truncates styled s to width cells, ending in tail, and pads it
// on the right.
func fitANSI(s string, width int, tail string) string {
	s = termtext.Truncate(s, width, tail)
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

func spaces(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}
