package dashboard

import (
	"cmp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Sizes of the dashboard. From wideWidth by wideHeight cells it shows
// every pane at once: the profile on top, the pinned cards below it, the
// repositories beside the work and the notifications, and the calendar at
// the bottom. Below that it shows the profile and the focused pane, whose
// frame names the others.
const (
	wideWidth     = 100
	wideHeight    = 30
	profileHeight = 2
	pinnedHeight  = cardHeight + 2
	// calendarLines is what the calendar draws: the total, the months,
	// seven days and the legend.
	calendarLines  = 10
	calendarHeight = calendarLines + 2
	// calendarWidth fits a year of weeks and the weekday labels.
	calendarWidth = 4 + 53*2 - 1
	minInboxH     = 4
	maxInboxH     = 9
)

// box is the outer size of a pane, frame included.
type box struct{ w, h int }

// layout sizes the panes for the dashboard's size.
func (s *Section) layout() {
	s.wide = s.width >= wideWidth && s.height >= wideHeight
	rest := max(s.height-profileHeight, 0)
	var b [numPanes]box
	if s.wide {
		mid := max(rest-pinnedHeight-calendarHeight, 0)
		lw := s.width * 11 / 20
		inbox := min(max(mid*2/5, minInboxH), maxInboxH)
		b[pinnedPane] = box{s.width, pinnedHeight}
		b[reposPane] = box{lw, mid}
		b[workPane] = box{s.width - lw, max(mid-inbox, 0)}
		b[inboxPane] = box{s.width - lw, min(inbox, mid)}
		b[calendarPane] = box{s.width, calendarHeight}
	} else {
		for i := range b {
			b[i] = box{s.width, rest}
		}
	}
	s.boxes = b
	in := func(p paneID) (int, int) { return max(b[p].w-2, 0), max(b[p].h-2, 0) }
	s.pinned.resize(in(pinnedPane))
	s.repos.resize(in(reposPane))
	s.tasks.resize(in(workPane))
	cw, ch := in(calendarPane)
	s.cal.SetSize(min(cw-2, calendarWidth), min(ch, calendarLines))
}

// render renders the profile and every pane, and the dashboard from them.
func (s *Section) render() {
	s.head = s.profile()
	for p := range numPanes {
		s.renderPane(p)
	}
	s.compose()
}

// renderPane renders pane p in its frame. Compose the dashboard after.
func (s *Section) renderPane(p paneID) {
	if s.width <= 0 || s.height <= 0 || !s.wide && p != s.focus {
		return
	}
	b := s.boxes[p]
	w, h := max(b.w-2, 0), max(b.h-2, 0)
	var body []string
	switch p {
	case pinnedPane:
		body = s.pinnedBody(w, h)
	case reposPane:
		body = s.reposBody(w, h)
	case workPane:
		body = s.workBody(w, h)
	case inboxPane:
		body = s.inboxBody(w, h)
	default:
		body = s.calendarBody(w)
	}
	label, lw := s.label(p)
	s.frames[p] = s.frame(label, lw, b, s.focused && p == s.focus, body)
}

// compose joins the profile and the framed panes into the view.
func (s *Section) compose() {
	if s.width <= 0 || s.height <= 0 {
		s.view = ""
		return
	}
	lines := make([]string, 0, s.height+profileHeight)
	lines = append(lines, s.head...)
	if !s.wide {
		lines = append(lines, s.frames[s.focus]...)
	} else {
		lines = append(lines, s.frames[pinnedPane]...)
		right := make([]string, 0, s.boxes[reposPane].h)
		right = append(append(right, s.frames[workPane]...), s.frames[inboxPane]...)
		for i, l := range s.frames[reposPane] {
			if i < len(right) {
				l += right[i]
			}
			lines = append(lines, l)
		}
		lines = append(lines, s.frames[calendarPane]...)
	}
	blank := strings.Repeat(" ", s.width)
	for len(lines) < s.height {
		lines = append(lines, blank)
	}
	s.view = strings.Join(lines[:s.height], "\n")
}

// label is the text in the top edge of pane p, and its width. In the
// narrow layout the one frame names every pane, the focused one in full.
func (s *Section) label(p paneID) (label string, width int) {
	st := &s.st
	title := st.title
	if s.focused && p == s.focus {
		title = st.focusTitle
	}
	if s.wide {
		text := s.paneLabel(p)
		return title.render(text), ansi.StringWidth(text)
	}
	// The other panes go by their titles, then by shorter ones, then by
	// their keys alone, whichever fits.
	for tier, titles := range [][numPanes]string{paneTitles, shortTitles, {}} {
		var b strings.Builder
		n := 0
		for i := range numPanes {
			part := s.paneLabel(i)
			if i != p {
				part = strings.TrimSpace(s.keyLabel(i) + " " + titles[i])
			}
			if part == "" {
				continue
			}
			if n > 0 {
				b.WriteString("  ")
				n += 2
			}
			if i == p {
				title.write(&b, part)
			} else {
				st.subtle.write(&b, part)
			}
			n += ansi.StringWidth(part)
		}
		if n <= s.width-4 || tier == 2 {
			return b.String(), n
		}
	}
	return "", 0
}

// keyLabel is the key that focuses pane p, or "" when it has none.
func (s *Section) keyLabel(p paneID) string {
	b := s.keys.Panes[p]
	if !b.Enabled() {
		return ""
	}
	return b.Help().Key
}

// paneLabel is the label of pane p, such as "[3] Waiting on you · 4".
func (s *Section) paneLabel(p paneID) string {
	text := paneTitles[p]
	if k := s.keyLabel(p); k != "" {
		text = "[" + k + "] " + text
	}
	switch p {
	case pinnedPane:
		if at, of := s.pinned.pages(); of > 1 {
			text += " · " + strconv.Itoa(at) + "/" + strconv.Itoa(of)
		}
	case workPane:
		if s.work.ok {
			text += " · " + strconv.Itoa(s.tasks.count(s.work.value))
		}
	case inboxPane:
		if n := s.unread(); n != "" {
			text += " · " + n + " unread"
		}
	default:
	}
	return text
}

// frame draws body in a frame of size b, with label in its top edge.
func (s *Section) frame(label string, labelW int, b box, focused bool, body []string) []string {
	if b.w < 2 || b.h < 2 {
		return nil
	}
	edge := s.st.edge
	if focused {
		edge = s.st.focusEdge
	}
	bd := lipgloss.RoundedBorder()
	lines := make([]string, 0, b.h)
	if labelW > b.w-4 {
		label = ansi.Truncate(label, max(b.w-5, 0), "…")
		labelW = ansi.StringWidth(label)
	}
	if labelW == 0 {
		lines = append(lines, edge.render(bd.TopLeft+strings.Repeat(bd.Top, b.w-2)+bd.TopRight))
	} else {
		rest := max(b.w-4-labelW, 0)
		lines = append(lines, edge.render(bd.TopLeft+bd.Top)+label+edge.render(" "+strings.Repeat(bd.Top, rest)+bd.TopRight))
	}
	side, inner := edge.render(bd.Left), b.w-2
	for i := range b.h - 2 {
		var l string
		if i < len(body) {
			l = body[i]
		}
		lines = append(lines, side+fit(l, inner)+side)
	}
	return append(lines, edge.render(bd.BottomLeft+strings.Repeat(bd.Bottom, b.w-2)+bd.BottomRight))
}

// profile renders the two lines above the panes: who the viewer is, and
// how the dashboard is doing.
func (s *Section) profile() []string {
	st, w := &s.st, s.width
	if w <= 2 {
		return []string{fit("", w), fit("", w)}
	}
	var first, second, right string
	switch h := s.header; {
	case h.ok:
		p := h.value.Profile
		first = st.name.render(cleanLine(cmp.Or(p.Name, p.Login))) + " " + st.login.render("@"+p.Login)
		if p.Bio != "" {
			first += st.subtle.render(" · ") + st.text.render(cleanLine(p.Bio))
		}
		second = s.facts(p)
	case h.err != nil:
		first = st.fail.render("Couldn't load your profile: "+cleanLine(h.err.Error())) + st.subtle.render(" · "+s.keys.Refresh.Help().Key+" retries")
	default:
		first = st.muted.render("Loading your profile…")
	}
	// The app's header counts the unread notifications, and so does the
	// notifications pane.
	switch {
	case s.offlineNow():
		right = st.warning.render("offline · showing the last visit")
	case s.updating():
		right = st.subtle.render("updating…")
	}
	return []string{spread(" "+first, "", w), spread(" "+second, right+" ", w)}
}

// facts lists the company, location, follows and status of p.
func (s *Section) facts(p core.Profile) string {
	st := &s.st
	parts := make([]string, 0, 6)
	for _, t := range []string{p.Company, p.Location} {
		if t = cleanLine(t); t != "" {
			parts = append(parts, st.muted.render(t))
		}
	}
	parts = append(parts,
		st.text.render(count(p.Followers))+st.muted.render(plural(p.Followers, " follower", " followers")),
		st.text.render(count(p.Following))+st.muted.render(" following"),
	)
	if m := cleanLine(p.Status.Message); m != "" || p.Status.Busy {
		status := m
		if p.Status.Busy {
			status = strings.TrimSpace("busy " + status)
		}
		parts = append(parts, st.accent.render(status))
	}
	return strings.Join(parts, st.subtle.render(" · "))
}

// unread is the number of unread threads, with a "+" when more pages
// follow, or "" when there are none.
func (s *Section) unread() string {
	if !s.notes.ok {
		return ""
	}
	n := 0
	for i := range s.notes.value.Items {
		if s.notes.value.Items[i].Unread {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	t := strconv.Itoa(n)
	if !s.notes.value.Last() {
		t += "+"
	}
	return t
}

func (s *Section) pinnedBody(w, h int) []string {
	st, c := &s.st, &s.pinned
	if c.cols == 0 {
		return nil
	}
	if len(c.items) == 0 {
		switch {
		case s.header.err != nil && !s.header.ok:
			return []string{" " + st.fail.render("Couldn't load your pins.")}
		case !s.header.ok:
			return []string{" " + st.muted.render("Loading pinned repositories…")}
		}
		return []string{" " + st.muted.render("Nothing pinned. Pin repositories on your GitHub profile to see them here.")}
	}
	cw := max((w-cardGap*(c.cols-1))/c.cols, 1)
	lines := make([]string, 0, h)
	for row := c.top; row < c.top+c.rows && row*c.cols < len(c.items); row++ {
		if row > c.top {
			lines = append(lines, "")
		}
		var rowLines [cardHeight]strings.Builder
		for col := range c.cols {
			i := row*c.cols + col
			if i >= len(c.items) {
				break
			}
			card := s.card(c.items[i], i == c.sel, cw)
			for l := range cardHeight {
				if col > 0 {
					rowLines[l].WriteString(strings.Repeat(" ", cardGap))
				}
				rowLines[l].WriteString(card[l])
			}
		}
		for l := range rowLines {
			lines = append(lines, rowLines[l].String())
		}
	}
	return lines
}

// card renders a pinned repository in w cells: its name, two lines of its
// description, and its language and stars.
func (s *Section) card(c card, selected bool, w int) [cardHeight]string {
	st := &s.st
	gutter := "  "
	if selected {
		gutter = st.blurred
		if s.focused && s.focus == pinnedPane {
			gutter = st.cursor
		}
	}
	inner := max(w-2, 0)
	r := c.repo
	var out [cardHeight]string
	out[0] = gutter + st.name.render(truncate(r.Ref.String(), inner))
	desc := wrap(cleanLine(r.Description), inner, 2)
	if len(desc) == 0 && c.here && r.Description == "" {
		desc = []string{"The repository of this directory."}
	}
	for i, d := range desc {
		out[1+i] = gutter + st.muted.render(d)
	}
	for i := len(desc); i < 2; i++ {
		out[1+i] = gutter
	}
	facts := s.repoFacts(r, inner)
	if c.here {
		facts = st.accent.render("⌂ here") + "  " + facts
	}
	out[3] = gutter + ansi.Truncate(facts, inner, "…")
	for i := range out {
		out[i] = fit(out[i], w)
	}
	return out
}

// repoFacts renders the language, stars and marks of r in at most w cells.
func (s *Section) repoFacts(r core.Repo, w int) string {
	st := &s.st
	parts := make([]string, 0, 4)
	if r.Language != "" {
		parts = append(parts, st.accent.render("●")+" "+st.text.render(r.Language))
	}
	parts = append(parts, st.muted.render("★ "+count(r.Stars)))
	for _, m := range repoMarks(r) {
		parts = append(parts, st.subtle.render(m))
	}
	return ansi.Truncate(strings.Join(parts, "  "), w, "…")
}

func repoMarks(r core.Repo) []string {
	var marks []string
	if r.Private {
		marks = append(marks, "private")
	}
	if r.Fork {
		marks = append(marks, "fork")
	}
	if r.Archived {
		marks = append(marks, "archived")
	}
	return marks
}

// reposBody renders the tabs of the owners above the list or the filter of
// the tab on view.
func (s *Section) reposBody(w, h int) []string {
	t := &s.repos
	lines := make([]string, 0, h)
	lines = append(lines, s.tabsLine(w))
	var body string
	if t.filtering {
		body = t.picker.View()
	} else {
		body = t.current().feed.View()
	}
	if body != "" {
		lines = append(lines, strings.Split(body, "\n")...)
	}
	return lines
}

// tabsLine renders the tabs of the owners, scrolled to show the one on
// view, and the progress of the filter on the right.
func (s *Section) tabsLine(w int) string {
	st, t := &s.st, &s.repos
	var right string
	if t.filtering {
		o := t.current()
		n := strconv.Itoa(len(o.all))
		switch {
		case o.fillErr != nil:
			right = st.fail.render(n + " read · some failed")
		case o.filling:
			right = st.subtle.render("reading " + n + "…")
		default:
			right = st.subtle.render(n + " repositories")
		}
	}
	room := max(w-ansi.StringWidth(right)-2, 0)
	// Scroll the tabs so the one on view fits, with the ones before it
	// when there is room.
	start := 0
	for start < t.cur && tabsWidth(t.tabs[start:t.cur+1]) > room-2 {
		start++
	}
	var b strings.Builder
	b.WriteByte(' ')
	used := 1
	if start > 0 {
		st.subtle.write(&b, "‹ ")
		used += 2
	}
	for i := start; i < len(t.tabs); i++ {
		label := t.tabs[i].label
		lw := ansi.StringWidth(label) + 2
		if used+lw > room {
			st.subtle.write(&b, "›")
			break
		}
		if i == t.cur {
			st.focusTitle.write(&b, label)
		} else {
			st.muted.write(&b, label)
		}
		b.WriteString("  ")
		used += lw
	}
	return spread(b.String(), right, w)
}

func tabsWidth(tabs []*owner) int {
	n := 0
	for _, o := range tabs {
		n += ansi.StringWidth(o.label) + 2
	}
	return n
}

// Widths of the columns at the right of a repository row.
const (
	langWidth  = 12
	starsWidth = 7
	ageWidth   = 4
)

// renderRepo renders a repository of the list in width cells: its name
// and marks, its description, and its language, stars and age.
func (t *repoTabs) renderRepo(r core.Repo, selected bool, width int) string {
	s := t.s
	st := &s.st
	lang, stars := width >= 60, width >= 44
	right := ageWidth
	if lang {
		right += langWidth + 1
	}
	if stars {
		right += starsWidth + 1
	}
	left := max(width-right-1, 0)
	var b strings.Builder
	b.Grow(width + 64)
	name := truncate(r.Ref.Name, left)
	nameStyle := st.text
	if selected {
		nameStyle = st.selected
	}
	nameStyle.write(&b, name)
	used := ansi.StringWidth(name)
	for _, m := range repoMarks(r) {
		if used+len(m)+2 > left {
			break
		}
		b.WriteString(" ")
		st.subtle.write(&b, m)
		used += len(m) + 1
	}
	if d := cleanLine(r.Description); d != "" && used+4 < left {
		d = truncate(d, left-used-2)
		b.WriteString("  ")
		st.muted.write(&b, d)
		used += 2 + ansi.StringWidth(d)
	}
	b.WriteString(strings.Repeat(" ", max(left-used, 0)+1))
	if lang {
		l := truncate(r.Language, langWidth)
		st.muted.write(&b, l)
		b.WriteString(strings.Repeat(" ", langWidth-ansi.StringWidth(l)+1))
	}
	if stars {
		n := "★ " + count(r.Stars)
		b.WriteString(strings.Repeat(" ", max(starsWidth-ansi.StringWidth(n), 0)))
		st.muted.write(&b, n)
		b.WriteByte(' ')
	}
	age := ""
	if !r.UpdatedAt.IsZero() {
		age = ui.Ago(r.UpdatedAt, s.now())
	}
	b.WriteString(strings.Repeat(" ", max(ageWidth-len(age), 0)))
	st.subtle.write(&b, age)
	return b.String()
}

func (s *Section) workBody(w, h int) []string {
	st, l := &s.st, &s.tasks
	switch {
	case !s.work.ok && s.work.err != nil:
		return []string{" " + st.fail.render("Couldn't load your work: "+cleanLine(s.work.err.Error())), " " + st.subtle.render(s.keys.Refresh.Help().Key+" retries")}
	case !s.work.ok:
		return []string{" " + st.muted.render("Loading the work waiting on you…")}
	}
	lines := make([]string, 0, h)
	focused := s.focused && s.focus == workPane
	for i := l.top; i < len(l.rows); i++ {
		// Rows show whole, but for one taller than the pane.
		if n := l.lines(i, l.top); len(lines)+n > h && i > l.top {
			break
		}
		r := &l.rows[i]
		switch {
		case r.header != "":
			if i > l.top {
				lines = append(lines, "")
			}
			lines = append(lines, " "+st.muted.render(r.header)+" "+st.text.render(strconv.Itoa(r.count)))
		case r.note != "":
			lines = append(lines, "   "+st.subtle.render(r.note))
		default:
			sel := l.sel < len(l.items) && l.items[l.sel] == i
			lines = s.workItem(lines, r, sel, focused, w)
		}
	}
	return lines
}

// workItem appends the lines of a pull request or issue waiting on the
// viewer: its state, where it is and its title, wrapped under the text
// with the age at the end. The cursor marks every line of the selected
// one.
func (s *Section) workItem(lines []string, r *workRow, selected, focused bool, w int) []string {
	st := &s.st
	gutter := "  "
	titleStyle := st.text
	if selected {
		gutter = st.blurred
		if focused {
			gutter = st.cursor
		}
		titleStyle = st.selected
	}
	hit := r.hit
	age := ui.Ago(hit.Issue.UpdatedAt, s.now())
	for i, text := range r.lines {
		var b strings.Builder
		b.WriteString(gutter)
		used := workIndent
		if i == 0 {
			state := ui.HitState(*hit)
			st.states[state].write(&b, s.icons.State(state))
			b.WriteByte(' ')
			st.muted.write(&b, r.ref)
			used += ansi.StringWidth(r.ref)
			if text != "" {
				b.WriteByte(' ')
				used++
			}
		} else {
			b.WriteString("  ")
		}
		titleStyle.write(&b, text)
		used += ansi.StringWidth(text)
		if i == len(r.lines)-1 {
			b.WriteString(strings.Repeat(" ", max(w-used-len(age), 1)))
			st.subtle.write(&b, age)
		}
		lines = append(lines, b.String())
	}
	return lines
}

func (s *Section) inboxBody(w, h int) []string {
	st := &s.st
	switch {
	case s.inbox == nil:
		return []string{" " + st.muted.render("Notifications aren't available.")}
	case !s.notes.ok && s.notes.err != nil:
		return []string{" " + st.fail.render("Couldn't load your notifications.")}
	case !s.notes.ok:
		return []string{" " + st.muted.render("Loading notifications…")}
	}
	n := s.unread()
	if n == "" {
		return []string{" " + st.muted.render("All caught up. Nothing unread.")}
	}
	lines := make([]string, 0, h)
	lines = append(lines, " "+st.text.render(n)+st.muted.render(" unread")+st.subtle.render(" · "+s.keys.Select.Help().Key+" shows them all"))
	for i := range s.notes.value.Items {
		if len(lines) >= h {
			break
		}
		nt := &s.notes.value.Items[i]
		if !nt.Unread {
			continue
		}
		age := ui.Ago(nt.UpdatedAt, s.now())
		room := max(w-4-ageWidth-1, 0)
		repo := truncate(nt.Repo.Name, min(ansi.StringWidth(nt.Repo.Name), room/3))
		title := truncate(cleanLine(nt.Subject.Title), max(room-ansi.StringWidth(repo)-2, 0))
		used := 4 + ansi.StringWidth(repo) + 2 + ansi.StringWidth(title)
		lines = append(lines, "  "+st.accent.render("●")+" "+st.muted.render(repo)+"  "+st.text.render(title)+
			strings.Repeat(" ", max(w-used-len(age), 1))+st.subtle.render(age))
	}
	return lines
}

// calendarBody centers the calendar in w cells.
func (s *Section) calendarBody(w int) []string {
	if !s.contribs.ok && s.contribs.err != nil {
		return []string{" " + s.st.fail.render("Couldn't load your contributions.")}
	}
	view := s.cal.View()
	if view == "" {
		return nil
	}
	pad := strings.Repeat(" ", max((w-s.cal.Width())/2, 0))
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return lines
}
