package dashboard

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Sizes of the dashboard. From wideWidth by wideHeight cells it shows
// every pane at once: the profile on top, the pinned cards below it, the
// repositories beside the work, and the calendar beside the notifications
// at the bottom. Below that, or zoomed, it shows the profile and the
// focused pane, whose frame names the others.
const (
	wideWidth    = 100
	wideHeight   = 30
	pinnedHeight = ownerui.CardHeight + 2
	// calendarLines is what the calendar draws: the total, the months,
	// seven days and the legend.
	calendarLines  = 10
	calendarHeight = calendarLines + 2
	// calendarPad is the frame of the calendar and a space on each side.
	calendarPad = 4
	// minCalendarW is the narrowest the calendar gets beside the
	// notifications, about twenty weeks, and minInboxW the narrowest the
	// notifications get before the calendar stops giving way.
	minCalendarW = calendarPad + 4 + 20*2 - 1
	minInboxW    = 40
)

// box is the outer size of a pane, frame included.
type box struct{ w, h int }

// layout sizes the panes for the dashboard's size.
func (s *Section) layout() {
	s.wide = s.width >= wideWidth && s.height >= wideHeight
	rest := max(s.height-s.profileHeight(), 0)
	var b [numPanes]box
	if !s.onePane() {
		mid := max(rest-pinnedHeight-calendarHeight, 0)
		lw := s.width * 11 / 20
		b[pinnedPane] = box{s.width, pinnedHeight}
		b[reposPane] = box{lw, mid}
		b[workPane] = box{s.width - lw, mid}
		cw := s.calendarWidth()
		b[calendarPane] = box{cw, calendarHeight}
		b[inboxPane] = box{s.width - cw, calendarHeight}
	} else {
		for i := range b {
			b[i] = box{s.width, rest}
		}
	}
	s.boxes = b
	in := func(p paneID) (int, int) { return max(b[p].w-2, 0), max(b[p].h-2, 0) }
	s.pinned.Resize(in(pinnedPane))
	s.repos.resize(in(reposPane))
	s.tasks.resize(in(workPane))
	cw, ch := in(calendarPane)
	s.cal.SetSize(min(cw-2, s.cal.FitWidth()), min(ch, calendarLines))
}

// onePane reports whether the dashboard shows the focused pane alone:
// when the panes don't fit, or while it is zoomed.
func (s *Section) onePane() bool { return !s.wide || s.zoom }

// zoomed reports whether the zoom shows. A dashboard too small for every
// pane shows one anyway, so there the back key keeps its other uses.
func (s *Section) zoomed() bool { return s.zoom && s.wide }

// setZoom shows the focused pane alone, or every pane again.
func (s *Section) setZoom(zoom bool) {
	s.zoom = zoom
	s.layout()
}

