package dashboard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var (
	_ Service = (*dashboard.Service)(nil)
	_ Inbox   = (*notifications.Service)(nil)
)

// now is the clock of every test, so ages are stable.
var now = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

// fakeService serves a fixed dashboard. Repositories come in pages of
// size, with the start index as cursor, and a page counts as cached once
// it was read.
type fakeService struct {
	mu      sync.Mutex
	header  core.Header
	work    core.Work
	contrib core.Contributions
	repos   map[string][]core.Repo
	size    int

	// cached makes the Cached reads of the header, work and contributions
	// hit before any read.
	cached bool
	// stale makes the first read of the header return it marked stale.
	stale bool
	// offline marks every read served offline, and limited served
	// because of a rate limit.
	offline, limited bool
	// fail, when set, fails the read of that kind.
	fail map[string]error

	read map[string]bool
	// expired holds what went past its TTL since it was read: header,
	// work, contributions, and the pages of repositories by pageKey.
	expired     map[string]bool
	calls       []string
	invalidated int
}

func newFake() *fakeService {
	return &fakeService{
		header:  header(),
		work:    work(),
		contrib: contributions(),
		repos: map[string][]core.Repo{
			"@me":           repos("octocat", 250),
			"github":        repos("github", 5),
			"charmbracelet": repos("charmbracelet", 3),
		},
		size:    100,
		fail:    map[string]error{},
		read:    map[string]bool{},
		expired: map[string]bool{},
	}
}

func (f *fakeService) call(what string) error {
	f.calls = append(f.calls, what)
	return f.fail[what]
}

// fresh reports whether what was read and hasn't expired since.
func (f *fakeService) fresh(what string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read[what] && !f.expired[what]
}

// took records that what was read, fresh.
func (f *fakeService) took(what string) {
	f.read[what] = true
	delete(f.expired, what)
}

func (f *fakeService) FreshHeader() bool                      { return f.fresh("header") }
func (f *fakeService) FreshWork(dashboard.WorkQuery) bool     { return f.fresh("work") }
func (f *fakeService) FreshContributions() bool               { return f.fresh("contributions") }
func (f *fakeService) FreshRepos(q dashboard.ReposQuery) bool { return f.fresh(pageKey(q)) }

func (f *fakeService) CachedHeader() (core.Header, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.header, f.cached || f.read["header"]
}

func (f *fakeService) Header(_ context.Context, q dashboard.HeaderQuery) (core.Header, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("header"); err != nil {
		return core.Header{}, err
	}
	h := f.header
	// A kept header is served stale until a read with Again set.
	h.Stale = f.stale && !q.Again
	h.Offline, h.Limited = f.offline, f.limited
	f.took("header")
	return h, nil
}

func (f *fakeService) CachedWork(dashboard.WorkQuery) (core.Work, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.work, f.cached || f.read["work"]
}

func (f *fakeService) Work(context.Context, dashboard.WorkQuery) (core.Work, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("work"); err != nil {
		return core.Work{}, err
	}
	f.took("work")
	w := f.work
	w.Offline, w.Limited = f.offline, f.limited
	return w, nil
}

func (f *fakeService) CachedContributions() (core.Contributions, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.contrib, f.cached || f.read["contributions"]
}

func (f *fakeService) Contributions(context.Context, dashboard.ContributionsQuery) (core.Contributions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("contributions"); err != nil {
		return core.Contributions{}, err
	}
	f.took("contributions")
	return f.contrib, nil
}

func ownerKey(q dashboard.ReposQuery) string {
	if q.Viewer {
		return "@me"
	}
	return strings.ToLower(q.Owner)
}

func (f *fakeService) page(q dashboard.ReposQuery) core.Page[core.Repo] {
	all := f.repos[ownerKey(q)]
	start, _ := strconv.Atoi(q.Cursor)
	start = min(start, len(all))
	end := min(start+f.size, len(all))
	p := core.Page[core.Repo]{Items: slices.Clone(all[start:end])}
	if end < len(all) {
		p.Next = strconv.Itoa(end)
	}
	return p
}

func pageKey(q dashboard.ReposQuery) string { return ownerKey(q) + "@" + q.Cursor }

func (f *fakeService) CachedRepos(q dashboard.ReposQuery) (core.Page[core.Repo], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.page(q), f.read[pageKey(q)]
}

func (f *fakeService) Repos(_ context.Context, q dashboard.ReposQuery) (core.Page[core.Repo], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("repos " + pageKey(q)); err != nil {
		return core.Page[core.Repo]{}, err
	}
	f.took(pageKey(q))
	return f.page(q), nil
}

