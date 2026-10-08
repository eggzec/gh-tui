package owner

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ Service = (*owners.Service)(nil)

// now is the clock of every test, so ages are stable.
var now = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

// fakeService serves fixed accounts by their logins in lower case.
// Repositories come in pages of size, with the start index as cursor.
type fakeService struct {
	mu     sync.Mutex
	owners map[string]core.Owner
	repos  map[string][]core.Repo
	// stars and teams are by login, and people by login and list, as
	// "octocat followers".
	stars  map[string][]core.Repo
	people map[string][]core.Person
	teams  map[string][]core.Team
	size   int
	// offline marks every header served offline.
	offline bool
	// fail, when set, fails the read of that kind: header, repos, stars,
	// people or teams.
	fail map[string]error
	// read holds what was read and is fresh: "header login", "repos
	// login", "stars login", "people login list", "teams login".
	read        map[string]bool
	calls       []string
	invalidated []string
	// sideFake serves the READMEs, calendars, sponsors and follower
	// counts.
	*sideFake
}

func newFake() *fakeService {
	return &fakeService{
		owners: map[string]core.Owner{"octocat": user(), "github": org()},
		repos:  map[string][]core.Repo{"octocat": repos("octocat", 40), "github": repos("github", 6)},
		stars:  map[string][]core.Repo{"octocat": starred()},
		people: map[string][]core.Person{
			"octocat followers": people(core.OwnerUser, 40),
			"octocat following": people(core.OwnerUser, 3),
			"octocat orgs":      orgs(),
			"github members":    members(),
		},
		teams: map[string][]core.Team{"github": teams()},
		size:  30,
		fail:  map[string]error{},
		read:  map[string]bool{},

		sideFake: newSideFake(),
	}
}

func user() core.Owner {
	return core.Owner{
		Kind: core.OwnerUser,
		Profile: core.Profile{
			Login: "octocat", Name: "Mona Lisa Octocat", Bio: "Building tools for the terminal",
			Company: "@github", Location: "San Francisco", Website: "https://octocat.dev/", Followers: 1234, Following: 56, Repos: 40,
			Status: core.Status{Busy: true, Message: "Shipping the dashboard"},
			URL:    "https://github.com/octocat", AvatarURL: "https://avatars.githubusercontent.com/u/583231?v=4",
		},
		Pronouns: "she/her",
		Stars:    4,
		Pinned: []core.Repo{
			pinnedRepo("octocat", "hello-world", "My first repository on GitHub, with a description long enough to wrap onto a second line", "Go", 2345),
			pinnedRepo("octocat", "spoon-knife", "This repo is for demonstration purposes only.", "HTML", 12800),
			pinnedRepo("charmbracelet", "bubbletea", "A powerful little TUI framework", "Go", 31000),
			pinnedRepo("cli", "cli", "GitHub's official command line tool", "Go", 38000),
		},
		Viewer: core.Relation{FollowsViewer: true},
	}
}

func org() core.Owner {
	return core.Owner{
		Kind: core.OwnerOrg,
		Profile: core.Profile{
			Login: "github", Name: "GitHub", Bio: "How people build software", Location: "San Francisco",
			Website: "https://github.com", Repos: 6, URL: "https://github.com/github",
		},
		Email: "support@github.com", Verified: true, Members: 5, Teams: 3,
		Pinned: []core.Repo{
			pinnedRepo("github", "docs", "The open-source repo for docs.github.com", "TypeScript", 17000),
			pinnedRepo("github", "gitignore", "A collection of useful .gitignore templates", "", 165000),
		},
		Viewer: core.Relation{Member: true},
	}
}

func pinnedRepo(owner, name, desc, lang string, stars int) core.Repo {
	return core.Repo{
		Ref: core.RepoRef{Owner: owner, Name: name}, Description: desc, Language: lang, Stars: stars,
		UpdatedAt: now.Add(-48 * time.Hour), URL: "https://github.com/" + owner + "/" + name,
	}
}

// outsider returns the organization as someone outside it sees it.
func outsider() core.Owner {
	o := org()
	o.Viewer.Member = false
	o.Members, o.Teams = 2, 0
	return o
}

// starred returns the repositories octocat starred, of several owners.
func starred() []core.Repo {
	return []core.Repo{
		pinnedRepo("charmbracelet", "bubbletea", "A powerful little TUI framework", "Go", 31000),
		pinnedRepo("cli", "cli", "GitHub's official command line tool", "Go", 38000),
		pinnedRepo("rust-lang", "rust", "Empowering everyone to build reliable and efficient software.", "Rust", 99000),
		pinnedRepo("github", "gitignore", "A collection of useful .gitignore templates", "", 165000),
	}
}

