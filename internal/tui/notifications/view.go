package notifications

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// headerHeight is the line above the list that shows the filter.
const headerHeight = 1

// Widths of the fixed columns and of the gaps between columns.
const (
	dotWidth    = 1
	tagWidth    = 3
	reasonWidth = 7
	ageWidth    = 4
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

func newStyles(t ui.Theme) styles {
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
		unreadDot:         t.Accent.Render("●"),
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
	tag, repo, title, reason int
}

func newLayout(width int) layout {
	// The dot, the title and the age are always shown.
	rest := width - dotWidth - 1 - gap - ageWidth
	l := layout{
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
	l := newLayout(width)
	tag, repo, title := st.tag, st.repo, st.title
	dot := st.unreadDot
	if !n.Unread {
		dot = st.readDot
		tag, repo, title = st.readTag, st.readRepo, st.readTitle
		if selected {
			title = st.selectedReadTitle
		}
	}

	var b strings.Builder
	b.Grow(width + 96)
	b.WriteString(dot)
	b.WriteByte(' ')
	if l.tag > 0 {
		tag.write(&b, fit(subjectTag(n.Subject.Type), l.tag))
		b.WriteByte(' ')
	}
	if l.repo > 0 {
		repo.write(&b, fit(repoLabel(n.Repo, l.repo), l.repo))
		spaces(&b, gap)
	}
	title.write(&b, fit(n.Subject.Title, l.title))
	spaces(&b, gap)
	if l.reason > 0 {
		st.reason.write(&b, fit(shortReason(n.Reason), l.reason))
		spaces(&b, gap)
	}
	age := ui.Ago(n.UpdatedAt, s.now())
	st.age.write(&b, fitRight(age, ageWidth))
	return b.String()
}

// renderHeader draws the filter line, such as "Unread · f show all".
func (s *Section) renderHeader() {
	name, other := "Unread", "show all"
	if s.all.Load() {
		name, other = "All", "unread only"
	}
	h := "  " + s.styles.filter.render(name)
	if k := s.keys.Filter.Help().Key; k != "" {
		h += s.styles.hint.render(" · " + k + " " + other)
	}
	s.header = fitANSI(h, s.width)
	s.feed.SetEmptyText(s.emptyText())
}

func (s *Section) emptyText() string {
	k := s.keys.Filter.Help().Key
	switch {
	case s.all.Load():
		return "No notifications."
	case k != "":
		return "You're all caught up. Press " + k + " to show read notifications."
	}
	return "You're all caught up."
}

// View renders the filter line and the list.
func (s *Section) View() string {
	if s.height < headerHeight+1 {
		return s.feed.View()
	}
	return s.header + "\n" + s.feed.View()
}

// subjectTag is the short tag of a subject type.
func subjectTag(t core.SubjectType) string {
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
	case "CheckSuite":
		return "ci"
	}
	return "·"
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
	return strings.ReplaceAll(r, "_", " ")
}

// repoLabel names r in at most width cells. The name tells repositories
// apart better than the owner, so the owner is shortened first.
func repoLabel(r core.RepoRef, width int) string {
	full := r.String()
	if ansi.StringWidth(full) <= width {
		return full
	}
	// Keep at least a letter of the owner, an ellipsis and the slash.
	room := width - ansi.StringWidth(r.Name) - 1
	if room < 2 {
		return r.Name
	}
	return ansi.Truncate(r.Owner, room, "…") + "/" + r.Name
}

// fit truncates plain s to width cells and pads it on the right.
func fit(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		s = ansi.Truncate(s, width, "…")
		w = ansi.StringWidth(s)
	}
	return s + strings.Repeat(" ", width-w)
}

// fitRight truncates plain s to width cells and pads it on the left.
func fitRight(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "")
	}
	return strings.Repeat(" ", width-w) + s
}

// fitANSI truncates styled s to width cells and pads it on the right.
func fitANSI(s string, width int) string {
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

func spaces(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}
