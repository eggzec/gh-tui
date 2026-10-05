package owner

import (
	"cmp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Sizes of the page. From wideWidth by wideHeight cells it shows every
// pane at once: the profile on top, the pinned cards below it and the list
// below them. Below that, or zoomed, it shows the profile and the focused
// pane, whose frame names the others. They are the dashboard's, so the
// two pages change layout at the same size.
const (
	wideWidth    = 100
	wideHeight   = 30
	pinnedHeight = ownerui.CardHeight + 2
)

// box is the outer size of a pane, frame included.
type box struct{ w, h int }

// layout sizes the panes for the page's size.
func (s *Section) layout() {
	s.wide = s.width >= wideWidth && s.height >= wideHeight
	rest := max(s.height-s.profileHeight(), 0)
	var b [numPanes]box
	if s.onePane() {
		for i := range b {
			b[i] = box{s.width, rest}
		}
	} else {
		b[pinnedPane] = box{s.width, pinnedHeight}
		b[listPane] = box{s.width, max(rest-pinnedHeight, 0)}
	}
	s.boxes = b
	p := s.page
	if p == nil {
		return
	}
	p.pinned.Resize(s.inside(pinnedPane))
	for _, l := range p.lists {
		if l != nil {
			s.layoutList(l)
		}
	}
}

// inside is the room inside the frame of pane p.
func (s *Section) inside(p paneID) (width, height int) {
	return max(s.boxes[p].w-2, 0), max(s.boxes[p].h-2, 0)
}

// layoutList sizes l, and lays its columns out, to fit its pane.
func (s *Section) layoutList(l lister) {
	w, h := s.inside(listPane)
	l.resize(s, w, h)
}

// onePane reports whether the page shows the focused pane alone: when the
// panes don't fit, or while it is zoomed.
func (s *Section) onePane() bool { return !s.wide || s.zoom }

// zoomed reports whether the zoom shows. A page too small for every pane
// shows one anyway, so there the back key keeps its other uses.
func (s *Section) zoomed() bool { return s.zoom && s.wide }

// setZoom shows the focused pane alone, or every pane again.
func (s *Section) setZoom(zoom bool) {
	s.zoom = zoom
	s.layout()
}

// render renders the profile and every pane, and the page from them.
func (s *Section) render() {
	s.head = s.profile()
	for p := range numPanes {
		s.renderPane(p)
	}
	s.compose()
}

// renderPane renders pane p in its frame. Compose the page after.
func (s *Section) renderPane(p paneID) {
	if s.width <= 0 || s.height <= 0 || s.page == nil || s.onePane() && p != s.page.focus {
		return
	}
	w, h := s.inside(p)
	var body []string
	switch p {
	case pinnedPane:
		body = s.pinnedBody(w, h)
	default:
		body = s.listBody(w, h)
	}
	label, lw := s.label(p)
	edge := s.st.edge
	if s.focused && p == s.page.focus {
		edge = s.st.focusEdge
	}
	b := s.boxes[p]
	s.frames[p] = ownerui.Frame(label, lw, b.w, b.h, edge, s.icons.Border, s.icons.Ellipsis, body)
}

// compose joins the profile and the framed panes into the view.
func (s *Section) compose() {
	if s.width <= 0 || s.height <= 0 {
		s.view = ""
		return
	}
	lines := make([]string, 0, s.height+s.profileHeight())
	lines = append(lines, s.head...)
	switch {
	case s.page == nil:
	case s.onePane():
		lines = append(lines, s.frames[s.page.focus]...)
	default:
		lines = append(lines, s.frames[pinnedPane]...)
		lines = append(lines, s.frames[listPane]...)
	}
	blank := strings.Repeat(" ", s.width)
	for len(lines) < s.height {
		lines = append(lines, blank)
	}
	s.view = strings.Join(lines[:s.height], "\n")
}

// title is the title of pane p: the list pane's is that of its tab, or
// its short one if short is set.
func (s *Section) title(p paneID, short bool) string {
	if p != listPane {
		return paneTitles[p]
	}
	titles := tabTitles
	if short {
		titles = shortTabs
	}
	return s.tabTitle(s.page.tab, titles)
}

// tabTitle is the title of tab t among titles. Until the header says
// whether the account is a user or an organization, a tab of people goes
// by no title, since it may be either's.
func (s *Section) tabTitle(t tab, titles [numTabs]string) string {
	if t != reposTab && !s.page.header.ok {
		return "Loading" + s.icons.Ellipsis
	}
	return titles[t]
}

// tabWord names the tab on view in a sentence, such as "repositories", or
// "lists" while tabTitle gives it no title.
func (s *Section) tabWord() string {
	if s.page.tab != reposTab && !s.page.header.ok {
		return "lists"
	}
	return strings.ToLower(tabTitles[s.page.tab])
}

// label is the text in the top edge of pane p, and its width. In the
// narrow layout the one frame names every pane, the focused one in full.
func (s *Section) label(p paneID) (label string, width int) {
	st := &s.st
	title := st.title
	if s.focused && p == s.page.focus {
		title = st.focusTitle
	}
	if !s.onePane() {
		text := s.paneLabel(p)
		return title.Render(text), ansi.StringWidth(text)
	}
	// The other panes go by their titles, then by shorter ones, then by
	// their keys alone, whichever fits.
	for tier := range 3 {
		var b strings.Builder
		n := 0
		for i := range numPanes {
			part := s.paneLabel(i)
			if i != p {
				name := ""
				if tier < 2 {
					name = s.title(i, tier == 1)
				}
				part = strings.TrimSpace(s.keyLabel(i) + " " + name)
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

// paneLabel is the label of pane p, such as "[2] Repositories · go".
func (s *Section) paneLabel(p paneID) string {
	text := s.title(p, false)
	if k := s.keyLabel(p); k != "" {
		text = "[" + k + "] " + text
	}
	switch p {
	case pinnedPane:
		if at, of := s.page.pinned.Pages(); of > 1 {
			text += s.icons.Separator + strconv.Itoa(at) + "/" + strconv.Itoa(of)
		}
	default:
		if l := s.repoTab(); l != nil {
			if chips := l.Filter().Chips(s.icons); chips != "" {
				text += s.icons.Separator + chips
			}
		}
	}
	return text
}

// profileHeight is how many lines the profile takes.
func (s *Section) profileHeight() int {
	return ownerui.ProfileHeight(s.avatarShown())
}

// avatarShown reports whether the profile shows the avatar of the account:
// where avatars are drawn, on a page wide enough.
func (s *Section) avatarShown() bool {
	return s.avatars.Shown() && s.width >= ownerui.MinAvatarWidth
}

// drawer draws the profile, the pinned cards and the rows in the page's
// styles.
func (s *Section) drawer() ownerui.Drawer {
	return ownerui.Drawer{
		Styles: &s.st.shared, Icons: s.icons, Links: &s.links, URL: s.repoURL,
		Lang: func(r core.Repo) ownerui.Paint { return s.langPaint(r.Language, r.LanguageColor) },
	}
}

// profile renders the lines above the panes: who the account is, and how
// the page is doing, beside the avatar where it is drawn.
func (s *Section) profile() []string {
	w, n := s.width, s.profileHeight()
	p := s.page
	if w <= 2 || p == nil {
		return ownerui.BlankProfile(w, n)
	}
	h := p.header
	var box []string
	if s.avatarShown() {
		avatar := ""
		if h.ok {
			avatar = ui.SizedAvatar(h.value.Profile.AvatarURL, ui.AvatarLarge)
		}
		box = s.avatars.Box(avatar, ui.AvatarLarge)
		w -= 1 + ui.AvatarLarge.Cols
	}
	st := &s.st.shared
	var first, second, right string
	switch {
	case h.ok:
		first, second = s.nameLine(h.value), s.facts(h.value)
	case h.err != nil:
		// The error takes both lines of the profile, and no more: when
		// the hint would need a third, the text gives up all but its
		// first line to it.
		text, hint := s.say("load the profile of "+p.login, h.err)
		lines := ui.ErrorLine(s.errs, text, hint, w-1)
		if len(lines) > 2 {
			mark := ansi.StringWidth(s.errs.Mark + " ")
			lines = ui.ErrorLine(s.errs, termtext.Truncate(text, w-1-mark, s.icons.Ellipsis), hint, w-1)
		}
		lines = append(lines, "", "")
		first, second = lines[0], lines[1]
	default:
		first = st.Muted.Render("Loading " + p.login + s.icons.Ellipsis)
	}
	switch {
	case h.ok && h.value.Offline:
		right = s.st.warning.Render(ui.SayKept(core.Offline, s.icons))
	case h.ok && h.value.Limited:
		right = s.st.warning.Render(ui.SayKept(core.RateLimited, s.icons))
	case s.updating():
		right = st.Subtle.Render("updating" + s.icons.Ellipsis)
	}
	return s.drawer().Profile(box, first, second, right, w, n)
}

// nameLine renders the first line of the profile of o: the name, the login,
// whether GitHub verified the organization, the pronouns of the user and
// the bio or description.
func (s *Section) nameLine(o core.Owner) string {
	st := &s.st.shared
	p := o.Profile
	var b strings.Builder
	st.Name.Write(&b, ownerui.CleanLine(cmp.Or(p.Name, p.Login)))
	b.WriteByte(' ')
	st.Login.Write(&b, "@"+ownerui.CleanLine(p.Login))
	parts := make([]string, 0, 3)
	if o.Verified {
		parts = append(parts, st.Accent.Render("verified"))
	}
	if t := ownerui.CleanLine(o.Pronouns); t != "" {
		parts = append(parts, st.Muted.Render(t))
	}
	if t := ownerui.CleanLine(p.Bio); t != "" {
		parts = append(parts, st.Text.Render(t))
	}
	for _, part := range parts {
		st.Subtle.Write(&b, s.icons.Separator)
		b.WriteString(part)
	}
	return b.String()
}

// facts renders the second line of the profile of o: for a user, the
// company, location and website, the follows and the status; for an
// organization, the location, website and email, the repositories, and
// whether the viewer is a member.
func (s *Section) facts(o core.Owner) string {
	st := &s.st.shared
	p := o.Profile
	parts := make([]string, 0, 8)
	muted := func(texts ...string) {
		for _, t := range texts {
			if t = ownerui.CleanLine(t); t != "" {
				parts = append(parts, st.Muted.Render(t))
			}
		}
	}
	count := func(n int, one, many string) {
		parts = append(parts, st.Text.Render(ownerui.Count(n))+st.Muted.Render(ownerui.Plural(n, one, many)))
	}
	if o.Kind == core.OwnerOrg {
		muted(p.Location, site(p.Website), o.Email)
		count(p.Repos, " repository", " repositories")
		if o.Viewer.Member {
			parts = append(parts, st.Accent.Render("member"))
		}
		return strings.Join(parts, st.Subtle.Render(s.icons.Separator))
	}
	muted(p.Company, p.Location, site(p.Website))
	count(p.Followers, " follower", " followers")
	count(p.Following, " following", " following")
	if o.Viewer.FollowsViewer {
		parts = append(parts, st.Accent.Render("follows you"))
	}
	if m := ownerui.CleanLine(p.Status.Message); m != "" || p.Status.Busy {
		if p.Status.Busy {
			m = strings.TrimSpace("busy " + m)
		}
		parts = append(parts, st.Accent.Render(m))
	}
	return strings.Join(parts, st.Subtle.Render(s.icons.Separator))
}

// site is a website as a profile shows it, without its scheme or a slash
// at its end.
func site(url string) string {
	url = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	return strings.TrimSuffix(url, "/")
}

func (s *Section) pinnedBody(w, h int) []string {
	p, st := s.page, &s.st.shared
	c := &p.pinned
	if c.Cols == 0 {
		return nil
	}
	if len(c.Items) == 0 {
		switch {
		case p.header.err != nil && !p.header.ok:
			return ownerui.Indent(s.failure("load the pins of "+p.login, p.header.err, w-1))
		case !p.header.ok:
			return []string{" " + st.Muted.Render("Loading pinned repositories"+s.icons.Ellipsis)}
		}
		return []string{" " + st.Muted.Render(ui.None("pinned repositories"))}
	}
	return c.Lines(w, h, s.card)
}

// card renders a pinned repository in w cells, marked when selected.
func (s *Section) card(c ownerui.Card, selected bool, w int) [ownerui.CardHeight]string {
	gutter := "  "
	if selected {
		gutter = s.st.shared.Blurred
		if s.focused && s.page.focus == pinnedPane {
			gutter = s.st.shared.Cursor
		}
	}
	return s.drawer().Card(c, gutter, w)
}

// listBody renders the tabs of the list pane above the list of the tab on
// view.
func (s *Section) listBody(w, h int) []string {
	p, st := s.page, &s.st.shared
	lines := make([]string, 0, h)
	lines = append(lines, s.tabsLine(w))
	l := p.list()
	switch {
	case l == nil && p.header.err != nil:
		return append(lines, ownerui.Indent(s.failure("load the "+s.tabWord()+" of "+p.login, p.header.err, w-1))...)
	case l == nil:
		return append(lines, " "+st.Muted.Render("Loading "+s.tabWord()+s.icons.Ellipsis))
	}
	if t, ok := l.(*teamList); ok && t.hidden() {
		return append(lines, "", " "+st.Muted.Render(membersOnlyText(s.Login())))
	}
	// The headers name the columns once there are rows under them.
	lines = append(lines, l.header(s))
	if body := l.feed().View(); body != "" {
		lines = append(lines, strings.Split(body, "\n")...)
	}
	return lines
}

// tabsLine renders the tabs of the list pane, each with its count once the
// header says it, and on the right the language of the repository under
// the cursor, which the list shows as a glyph. The tabs go by their short
// titles where their titles don't fit.
func (s *Section) tabsLine(w int) string {
	p := s.page
	var right string
	switch l := p.list().(type) {
	case *repoList:
		right = s.language(&l.tableTab)
	case *starList:
		right = s.language(&l.tableTab)
	}
	tabs := []tab{p.tab}
	if p.header.ok {
		tabs = tabsOf(p.header.value.Kind)
	}
	line := s.tabs(tabs, tabTitles)
	if ansi.StringWidth(line) > w {
		line = s.tabs(tabs, shortTabs)
	}
	return ownerui.Spread(line, right, w, s.icons.Ellipsis)
}

// tabs renders tabs by their titles, with their counts once the header
// says them.
func (s *Section) tabs(tabs []tab, titles [numTabs]string) string {
	p, st := s.page, &s.st.shared
	var b strings.Builder
	b.WriteByte(' ')
	for i, t := range tabs {
		if i > 0 {
			b.WriteString("  ")
		}
		label := s.tabTitle(t, titles)
		if count := s.tabCount(t); count != "" {
			label += " " + count
		}
		if t == p.tab {
			s.st.focusTitle.Write(&b, label)
		} else {
			st.Muted.Write(&b, label)
		}
	}
	return b.String()
}

// language renders the language of the repository under the cursor of l,
// or "" when it has none.
func (s *Section) language(l *tableTab) string {
	r, ok := l.Feed.Selected()
	if !ok || r.Language == "" {
		return ""
	}
	return s.langPaint(r.Language, r.LanguageColor).Render(s.icons.Language(r.Language)) + " " + s.st.shared.Muted.Render(r.Language)
}

// tabCount is how many items tab t of the page on view lists, as its
// header counts them, or "" where it doesn't: for the organizations of a
// user, which are counted once all are read, and for the teams of an
// organization to someone outside it. Someone outside an organization
// sees only its public members.
func (s *Section) tabCount(t tab) string {
	p := s.page
	if !p.header.ok {
		return ""
	}
	o := p.header.value
	n := 0
	switch t {
	case reposTab:
		n = o.Profile.Repos
	case starsTab:
		n = o.Stars
	case followersTab:
		n = o.Profile.Followers
	case followingTab:
		n = o.Profile.Following
	case orgsTab:
		l := p.lists[t]
		if l == nil || !l.started() || !l.feed().Done() || l.feed().Err() != nil {
			return ""
		}
		n = l.feed().Len()
	case membersTab:
		if !o.Viewer.Member {
			return ownerui.Count(o.Members) + " public"
		}
		n = o.Members
	case teamsTab:
		if !o.Viewer.Member {
			return ""
		}
		n = o.Teams
	default:
	}
	return ownerui.Count(n)
}

// failure renders err, which stopped action, in lines of w cells. The
// open key opens what is under the cursor rather than what failed, so no
// hint names it.
func (s *Section) failure(action string, err error, w int) []string {
	text, hint := s.say(action, err)
	return ui.ErrorLine(s.errs, text, hint, w)
}

// say returns the text and the hint that failure renders. A read that was
// canceled has nothing to say, but the page still offers to read again,
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
