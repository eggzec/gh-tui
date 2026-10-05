package owner

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The headers of the columns of people.
const (
	loginHeader = "Login"
	nameHeader  = "Name"
	aboutHeader = "About"
	roleHeader  = "Role"
)

// Bounds of the columns of people and teams: the gap between two, the
// widest a login or a name gets while there is room for more, and the
// narrowest the last column may be before it gives way.
const (
	colGap   = 2
	maxLogin = 20
	maxName  = 24
	minLast  = 12
)

// peopleList is a tab that lists accounts: a user's followers, those they
// follow or their organizations, or an organization's members. enter opens
// the page of the account under the cursor.
type peopleList struct {
	q owners.PeopleQuery
	feedTab[core.Person]
	// roles shows the role of each member in the last column, instead of
	// what they say of themselves. Only members see the roles.
	roles bool
	// login and name are the widest of each read so far, and seen how many
	// were measured. They only grow, so the columns don't jump as the
	// list scrolls.
	login, name, seen int
	cols              peopleCols
}

// peopleCols are the widths of the columns of people. A width of 0 drops
// a column: narrow lists drop the last first, then the names.
type peopleCols struct {
	login, name, last int
}

// newPeopleList returns the tab t of the account login, one of its lists
// of people. An organization's members show their roles if roles is set.
func (s *Section) newPeopleList(t tab, login string, roles bool) *peopleList {
	lists := map[tab]owners.PeopleList{
		followersTab: owners.Followers, followingTab: owners.Following,
		orgsTab: owners.Orgs, membersTab: owners.Members,
	}
	empty := map[tab]string{
		followersTab: "No followers.",
		followingTab: "Not following anyone.",
		orgsTab:      "No public organizations.",
		membersTab:   login + " has no public members.",
	}
	if roles {
		empty[membersTab] = login + " has no members."
	}
	l := &peopleList{q: owners.PeopleQuery{Login: login, List: lists[t]}, roles: roles}
	query := func(cursor string) owners.PeopleQuery {
		q := l.q
		q.Cursor = cursor
		return q
	}
	svc := s.svc
	read := func(ctx context.Context, q owners.PeopleQuery, again bool) (core.Page[core.Person], error) {
		q.Again = again
		return svc.People(ctx, q)
	}
	render := func(p core.Person, selected bool, _ int) string {
		return s.personRow(l.cols, p, selected, l.roles)
	}
	what := strings.ToLower(tabTitles[t])
	l.Feed = feed.New(ui.FeedPages("owner."+l.q.List.String(), query, read), render,
		s.feedOptions(feed.WithKey(func(p core.Person) string { return p.Login }), empty[t], "load the "+what, login)...)
	return l
}

// feedOptions are the options of the feed of every tab: key names each
// row, empty is what the feed says while it has none, and the action that
// failed, such as "load the followers", is of the account login.
func (s *Section) feedOptions(key feed.Option, empty, action, login string) []feed.Option {
	return []feed.Option{
		feed.WithContext(s.ctx),
		key,
		feed.WithKeyMap(s.keys.feed),
		feed.WithEmptyText(empty),
		feed.WithStyles(s.theme.Feed(s.icons)),
		feed.WithErrorText(ui.ErrorText(action, login, s.voice)),
	}
}

func (l *peopleList) fresh(svc Service) bool { return svc.FreshPeople(l.q) }

func (l *peopleList) resize(_ *Section, width, height int) {
	l.Feed.SetSize(width, max(height-listTop, 0))
	l.layout(max(width-gutterWidth, 0))
}

// layout fits the columns in width cells.
func (l *peopleList) layout(width int) {
	c := peopleCols{
		login: min(max(l.login, len(loginHeader)), maxLogin),
		name:  min(max(l.name, len(nameHeader)), maxName),
	}
	if rest := width - c.login - c.name - 2*colGap; rest >= minLast {
		c.last = rest
		l.cols = c
		return
	}
	// Without the last column the name takes the rest, and without room
	// for a name the login takes it all.
	c.name = width - c.login - colGap
	if c.name < len(nameHeader) {
		c.login, c.name = width, 0
	}
	l.cols = c
}

