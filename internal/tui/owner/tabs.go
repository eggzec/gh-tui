package owner

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// tab is a tab of the list pane, which titles the pane.
type tab int

// The tabs of the list pane: a user's are the repositories, the stars,
// the followers, the accounts followed and the organizations, and an
// organization's the repositories, the members and the teams.
const (
	reposTab tab = iota
	starsTab
	followersTab
	followingTab
	orgsTab
	membersTab
	teamsTab
	numTabs
)

var (
	tabTitles = [numTabs]string{"Repositories", "Stars", "Followers", "Following", "Organizations", "Members", "Teams"}
	// shortTabs name the tabs where their titles don't fit, such as in a
	// narrow frame that names every pane.
	shortTabs = [numTabs]string{"Repos", "Stars", "Followers", "Following", "Orgs", "Members", "Teams"}
	// tabNames are the tabs owner.default_tab may name that the page has.
	// The people are a user's followers, which are an organization's
	// members on its page.
	tabNames = map[string]tab{config.OwnerTabRepositories: reposTab, config.OwnerTabPeople: followersTab}

	userTabs = []tab{reposTab, starsTab, followersTab, followingTab, orgsTab}
	orgTabs  = []tab{reposTab, membersTab, teamsTab}
)

// tabFor returns the tab name opens a page on, or the repositories when the
// page has no such tab.
func tabFor(name string) tab {
	if t, ok := tabNames[name]; ok {
		return t
	}
	return reposTab
}

// tabsOf returns the tabs of the page of an account of kind.
func tabsOf(kind core.OwnerKind) []tab {
	if kind == core.OwnerOrg {
		return orgTabs
	}
	return userTabs
}

// tabIn returns t on the page of an account of kind: t if the page has it,
// the members for the followers of an organization, which are its people
// too, and else the repositories.
func tabIn(t tab, kind core.OwnerKind) tab {
	if slices.Contains(tabsOf(kind), t) {
		return t
	}
	if t == followersTab && kind == core.OwnerOrg {
		return membersTab
	}
	return reposTab
}

// feedModel is what the page does with the feed of a tab, whatever it
// lists.
type feedModel interface {
	Init() tea.Cmd
	Reload() tea.Cmd
	Focus()
	Blur()
	SetStyles(feed.Styles)
	Settled() bool
	Len() int
	Done() bool
	View() string
	Err() error
	Retry() tea.Cmd
	RetryKept() tea.Cmd
	KeyMap() feed.KeyMap
	// Capturing reports whether the prompt of the find or filter is
	// open, and Takes whether the feed handles msg before the page.
	Capturing() bool
	Takes(msg tea.KeyPressMsg) bool
	// ShortHelp and FullHelp make the feed what the help reads.
	ShortHelp() []key.Binding
	FullHelp() [][]key.Binding
}

// lister is the list of a tab.
type lister interface {
	feed() feedModel
	// update gives msg to the feed.
	update(msg tea.Msg) tea.Cmd
	// start fetches the first page, unless the list did already.
	start() tea.Cmd
	started() bool
	// fresh reports whether the first page is cached and fresh, so that
	// reading it again costs no request.
	fresh(svc Service) bool
	// resize gives the list width by height cells, below the tabs and the
	// headers of its columns.
	resize(s *Section, width, height int)
	// header renders the headers of the columns, or "" while there are
	// no rows under them.
	header(s *Section) string
	// remeasure measures the rows read since it last did, and reports
	// whether the columns need laying out again.
	remeasure(s *Section) bool
	// selection is what the cursor is on, and enter opens it.
	selection(s *Section) (ui.Selection, bool)
	enter(s *Section) tea.Cmd
}

// feedTab is the part of a lister that holds the feed.
type feedTab[T any] struct {
	Feed feed.Model[T]
	// on is set once the feed has fetched its first page.
	on bool
}

func (l *feedTab[T]) feed() feedModel { return &l.Feed }

func (l *feedTab[T]) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.Feed, cmd = l.Feed.Update(msg)
	return cmd
}

func (l *feedTab[T]) start() tea.Cmd {
	if l.on {
		return nil
	}
	l.on = true
	return l.Feed.Init()
}

func (l *feedTab[T]) started() bool { return l.on }