func (f *fakeService) CachedAllRepos(q dashboard.ReposQuery, limit int) (core.Page[core.Repo], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all core.Page[core.Repo]
	all.Next = q.Cursor
	found := false
	for len(all.Items) < limit {
		q.Cursor = all.Next
		if !f.read[pageKey(q)] {
			break
		}
		found = true
		p := f.page(q)
		all.Items = append(all.Items, p.Items...)
		all.Next = p.Next
		if p.Last() {
			break
		}
	}
	return all, found
}

// AllRepos serves the pages read before from the cache, and reads the
// others, as the service does.
func (f *fakeService) AllRepos(_ context.Context, q dashboard.ReposQuery, limit int) (core.Page[core.Repo], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit <= 0 || limit > dashboard.MaxOwnerRepos {
		limit = dashboard.MaxOwnerRepos
	}
	var all core.Page[core.Repo]
	all.Next = q.Cursor
	for len(all.Items) < limit {
		q.Cursor = all.Next
		if !f.read[pageKey(q)] {
			if err := f.call("repos " + pageKey(q)); err != nil {
				return core.Page[core.Repo]{}, err
			}
			f.took(pageKey(q))
		}
		p := f.page(q)
		all.Items = append(all.Items, p.Items...)
		all.Next = p.Next
		if p.Last() {
			break
		}
	}
	return all, nil
}

func (f *fakeService) Invalidate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated++
	clear(f.read)
}

// count returns how many reads of what were made.
func (f *fakeService) count(what string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == what {
			n++
		}
	}
	return n
}

// fakeInbox serves the first page of the inbox, and caches it once read.
// The notifications service marks the threads the pane opens read.
var _ interface {
	Inbox
	Marker
} = (*notifications.Service)(nil)

type fakeInbox struct {
	mu      sync.Mutex
	threads []core.Notification
	read    bool
	lists   int
	// marked are the threads marked read.
	marked []string
	// err, when set, fails every list.
	err error
}

// MarkRead marks thread id read at once, as the service does in its
// cache, and never fails.
func (f *fakeInbox) MarkRead(id string) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, id)
	ts := slices.Clone(f.threads)
	for i := range ts {
		if ts[i].ID == id {
			ts[i].Unread = false
		}
	}
	f.threads = ts
	return optimistic.New(func(context.Context) error { return nil }, func() {})
}

func (f *fakeInbox) marks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.marked)
}

func (f *fakeInbox) CachedList(q notifications.ListQuery) (core.Page[core.Notification], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if q != (notifications.ListQuery{}) {
		return core.Page[core.Notification]{}, false
	}
	return core.Page[core.Notification]{Items: slices.Clone(f.threads)}, f.read
}

func (f *fakeInbox) List(context.Context, notifications.ListQuery) (core.Page[core.Notification], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = true
	f.lists++
	if f.err != nil {
		return core.Page[core.Notification]{}, f.err
	}
	return core.Page[core.Notification]{Items: slices.Clone(f.threads)}, nil
}

func (f *fakeInbox) set(threads ...core.Notification) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.threads = threads
}

var here = core.RepoRef{Owner: "octocat", Name: "hello-world"}

func header() core.Header {
	return core.Header{
		Profile: core.Profile{
			Login: "octocat", Name: "Mona Lisa Octocat", Bio: "Building tools for the terminal",
			Company: "@github", Location: "San Francisco", Followers: 1234, Following: 56,
			Status: core.Status{Emoji: ":ship:", Message: "Shipping the dashboard"},
			URL:    "https://github.com/octocat",
		},
		Pinned: []core.Repo{
			pinnedRepo("octocat", "hello-world", "My first repository on GitHub, with a description long enough to wrap onto a second line", "Go", 2345),
			pinnedRepo("octocat", "spoon-knife", "This repo is for demonstration purposes only.", "HTML", 12800),
			pinnedRepo("charmbracelet", "bubbletea", "A powerful little TUI framework", "Go", 31000),
			pinnedRepo("cli", "cli", "GitHub's official command line tool", "Go", 38000),
			pinnedRepo("octocat", "dotfiles", "", "Shell", 3),
		},
		Orgs: []core.Org{{Login: "github", Name: "GitHub"}, {Login: "charmbracelet", Name: "Charm"}},
	}
}

func pinnedRepo(owner, name, desc, lang string, stars int) core.Repo {
	return core.Repo{
		Ref: core.RepoRef{Owner: owner, Name: name}, Description: desc, Language: lang, Stars: stars,
		UpdatedAt: now.Add(-48 * time.Hour), URL: "https://github.com/" + owner + "/" + name,
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
			Archived:    i%11 == 10,
			UpdatedAt:   now.Add(-time.Duration(i) * 5 * time.Hour),
			URL:         "https://github.com/" + owner + fmt.Sprintf("/repo-%03d", i),
		}
	}
	return out
}