func (l *peopleList) remeasure(*Section) bool {
	n := l.Feed.Len()
	if n == l.seen {
		return false
	}
	login, name := l.login, l.name
	for i := range n {
		if p, ok := l.Feed.Item(i); ok {
			login = max(login, ansi.StringWidth(p.Login))
			name = max(name, ansi.StringWidth(ownerui.CleanLine(p.Name)))
		}
	}
	l.seen = n
	if login == l.login && name == l.name {
		return false
	}
	l.login, l.name = login, name
	return true
}

func (l *peopleList) header(s *Section) string {
	if l.Feed.Len() == 0 {
		return ""
	}
	last := aboutHeader
	if l.roles {
		last = roleHeader
	}
	r := s.newRow()
	r.cell(loginHeader, l.cols.login, false, s.st.shared.Subtle, "")
	r.cell(nameHeader, l.cols.name, false, s.st.shared.Subtle, "")
	r.cell(last, l.cols.last, false, s.st.shared.Subtle, "")
	return strings.Repeat(" ", gutterWidth) + r.String()
}

// personRow renders p in its columns c: the login, which links to the
// profile, the name, and the role if roles is set, or else the bio.
func (s *Section) personRow(c peopleCols, p core.Person, selected, roles bool) string {
	st := &s.st.shared
	login := st.Text
	if selected {
		login = st.Selected
	}
	last := ownerui.CleanLine(p.Bio)
	if roles {
		last = roleName(p.Role)
	}
	r := s.newRow()
	r.cell(p.Login, c.login, false, login, s.profileURL(p.Login))
	r.cell(ownerui.CleanLine(p.Name), c.name, false, st.Muted, "")
	r.cell(last, c.last, false, st.Subtle, "")
	return r.String()
}

// roleName names role as GitHub does, or "" for a role the viewer can't
// see.
func roleName(role core.MemberRole) string {
	switch role {
	case core.MemberRoleAdmin:
		return "Owner"
	case core.MemberRoleMember:
		return "Member"
	default:
		return ""
	}
}

func (l *peopleList) selection(s *Section) (ui.Selection, bool) {
	p, ok := l.Feed.Selected()
	if !ok {
		return ui.Selection{}, false
	}
	what := "user"
	if p.Kind == core.OwnerOrg {
		what = "organization"
	}
	return ui.Selection{What: what, URL: s.profileURL(p.Login)}, true
}

func (l *peopleList) enter(*Section) tea.Cmd {
	p, ok := l.Feed.Selected()
	if !ok {
		return nil
	}
	return func() tea.Msg { return ui.OwnerMsg{Login: p.Login} }
}

// profileURL is the page of the account login on GitHub.
func (s *Section) profileURL(login string) string {
	return ui.WebURL(s.host, login)
}

// row builds a row of cells, each in its width and colGap apart.
type row struct {
	b strings.Builder
	// cells counts the cells added, which the gaps go between.
	cells int
	// tail ends the text of a cell where it is cut, and links makes the
	// links of the cells that have one.
	tail  string
	links *termtext.Links
}

// newRow returns a row in the page's icons and links.
func (s *Section) newRow() *row {
	return &row{tail: s.icons.Ellipsis, links: &s.links}
}

// cell adds plain text, cut to w cells and padded to them, on the left or
// on the right, in paint, and linked to the address link if it isn't "".
// A width of 0 adds nothing.
func (r *row) cell(text string, w int, right bool, paint ownerui.Paint, link string) {
	if w <= 0 {
		return
	}
	if r.cells > 0 {
		r.b.WriteString(strings.Repeat(" ", colGap))
	}
	r.cells++
	text = ownerui.Truncate(text, w, r.tail)
	pad := strings.Repeat(" ", w-ansi.StringWidth(text))
	if right {
		r.b.WriteString(pad)
	}
	styled := paint.Render(text)
	if link != "" {
		styled = r.links.Link(link, styled)
	}
	r.b.WriteString(styled)
	if !right {
		r.b.WriteString(pad)
	}
}

func (r *row) String() string { return r.b.String() }