// people returns n accounts of kind, some with a name and a bio.
func people(kind core.OwnerKind, n int) []core.Person {
	out := make([]core.Person, n)
	for i := range out {
		out[i] = core.Person{Kind: kind, Login: fmt.Sprintf("person-%02d", i)}
		if i%3 != 2 {
			out[i].Name = fmt.Sprintf("Person Number %d", i)
		}
		if i%2 == 0 {
			out[i].Bio = fmt.Sprintf("Writes tools for the terminal, number %d", i)
		}
	}
	return out
}

func orgs() []core.Person {
	return []core.Person{
		{Kind: core.OwnerOrg, Login: "github", Name: "GitHub", Bio: "How people build software"},
		{Kind: core.OwnerOrg, Login: "charmbracelet", Name: "Charm", Bio: "We make the command line glamorous"},
	}
}

func members() []core.Person {
	return []core.Person{
		{Login: "mona", Name: "Mona Lisa", Role: core.MemberRoleAdmin},
		{Login: "hubot", Name: "Hubot", Role: core.MemberRoleMember},
		{Login: "defunkt", Name: "Chris Wanstrath", Role: core.MemberRoleMember},
		{Login: "mojombo", Name: "Tom Preston-Werner", Role: core.MemberRoleAdmin},
		{Login: "nobe4", Role: core.MemberRoleMember},
	}
}

func teams() []core.Team {
	return []core.Team{
		{Name: "Core", Slug: "core", Description: "The people who keep the lights on", Members: 12, URL: "https://github.com/orgs/github/teams/core"},
		{Name: "Security", Slug: "security", Description: "Reports and responses", Secret: true, Members: 4, URL: "https://github.com/orgs/github/teams/security"},
		{Name: "Docs", Slug: "docs", Members: 1200, URL: "https://github.com/orgs/github/teams/docs"},
	}
}

// repos returns n repositories of owner, most recently updated first.
func repos(owner string, n int) []core.Repo {
	langs := []string{"Go", "Rust", "TypeScript", ""}
	out := make([]core.Repo, n)
	for i := range out {
		out[i] = core.Repo{
			Ref:         core.RepoRef{Owner: owner, Name: fmt.Sprintf("repo-%03d", i)},
			Description: fmt.Sprintf("Repository number %d of %s", i, owner),
			Language:    langs[i%len(langs)],
			Stars:       i * 7,
			Private:     i%5 == 3,
			Fork:        i%7 == 4,
			UpdatedAt:   now.Add(-time.Duration(i) * 5 * time.Hour),
			URL:         "https://github.com/" + owner + fmt.Sprintf("/repo-%03d", i),
		}
	}
	return out
}

func (f *fakeService) CachedHeader(login string) (core.Owner, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.owners[strings.ToLower(login)]
	return o, ok && f.read["header "+strings.ToLower(login)]
}

func (f *fakeService) FreshHeader(login string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read["header "+strings.ToLower(login)]
}

func (f *fakeService) Header(_ context.Context, q owners.HeaderQuery) (core.Owner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	login := strings.ToLower(q.Login)
	f.calls = append(f.calls, "header "+login)
	if err := f.fail["header"]; err != nil {
		return core.Owner{}, err
	}
	o, ok := f.owners[login]
	if !ok {
		return core.Owner{}, core.ErrNotFound
	}
	o.Offline = f.offline
	f.read["header "+login] = true
	return o, nil
}

func (f *fakeService) FreshRepos(q owners.ReposQuery) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read["repos "+strings.ToLower(q.Owner)]
}

func (f *fakeService) page(q owners.ReposQuery) core.Page[core.Repo] {
	return pageOf(f, f.repos[strings.ToLower(q.Owner)], q.Cursor)
}

func (f *fakeService) Repos(_ context.Context, q owners.ReposQuery) (core.Page[core.Repo], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "repos "+q.Owner+" "+q.Cursor)
	if err := f.fail["repos"]; err != nil {
		return core.Page[core.Repo]{}, err
	}
	f.read["repos "+strings.ToLower(q.Owner)] = true
	return f.page(q), nil
}

func (f *fakeService) CachedAllRepos(q owners.ReposQuery, _ int) (core.Page[core.Repo], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return core.Page[core.Repo]{Items: f.repos[strings.ToLower(q.Owner)]}, f.read["repos "+strings.ToLower(q.Owner)]
}