// calendarWidth is the outer width of the calendar in the bottom row: what
// its range needs, less recent weeks when the notifications would get
// narrower than minInboxW, down to minCalendarW.
func (s *Section) calendarWidth() int {
	return min(s.cal.FitWidth()+calendarPad, max(s.width-minInboxW, minCalendarW))
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
	if s.width <= 0 || s.height <= 0 || s.onePane() && p != s.focus {
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
	lines := make([]string, 0, s.height+s.profileHeight())
	lines = append(lines, s.head...)
	if s.onePane() {
		lines = append(lines, s.frames[s.focus]...)
	} else {
		lines = append(lines, s.frames[pinnedPane]...)
		lines = ownerui.Beside(lines, s.frames[reposPane], s.frames[workPane])
		lines = ownerui.Beside(lines, s.frames[calendarPane], s.frames[inboxPane])
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
	if !s.onePane() {
		text := s.paneLabel(p)
		return title.Render(text), ansi.StringWidth(text)
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
				title.Write(&b, part)
			} else {
				st.shared.Subtle.Write(&b, part)
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
		if at, of := s.pinned.Pages(); of > 1 {
			text += s.icons.Separator + strconv.Itoa(at) + "/" + strconv.Itoa(of)
		}
	case reposPane:
		if chips := s.repos.filter().Chips(s.icons); chips != "" {
			text += s.icons.Separator + chips
		}
	case workPane:
		if s.work.ok {
			text += s.icons.Separator + strconv.Itoa(s.tasks.count(s.work.value))
		}
	case inboxPane:
		if n := s.unread(); n != "" {
			text += s.icons.Separator + n + " unread"
		}
	default:
	}
	return text
}

// frame draws body in a frame of size b, with label in its top edge.
func (s *Section) frame(label string, labelW int, b box, focused bool, body []string) []string {
	edge := s.st.edge
	if focused {
		edge = s.st.focusEdge
	}
	return ownerui.Frame(label, labelW, b.w, b.h, edge, s.icons.Border, s.icons.Ellipsis, body)
}

// profileHeight is how many lines the profile takes.
func (s *Section) profileHeight() int {
	return ownerui.ProfileHeight(s.avatarShown())
}

// avatarShown reports whether the profile shows the viewer's avatar:
// where avatars are drawn, on a dashboard wide enough.
func (s *Section) avatarShown() bool {
	return s.avatars.Shown() && s.width >= ownerui.MinAvatarWidth
}

// drawer draws the profile and the pinned cards in the dashboard's styles.
func (s *Section) drawer() ownerui.Drawer {
	return ownerui.Drawer{Styles: &s.st.shared, Icons: s.icons, Links: &s.links, URL: s.repoURL, Lang: s.langPaint}
}

// profile renders the lines above the panes: who the viewer is, and how
// the dashboard is doing, beside the viewer's avatar where it is drawn.
func (s *Section) profile() []string {
	st, w := &s.st, s.width
	n := s.profileHeight()
	if w <= 2 {
		return ownerui.BlankProfile(w, n)
	}
	avatar := ""
	if s.header.ok {
		avatar = ui.SizedAvatar(s.header.value.Profile.AvatarURL, ui.AvatarLarge)
	}
	var box []string
	if s.avatarShown() {
		box = s.avatars.Box(avatar, ui.AvatarLarge)
		w -= 1 + ui.AvatarLarge.Cols
	}
	var first, second, right string
	switch h := s.header; {
	case h.ok:
		d := s.drawer()
		first, second = d.NameLine(h.value.Profile), d.Facts(h.value.Profile)
	case h.err != nil:
		// The error takes both lines of the profile, and no more: when
		// the hint would need a third, the text gives up all but its
		// first line to it.
		text, hint := s.say("load your profile", h.err)
		lines := ui.ErrorLine(s.errs, text, hint, w-1)
		if len(lines) > 2 {
			mark := ansi.StringWidth(s.errs.Mark + " ")
			lines = ui.ErrorLine(s.errs, termtext.Truncate(text, w-1-mark, s.icons.Ellipsis), hint, w-1)
		}
		lines = append(lines, "", "")
		first, second = lines[0], lines[1]
	default:
		first = st.shared.Muted.Render("Loading your profile" + s.icons.Ellipsis)
	}
	// The app's header counts the unread notifications, and so does the
	// notifications pane.
	switch {
	case s.offlineNow():
		right = st.warning.Render(ui.SayKept(core.Offline, s.icons))
	case s.limitedNow():
		right = st.warning.Render(ui.SayKept(core.RateLimited, s.icons))
	case s.updating():
		right = st.shared.Subtle.Render("updating" + s.icons.Ellipsis)
	}
	return s.drawer().Profile(box, first, second, right, w, n)
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
	if c.Cols == 0 {
		return nil
	}
	if len(c.Items) == 0 {
		switch {
		case s.header.err != nil && !s.header.ok:
			return ownerui.Indent(s.failure("load your pins", s.header.err, w-1))
		case !s.header.ok:
			return []string{" " + st.shared.Muted.Render("Loading pinned repositories"+s.icons.Ellipsis)}
		}
		return []string{" " + st.shared.Muted.Render(ui.None("pinned repositories")+" Pin them on your GitHub profile to see them here.")}
	}
	return c.Lines(w, h, s.card)
}

// card renders a pinned repository in w cells, marked when selected.
func (s *Section) card(c ownerui.Card, selected bool, w int) [ownerui.CardHeight]string {
	gutter := "  "
	if selected {
		gutter = s.st.shared.Blurred
		if s.focused && s.focus == pinnedPane {
			gutter = s.st.shared.Cursor
		}
	}
	return s.drawer().Card(c, gutter, w)
}

// reposBody renders the tabs of the owners above the list of the tab on
// view.
func (s *Section) reposBody(w, h int) []string {
	lines := make([]string, 0, h)
	lines = append(lines, s.tabsLine(w))
	o := s.repos.current()
	// The headers name the columns once there are rows under them.
	head := ""
	if o.Feed.Len() > 0 {
		head = strings.Repeat(" ", gutterWidth) + s.st.shared.Subtle.Render(o.Cols().Header(s.icons.Star, s.icons.Ellipsis))
	}
	lines = append(lines, head)
	if body := o.Feed.View(); body != "" {
		lines = append(lines, strings.Split(body, "\n")...)
	}
	return lines
}

// tabsLine renders the tabs of the owners, scrolled to show the one on
// view, and on the right the language of the repository under the cursor,
// which the list shows as a glyph.
func (s *Section) tabsLine(w int) string {
	st, t := &s.st, &s.repos
	var right string
	if r, ok := t.selected(); ok && r.Language != "" {
		right = s.langPaint(r).Render(s.icons.Language(r.Language)) + " " + st.shared.Muted.Render(r.Language)
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
		st.shared.Subtle.Write(&b, s.icons.Before+" ")
		used += 2
	}
	for i := start; i < len(t.tabs); i++ {
		label := t.tabs[i].label
		lw := ansi.StringWidth(label) + 2
		if used+lw > room {
			st.shared.Subtle.Write(&b, s.icons.Crumb)
			break
		}
		if i == t.cur {
			st.focusTitle.Write(&b, label)
		} else {
			st.shared.Muted.Write(&b, label)
		}
		b.WriteString("  ")
		used += lw
	}
	return ownerui.Spread(b.String(), right, w, s.icons.Ellipsis)
}

func tabsWidth(tabs []*owner) int {
	n := 0
	for _, o := range tabs {
		n += ansi.StringWidth(o.label) + 2
	}
	return n
}

// workBody renders the tabs of the lists of work above the one on view.
func (s *Section) workBody(w, h int) []string {
	st, l := &s.st, &s.tasks
	switch {
	case !s.work.ok && s.work.err != nil:
		return ownerui.Indent(s.failure("load your work", s.work.err, w-1))
	case !s.work.ok:
		return []string{" " + st.shared.Muted.Render("Loading the work waiting on you"+s.icons.Ellipsis)}
	}
	lines := make([]string, 0, h)
	lines = append(lines, s.workTabs(w))
	focused := s.focused && s.focus == workPane
	t := l.current()
	if t.refused {
		// GitHub refused this list's search and answered the others.
		return append(lines, ownerui.Indent(s.failure(workLists[l.cur].action, core.ErrForbidden, w-1))...)
	}
	for i := t.top; i < len(t.rows); i++ {
		// Rows show whole, but for one taller than the pane.
		if n := t.lines(i); len(lines)+n > h && i > t.top {
			break
		}
		r := &t.rows[i]
		if r.hit == nil {
			lines = append(lines, "   "+st.shared.Subtle.Render(r.note))
			continue
		}
		lines = s.workItem(lines, r, i == t.sel, focused, w)
	}
	return lines
}

// workTabs renders the tabs of the lists of work, each with its count, by
// their short titles when the full ones don't fit in w cells.
func (s *Section) workTabs(w int) string {
	st, l := &s.st, &s.tasks
	labels := make([]string, len(workLists))
	for _, short := range []bool{false, true} {
		n := 1
		for i, wl := range workLists {
			title := wl.title
			if short {
				title = wl.short
			}
			count := strconv.Itoa(l.tabs[i].count)
			if l.tabs[i].refused {
				// Not none: unknown.
				count = "?"
			}
			labels[i] = title + " " + count
			n += ansi.StringWidth(labels[i]) + 2
		}
		if n-2 <= w {
			break
		}
	}
	var b strings.Builder
	b.WriteByte(' ')
	for i, label := range labels {
		if i > 0 {
			b.WriteString("  ")
		}
		if i == l.cur {
			st.focusTitle.Write(&b, label)
		} else {
			st.shared.Muted.Write(&b, label)
		}
	}
	return b.String()
}

// workItem appends the lines of a pull request or issue waiting on the
// viewer: its state, where it is and its title, wrapped under the text
// with the age at the end. The cursor marks every line of the selected
// one.
func (s *Section) workItem(lines []string, r *workRow, selected, focused bool, w int) []string {
	st := &s.st
	gutter := "  "
	titleStyle := st.shared.Text
	if selected {
		gutter = st.shared.Blurred
		if focused {
			gutter = st.shared.Cursor
		}
		titleStyle = st.shared.Selected
	}
	hit := r.hit
	age := s.dates.Short(hit.Issue.UpdatedAt, s.now())
	for i, text := range r.lines {
		var b, link strings.Builder
		b.WriteString(gutter)
		used := workIndent
		if i == 0 {
			state := ui.HitState(*hit)
			st.states[state].Write(&b, s.icons.State(state))
			b.WriteByte(' ')
			st.shared.Muted.Write(&link, r.ref)
			used += ansi.StringWidth(r.ref)
			if text != "" {
				link.WriteByte(' ')
				used++
			}
		} else {
			b.WriteString("  ")
		}
		// Where it is and each line of its title link to its page.
		titleStyle.Write(&link, text)
		b.WriteString(s.links.Link(hit.Issue.URL, link.String()))
		used += ansi.StringWidth(text)
		if i == len(r.lines)-1 {
			b.WriteString(strings.Repeat(" ", max(w-used-ansi.StringWidth(age), 1)))
			st.shared.Subtle.Write(&b, age)
		}
		lines = append(lines, b.String())
	}
	return lines
}

func (s *Section) inboxBody(w, h int) []string {
	st := &s.st
	if s.inbox != nil {
		if text, hint, ok := ui.Unreadable(core.NeedNotifications, "load your notifications", s.voice); ok {
			lines := ui.ErrorLine(s.theme.Empty(s.icons), text, hint, max(w-1, 1))
			for i := range lines {
				lines[i] = " " + lines[i]
			}
			return lines
		}
	}
	switch {
	case s.inbox == nil:
		return []string{" " + st.shared.Muted.Render("Notifications aren't available.")}
	case !s.notes.ok && s.notes.err != nil:
		return ownerui.Indent(s.failure("load your notifications", s.notes.err, w-1))
	case !s.notes.ok:
		return []string{" " + st.shared.Muted.Render("Loading notifications"+s.icons.Ellipsis)}
	}
	n := s.unread()
	if n == "" {
		return []string{" " + st.shared.Muted.Render(ui.None("unread notifications"))}
	}
	lines := make([]string, 0, h)
	head := " " + st.shared.Text.Render(n) + st.shared.Muted.Render(" unread")
	if k := s.keys.Notifications.Help().Key; k != "" {
		head += st.shared.Subtle.Render(s.icons.Separator + s.icons.Key(k) + " shows them all")
	}
	lines = append(lines, head)
	l := &s.threads
	l.scroll(h - 1)
	focused := s.focused && s.focus == inboxPane
	for i := l.top; i < len(l.rows) && len(lines) < h; i++ {
		nt := &l.rows[i]
		gutter := "  "
		titleStyle := st.shared.Text
		if i == l.sel {
			gutter = st.shared.Blurred
			if focused {
				gutter = st.shared.Cursor
			}
			titleStyle = st.shared.Selected
		}
		age := s.dates.Short(nt.UpdatedAt, s.now())
		room := max(w-4-s.dates.Width()-1, 0)
		repo := ownerui.Truncate(nt.Repo.Name, min(ansi.StringWidth(nt.Repo.Name), room/3), s.icons.Ellipsis)
		title := ownerui.Truncate(ownerui.CleanLine(nt.Subject.Title), max(room-ansi.StringWidth(repo)-2, 0), s.icons.Ellipsis)
		used := 4 + ansi.StringWidth(repo) + 2 + ansi.StringWidth(title)
		// The repository and the title link to the thread's page.
		link := s.links.Link(nt.Subject.WebURL, st.shared.Muted.Render(repo)+"  "+titleStyle.Render(title))
		lines = append(lines, gutter+st.shared.Accent.Render(s.icons.Dot)+" "+link+
			strings.Repeat(" ", max(w-used-ansi.StringWidth(age), 1))+st.shared.Subtle.Render(age))
	}
	return lines
}

// failure renders err, which stopped action, in lines of w cells. Every
// read of the dashboard is the viewer's own, so it names no subject, and
// the open key opens the row under the cursor rather than what failed, so
// no hint names it.
func (s *Section) failure(action string, err error, w int) []string {
	text, hint := s.say(action, err)
	return ui.ErrorLine(s.errs, text, hint, w)
}

// say returns the text and the hint that failure renders. A read that was
// canceled has nothing to say, but the pane still offers to read again,
// rather than stay blank.
func (s *Section) say(action string, err error) (text, hint string) {
	v := s.voice
	v.Open.SetEnabled(false)
	text, hint = ui.ErrorText(action, "", v)(err)
	if k := v.Retry; text == "" && hint == "" && k.Enabled() && k.Help().Key != "" {
		hint = s.icons.Key(k.Help().Key) + " to retry"
	}
	return text, hint
}

// calendarBody centers the calendar in w cells.
func (s *Section) calendarBody(w int) []string {
	if !s.contribs.ok && s.contribs.err != nil {
		return ownerui.Indent(s.failure("load your contributions", s.contribs.err, w-1))
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
