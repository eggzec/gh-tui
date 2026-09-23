package notifications

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ Service = (*notifications.Service)(nil)

// now is the clock of every test, so ages are stable.
var now = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

// fakeService serves threads in pages of size, with the start index as
// cursor, and changes them the way the real service changes its cache: at
// once, rolled back if the op fails.
type fakeService struct {
	mu      sync.Mutex
	threads []core.Notification
	size    int
	// cached is what CachedList returns for the default inbox.
	cached   core.Page[core.Notification]
	isCached bool
	// fail makes every op fail.
	fail error

	lists   []notifications.ListQuery
	reads   []string
	dones   []string
	allRead int
}

func newFake(threads ...core.Notification) *fakeService {
	return &fakeService{threads: threads, size: 30}
}

func (f *fakeService) CachedList(q notifications.ListQuery) (core.Page[core.Notification], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if q != (notifications.ListQuery{}) {
		return core.Page[core.Notification]{}, false
	}
	return f.cached, f.isCached
}

func (f *fakeService) List(_ context.Context, q notifications.ListQuery) (core.Page[core.Notification], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, q)
	var shown []core.Notification
	for i := range f.threads {
		if q.Filter.All || f.threads[i].Unread {
			shown = append(shown, f.threads[i])
		}
	}
	start, _ := strconv.Atoi(q.Cursor)
	start = min(start, len(shown))
	end := min(start+f.size, len(shown))
	p := core.Page[core.Notification]{Items: slices.Clone(shown[start:end])}
	if end < len(shown) {
		p.Next = strconv.Itoa(end)
	}
	return p, nil
}

func (f *fakeService) MarkRead(id string) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, id)
	return f.change(func(ts []core.Notification) []core.Notification {
		for i := range ts {
			if ts[i].ID == id {
				ts[i].Unread = false
			}
		}
		return ts
	})
}

func (f *fakeService) MarkDone(id string) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dones = append(f.dones, id)
	return f.change(func(ts []core.Notification) []core.Notification {
		return slices.DeleteFunc(ts, func(n core.Notification) bool { return n.ID == id })
	})
}

func (f *fakeService) MarkAllRead() *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allRead++
	return f.change(func(ts []core.Notification) []core.Notification {
		for i := range ts {
			ts[i].Unread = false
		}
		return ts
	})
}

// change applies edit to a copy of the threads now, and returns the op
// that sends it. Call it with f.mu held.
func (f *fakeService) change(edit func([]core.Notification) []core.Notification) *optimistic.Op {
	before := f.threads
	f.threads = edit(slices.Clone(before))
	fail := f.fail
	return optimistic.New(func(context.Context) error { return fail }, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.threads = before
	})
}

func (f *fakeService) listCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.lists)
}

func (f *fakeService) lastList() notifications.ListQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists[len(f.lists)-1]
}

// thread returns a notification about subject type typ, updated ago before
// now.
func thread(id, repo string, typ core.SubjectType, title, reason string, unread bool, ago time.Duration) core.Notification {
	r, _ := core.ParseRepoRef(repo)
	return core.Notification{
		ID:        id,
		Repo:      r,
		Subject:   core.Subject{Title: title, Type: typ, WebURL: "https://github.com/" + repo + "/" + id},
		Reason:    reason,
		Unread:    unread,
		UpdatedAt: now.Add(-ago),
	}
}

// inbox is a varied inbox, read and unread.
func inbox() []core.Notification {
	return []core.Notification{
		thread("1", "charmbracelet/bubbletea", core.SubjectPullRequest, "Add a renderer that only redraws changed cells", "review_requested", true, 4*time.Minute),
		thread("2", "eggzec/gh-tui", core.SubjectIssue, "Notifications badge is off by one after mark all read", "mention", true, 3*time.Hour),
		thread("3", "golang/go", core.SubjectRelease, "go1.27.1", "subscribed", true, 26*time.Hour),
		thread("4", "cli/cli", core.SubjectDiscussion, "RFC: extensions that render their own TUI", "comment", false, 5*24*time.Hour),
		thread("5", "charmbracelet/lipgloss", core.SubjectCommit, "Fix width of wide runes in tables", "author", false, 40*24*time.Hour),
		thread("6", "a-very-long-organization-name/with-an-even-longer-repository-name", "CheckSuite", "CI run failed on main", "ci_activity", true, 30*time.Second),
		thread("7", "eggzec/gh-tui", "RepositoryVulnerabilityAlert", "Moderate severity vulnerability in golang.org/x/net", "security_alert", false, 400*24*time.Hour),
	}
}

func newSection(tb testing.TB, svc Service, width, height int) *Section {
	tb.Helper()
	ctx := tb.Context()
	s := New(ctx, svc, config.Default().Keys, WithNow(func() time.Time { return now }))
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
// would, until no commands are left. It returns the messages meant for the
// app. Spinner ticks are dropped so tests never sleep.
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
		case ui.OpenMsg, ui.NotifyMsg:
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
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}