func hit(kind core.SearchKind, repo string, number int, title string, draft bool, ago time.Duration) core.SearchHit {
	r, _ := core.ParseRepoRef(repo)
	return core.SearchHit{Kind: kind, Draft: draft, Issue: core.Issue{
		Repo: r, Number: number, Title: title, State: core.StateOpen, UpdatedAt: now.Add(-ago),
		URL: fmt.Sprintf("https://github.com/%s/issues/%d", repo, number),
	}}
}

func work() core.Work {
	return core.Work{
		ReviewRequested: core.WorkList{Count: 3, Items: []core.SearchHit{
			hit(core.SearchPulls, "charmbracelet/bubbletea", 1402, "Render only the cells that changed", false, 2*time.Hour),
			hit(core.SearchPulls, "cli/cli", 9001, "Add an extension that opens a dashboard", false, 30*time.Hour),
		}},
		Authored: core.WorkList{Count: 2, Items: []core.SearchHit{
			hit(core.SearchPulls, "octocat/hello-world", 12, "Show the dashboard at start-up", true, 20*time.Minute),
			hit(core.SearchPulls, "octocat/spoon-knife", 3, "Fix the typo in the README", false, 6*24*time.Hour),
		}},
		Assigned: core.WorkList{Count: 1, Items: []core.SearchHit{
			hit(core.SearchIssues, "octocat/hello-world", 40, "The calendar draws two cells per day in some terminals", false, 3*24*time.Hour),
		}},
	}
}

// contributions returns a year of weeks, from Sunday, with counts that
// repeat so the levels vary.
func contributions() core.Contributions {
	start := time.Date(2025, 9, 21, 0, 0, 0, 0, time.UTC)
	var c core.Contributions
	for w := range 53 {
		var week []core.ContributionDay
		for d := range 7 {
			day := start.AddDate(0, 0, w*7+d)
			if day.After(now) {
				break
			}
			n := (w*3 + d*5) % 9
			if d == 0 || d == 6 {
				n /= 3
			}
			week = append(week, core.ContributionDay{Date: day, Count: n, Level: min(n/2, 4)})
			c.Total += n
		}
		c.Weeks = append(c.Weeks, week)
	}
	return c
}

// thread returns a notification of pull request id of repo.
func thread(id, repo, title string, unread bool, ago time.Duration) core.Notification {
	r, _ := core.ParseRepoRef(repo)
	n, _ := strconv.Atoi(id)
	return core.Notification{
		ID: id, Repo: r, Unread: unread, UpdatedAt: now.Add(-ago),
		Subject: core.Subject{Title: title, Type: core.SubjectPullRequest, Number: n, WebURL: "https://github.com/" + repo + "/pull/" + id},
	}
}

func inboxThreads() []core.Notification {
	return []core.Notification{
		thread("1", "charmbracelet/bubbletea", "Render only the cells that changed", true, 5*time.Minute),
		thread("2", "octocat/hello-world", "Dashboard: show the contribution calendar", true, 3*time.Hour),
		thread("3", "cli/cli", "v2.80.0", true, 26*time.Hour),
		thread("4", "golang/go", "spec: range over func", false, 50*time.Hour),
	}
}

// newSection returns a started, focused dashboard of width by height over
// svc and in, with the repository of the current directory.
func newSection(tb testing.TB, svc Service, in Inbox, width, height int, opts ...Option) *Section {
	tb.Helper()
	opts = append([]Option{
		WithNow(func() time.Time { return now }),
		WithHere(here, nil),
	}, opts...)
	if in != nil {
		opts = append(opts, WithInbox(in))
	}
	s := New(tb.Context(), svc, config.Default().Keys, opts...)
	p, err := config.Default().Palette(true)
	if err != nil {
		tb.Fatal(err)
	}
	s.SetTheme(ui.NewTheme(p, true))
	s.SetSize(width, height)
	s.Focus()
	run(tb, s, s.Init())
	return s
}

// run executes cmd and gives every resulting message to s, the way the app
// would, until no commands are left, and returns the messages meant for
// the app. Spinner ticks are dropped, and so are commands that don't
// return at once, such as the blink of a cursor, so tests never sleep.
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
		case ui.OpenMsg, ui.NotifyMsg, ui.RepoMsg, ui.OpenPullMsg, ui.OpenIssueMsg, ui.ShowMsg,
			ui.OpenReleaseMsg, ui.OpenCommitMsg, ui.OpenActionsMsg:
			app = append(app, msg)
		case ui.DoneMsg:
			app = append(app, msg)
			queue = append(queue, s.Update(msg))
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
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// logVoice returns the voice of the default keys, pointing to the log
// file where the app keeps it, so that the view shows it under ~.
func logVoice(tb testing.TB) ui.Voice {
	tb.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		tb.Skip("no home directory:", err)
	}
	return ui.NewVoice(config.Default().Keys, filepath.Join(home, ".local", "state", "gh-tui", "gh-tui.log"))
}