func (f *fakeService) AllRepos(_ context.Context, q owners.ReposQuery, _ int) (core.Page[core.Repo], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "all "+q.Owner)
	if err := f.fail["repos"]; err != nil {
		return core.Page[core.Repo]{}, err
	}
	return core.Page[core.Repo]{Items: f.repos[strings.ToLower(q.Owner)]}, nil
}

func (f *fakeService) InvalidateLogin(login string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated = append(f.invalidated, login)
	login = strings.ToLower(login)
	for k := range f.read {
		if _, rest, _ := strings.Cut(k, " "); rest == login || strings.HasPrefix(rest, login+" ") {
			delete(f.read, k)
		}
	}
}

// pageOf returns the page of all at cursor, in pages of f.size.
func pageOf[T any](f *fakeService, all []T, cursor string) core.Page[T] {
	start, _ := strconv.Atoi(cursor)
	start = min(start, len(all))
	end := min(start+f.size, len(all))
	p := core.Page[T]{Items: all[start:end]}
	if end < len(all) {
		p.Next = strconv.Itoa(end)
	}
	return p
}

func (f *fakeService) FreshStars(q owners.StarsQuery) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read["stars "+strings.ToLower(q.Login)]
}

func (f *fakeService) Stars(_ context.Context, q owners.StarsQuery) (core.Page[core.Repo], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	login := strings.ToLower(q.Login)
	f.calls = append(f.calls, "stars "+login+" "+q.Cursor)
	if err := f.fail["stars"]; err != nil {
		return core.Page[core.Repo]{}, err
	}
	f.read["stars "+login] = true
	return pageOf(f, f.stars[login], q.Cursor), nil
}

func (f *fakeService) FreshPeople(q owners.PeopleQuery) bool {
	if sponsorList(q.List) {
		return f.sideFake.FreshPeople(q)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read["people "+strings.ToLower(q.Login)+" "+q.List.String()]
}

func (f *fakeService) People(ctx context.Context, q owners.PeopleQuery) (core.Page[core.Person], error) {
	if sponsorList(q.List) {
		// The README's sponsors are the side's.
		return f.sideFake.People(ctx, q)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	k := strings.ToLower(q.Login) + " " + q.List.String()
	f.calls = append(f.calls, "people "+k+" "+q.Cursor)
	if err := f.fail["people"]; err != nil {
		return core.Page[core.Person]{}, err
	}
	f.read["people "+k] = true
	return pageOf(f, f.people[k], q.Cursor), nil
}

func (f *fakeService) FreshTeams(q owners.TeamsQuery) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read["teams "+strings.ToLower(q.Login)]
}

func (f *fakeService) Teams(_ context.Context, q owners.TeamsQuery) (core.Page[core.Team], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	login := strings.ToLower(q.Login)
	f.calls = append(f.calls, "teams "+login+" "+q.Cursor)
	if err := f.fail["teams"]; err != nil {
		return core.Page[core.Team]{}, err
	}
	f.read["teams "+login] = true
	return pageOf(f, f.teams[login], q.Cursor), nil
}

// newSection returns a page of width by height, focused and started, on
// the account login.
func newSection(tb testing.TB, svc Service, login string, width, height int, opts ...Option) *Section {
	tb.Helper()
	opts = append([]Option{WithNow(func() time.Time { return now })}, opts...)
	s := New(tb.Context(), svc, config.Default().Keys, opts...)
	p, err := config.Default().Palette(true)
	if err != nil {
		tb.Fatal(err)
	}
	s.SetTheme(ui.NewTheme(p, true))
	s.SetSize(width, height)
	run(tb, s, s.Init())
	if login != "" {
		run(tb, s, s.Update(ui.OwnerMsg{Login: login}))
	}
	s.Focus()
	return s
}

// run executes cmd and gives every resulting message to s, the way the app
// would, until no commands are left, and returns the messages meant for
// the app. Spinner ticks are dropped, so tests never sleep.
func run(tb testing.TB, s *Section, cmd tea.Cmd) []tea.Msg {
	tb.Helper()
	var app []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, spinner.TickMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case ui.OpenMsg, ui.NotifyMsg, ui.RepoMsg, ui.OwnerMsg:
			app = append(app, msg)
		default:
			queue = append(queue, s.Update(msg))
		}
	}
	return app
}

// press presses each key and runs the resulting commands.
func press(tb testing.TB, s *Section, keys ...string) []tea.Msg {
	tb.Helper()
	var app []tea.Msg //nolint:prealloc // Most keys send nothing to the app.
	for _, k := range keys {
		app = append(app, run(tb, s, s.Update(keyPress(k)))...)
	}
	return app
}

func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}
