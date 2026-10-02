package notifications

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// headerHeight is the line above the list that shows the filter.
const headerHeight = 1

// Widths of the fixed columns and of the gaps between columns.
const (
	dotWidth    = 1
	tagWidth    = 3
	reasonWidth = 7
	gap         = 2
)

// Narrow rows give up columns, least important first, so the title keeps at
// least this many cells.
const (
	minTitleWithReason = 32
	minTitleWithTag    = 24
	minTitleWithRepo   = 20
)

// styles are the row styles, built once per theme.
type styles struct {
	tag, repo, title, reason, age paint
	readRepo, readTitle, readTag  paint
	selectedReadTitle             paint
	filter, hint                  paint
	unreadDot, readDot            string
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	return styles{
		tag:               newPaint(t.Muted),
		repo:              newPaint(t.Muted),
		title:             newPaint(t.Title),
		reason:            newPaint(t.Muted),
		age:               newPaint(t.Muted),
		readTag:           newPaint(t.Subtle),
		readRepo:          newPaint(t.Subtle),
		readTitle:         newPaint(t.Muted),
		selectedReadTitle: newPaint(t.Text),
		filter:            newPaint(t.Text),
		hint:              newPaint(t.Subtle),
		unreadDot:         t.Accent.Render(ic.Dot),
		readDot:           " ",
	}
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

// layout is where the columns of a row go at one width. A zero width drops
// the column.
type layout struct {
	tag, repo, title, reason, age int
}

// newLayout lays out a row of width cells, with dates of age cells at
// most.
func newLayout(width, age int) layout {
	// The dot, the title and the age are always shown.
	rest := width - dotWidth - 1 - gap - age
	l := layout{
		age:    age,
		tag:    tagWidth,
		repo:   min(max(width/5, 12), 28),
		reason: reasonWidth,
	}
	l.title = rest - l.tag - 1 - l.repo - gap - l.reason - gap
	if l.title < minTitleWithReason {
		l.title += l.reason + gap
		l.reason = 0
	}
	if l.title < minTitleWithTag {
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
//	● pr  owner/name      Title of the thread               mention  3h
func (s *Section) render(n core.Notification, selected bool, width int) string {
	st := &s.styles
	l := newLayout(width, s.dates.Width())
	tag, repo, title := st.tag, st.repo, st.title
	dot := st.unreadDot
	if !n.Unread {
		dot = st.readDot
		tag, repo, title = st.readTag, st.readRepo, st.readTitle
		if selected {
			title = st.selectedReadTitle
		}
	}

	ell := s.icons.Ellipsis
	var b strings.Builder
	b.Grow(width + 96)
	b.WriteString(dot)
	b.WriteByte(' ')
	if l.tag > 0 {
		tag.write(&b, fit(subjectTag(n.Subject.Type, strings.TrimSpace(s.icons.Separator)), l.tag, ell))
		b.WriteByte(' ')
	}
	if l.repo > 0 {
		repo.write(&b, fit(repoLabel(n.Repo, l.repo, ell), l.repo, ell))
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
	st.age.write(&b, fitRight(age, l.age))
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
	h := "  " + s.styles.filter.render(name)
	sep := s.icons.Separator
	if chips := f.chips(sep); chips != "" {
		h += s.styles.filter.render(sep + chips)
	}
	var hints []string
	if k := s.keys.Filter.Help().Key; k != "" {
		hints = append(hints, s.icons.Key(k)+" filter")
	}
	if k := s.keys.ClearFilter.Help().Key; k != "" && s.filtered() {
		hints = append(hints, s.icons.Key(k)+" clear")
	}
	if len(hints) > 0 {
		h += s.styles.hint.render("  " + strings.Join(hints, sep))
	}
	s.header = fitANSI(h, s.width, s.icons.Ellipsis)
	s.feed.SetEmptyText(s.emptyText())
}

func (s *Section) emptyText() string {
	switch f := s.filter(); {
	case f.local():
		return ui.NoMatch("notifications", ui.KeyOf(s.keys.ClearFilter))
	case f.all:
		return ui.None("notifications")
	}
	return ui.Press(ui.None("unread notifications"), ui.KeyOf(s.keys.Filter), "show read ones too")
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
	return s.header + "\n" + body
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

// subjectTag is the short tag of a subject type, or other for a type
// without one.
func subjectTag(t core.SubjectType, other string) string {
	switch t {
	case core.SubjectPullRequest:
		return "pr"
	case core.SubjectIssue:
		return "iss"
	case core.SubjectRelease:
		return "rel"
	case core.SubjectDiscussion:
		return "dsc"
	case core.SubjectCommit:
		return "cmt"
	case core.SubjectCheckSuite:
		return "ci"
	}
	return other
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

// repoLabel names r in at most width cells. The name tells repositories
// apart better than the owner, so the owner is shortened first, to end in
// tail.
func repoLabel(r core.RepoRef, width int, tail string) string {
	full := r.String()
	if ansi.StringWidth(full) <= width {
		return full
	}
	// Keep at least a letter of the owner, an ellipsis and the slash.
	room := width - ansi.StringWidth(r.Name) - 1
	if room < 2 {
		return r.Name
	}
	return termtext.Truncate(r.Owner, room, tail) + "/" + r.Name
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

// fitRight truncates plain s to width cells and pads it on the left.
func fitRight(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "")
	}
	return strings.Repeat(" ", width-w) + s
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
