package owner

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// The headers of the columns of teams. The flag of a secret team has
// none: its glyph speaks for itself.
const (
	teamHeader    = "Team"
	membersHeader = "Members"
	descHeader    = "Description"
)

// teamList is the Teams tab of an organization, by name. There is no page
// of a team here, so only the open key opens one, on GitHub.
type teamList struct {
	q owners.TeamsQuery
	feedTab[core.Team]
	// membersOnly is set while the viewer isn't a member of the
	// organization, whose teams only members see. The tab then says so
	// instead of asking GitHub.
	membersOnly bool
	// name, members and secret measure the teams read so far: the widest
	// name, the widest count of members and whether any is secret. seen
	// is how many were measured.
	name, members, seen int
	secret              bool
	cols                teamCols
}

// teamCols are the widths of the columns of teams. A width of 0 drops a
// column: narrow lists drop the description first.
type teamCols struct {
	name, flag, members, desc int
}

// newTeamList returns the list of the teams of the organization login,
// which says that only members see them if membersOnly is set.
func (s *Section) newTeamList(login string, membersOnly bool) *teamList {
	l := &teamList{q: owners.TeamsQuery{Login: login}, membersOnly: membersOnly}
	query := func(cursor string) owners.TeamsQuery {
		q := l.q
		q.Cursor = cursor
		return q
	}
	svc := s.svc
	read := func(ctx context.Context, q owners.TeamsQuery, again bool) (core.Page[core.Team], error) {
		q.Again = again
		return svc.Teams(ctx, q)
	}
	render := func(t core.Team, selected bool, _ int) string {
		return s.teamRow(l.cols, t, selected)
	}
	l.Feed = feed.New(ui.FeedPages("owner.teams", query, read), render,
		s.feedOptions(feed.WithKey(func(t core.Team) string { return t.Slug }), "No teams you can see.", "load the teams", login)...)
	return l
}

// start fetches the first page, unless only members see the teams.
func (l *teamList) start() tea.Cmd {
	if l.membersOnly {
		return nil
	}
	return l.feedTab.start()
}

func (l *teamList) fresh(svc Service) bool { return l.membersOnly || svc.FreshTeams(l.q) }

// hidden reports whether the viewer can't see the teams: the header says
// they aren't a member, or GitHub refused the teams for a reason other
// than SSO, as it does to someone outside the organization whose header
// is out of date.
func (l *teamList) hidden() bool {
	if l.membersOnly {
		return true
	}
	err := l.Feed.Err()
	return l.Feed.Len() == 0 && errors.Is(err, core.ErrForbidden) && !ui.SSO(err)
}

func (l *teamList) resize(s *Section, width, height int) {
	l.Feed.SetSize(width, max(height-listTop, 0))
	l.layout(max(width-gutterWidth, 0), ansi.StringWidth(s.icons.Private))
}

// layout fits the columns in width cells, with flag cells for the glyph
// of a secret team.
func (l *teamList) layout(width, flag int) {
	c := teamCols{
		name:    min(max(l.name, len(teamHeader)), maxName),
		members: max(l.members, len(membersHeader)),
	}
	if l.secret {
		c.flag = flag
	}
	used := c.name + c.members + colGap
	if c.flag > 0 {
		used += c.flag + colGap
	}
	if rest := width - used - colGap; rest >= minLast {
		c.desc = rest
	} else {
		c.name = max(width-used+c.name, 0)
	}
	l.cols = c
}

func (l *teamList) remeasure(*Section) bool {
	n := l.Feed.Len()
	if n == l.seen {
		return false
	}
	name, members, secret := l.name, l.members, l.secret
	for i := range n {
		if t, ok := l.Feed.LoadedItem(i); ok {
			name = max(name, ansi.StringWidth(ownerui.CleanLine(t.Name)))
			members = max(members, len(ownerui.Count(t.Members)))
			secret = secret || t.Secret
		}
	}
	l.seen = n
	if name == l.name && members == l.members && secret == l.secret {
		return false
	}
	l.name, l.members, l.secret = name, members, secret
	return true
}

func (l *teamList) header(s *Section) string {
	if l.hidden() || l.Feed.Len() == 0 {
		return ""
	}
	sub := s.st.shared.Subtle
	r := s.newRow()
	r.cell(teamHeader, l.cols.name, false, sub, "")
	r.cell("", l.cols.flag, false, sub, "")
	r.cell(membersHeader, l.cols.members, true, sub, "")
	r.cell(descHeader, l.cols.desc, false, sub, "")
	return strings.Repeat(" ", gutterWidth) + r.String()
}

// teamRow renders t in its columns c: the name, which links to the team,
// whether it is secret, how many members it has, and its description.
func (s *Section) teamRow(c teamCols, t core.Team, selected bool) string {
	st := &s.st.shared
	name := st.Text
	if selected {
		name = st.Selected
	}
	flag := ""
	if t.Secret {
		flag = s.icons.Private
	}
	r := s.newRow()
	r.cell(ownerui.CleanLine(t.Name), c.name, false, name, t.URL)
	r.cell(flag, c.flag, false, st.Subtle, "")
	r.cell(ownerui.Count(t.Members), c.members, true, st.Muted, "")
	r.cell(ownerui.CleanLine(t.Description), c.desc, false, st.Muted, "")
	return r.String()
}

func (l *teamList) selection(*Section) (ui.Selection, bool) {
	t, ok := l.Feed.Selected()
	if !ok || l.hidden() {
		return ui.Selection{}, false
	}
	return ui.Selection{What: "team", URL: t.URL}, true
}

// enter does nothing: there is no page of a team here, and o opens it
// in the browser.
func (l *teamList) enter(*Section) tea.Cmd { return nil }

// membersOnlyText is what the Teams tab says to someone outside the
// organization login.
func membersOnlyText(login string) string {
	return "Only members of " + login + " see its teams."
}
