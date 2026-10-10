package notifications

import (
	"context"
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
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
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
	// fail makes every op fail, and listErr every List.
	fail    error
	listErr error

	lists []notifications.ListQuery
	// served holds the pages listed of other inboxes than the default,
	// which CachedList returns.
	served  map[notifications.ListQuery]core.Page[core.Notification]
	reads   []string
	dones   []string
	allRead int
	// until is what the last mark of all read marked until.
	until time.Time
	// invalidated holds, for each call of Invalidate, how many lists were
	// made before it.
	invalidated []int
}

func newFake(threads ...core.Notification) *fakeService {
	return &fakeService{threads: threads, size: 30}
}

func (f *fakeService) CachedList(q notifications.ListQuery) (core.Page[core.Notification], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if q != (notifications.ListQuery{}) {
		p, ok := f.served[q]
		return p, ok
	}
	return f.cached, f.isCached
}

func (f *fakeService) List(_ context.Context, q notifications.ListQuery) (core.Page[core.Notification], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Again doesn't select another page.
	q.Again = false
	f.lists = append(f.lists, q)
	if f.listErr != nil {
		return core.Page[core.Notification]{}, f.listErr
	}
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
	if q != (notifications.ListQuery{}) {
		if f.served == nil {
			f.served = map[notifications.ListQuery]core.Page[core.Notification]{}
		}
		f.served[q] = p
	}
	return p, nil
}

func (f *fakeService) MarkRead(id string) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, id)
	return f.changeThread(id, func(ts []core.Notification) []core.Notification {
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
	return f.changeThread(id, func(ts []core.Notification) []core.Notification {
		return slices.DeleteFunc(ts, func(n core.Notification) bool { return n.ID == id })
	})
}

// changeThread is change for the change of one thread, whose rollback
// puts back that thread alone, so that changes of other threads in flight
// at once keep theirs, as the service's cache does.
func (f *fakeService) changeThread(id string, edit func([]core.Notification) []core.Notification) *optimistic.Op {
	order := make([]string, len(f.threads))
	var was core.Notification
	for i := range f.threads {
		order[i] = f.threads[i].ID
		if order[i] == id {
			was = f.threads[i]
		}
	}
	f.threads = edit(slices.Clone(f.threads))
	fail := f.fail
	return optimistic.New(func(context.Context) error { return fail }, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		ts := slices.DeleteFunc(slices.Clone(f.threads), func(n core.Notification) bool { return n.ID == id })
		ts = append(ts, was)
		slices.SortStableFunc(ts, func(a, b core.Notification) int {
			return slices.Index(order, a.ID) - slices.Index(order, b.ID)
		})
		f.threads = ts
	})
}

func (f *fakeService) MarkAllRead(until time.Time) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allRead++
	f.until = until
	return f.change(func(ts []core.Notification) []core.Notification {
		for i := range ts {
			if !ts[i].UpdatedAt.After(until) {
				ts[i].Unread = false
			}
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

func (f *fakeService) Invalidate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated = append(f.invalidated, len(f.lists))
}

func (f *fakeService) invalidations() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.invalidated)
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
// now. Its subject is numbered by id: the issue or pull request, the
// release, or the commit.
func thread(id, repo string, typ core.SubjectType, title, reason string, unread bool, ago time.Duration) core.Notification {
	r, _ := core.ParseRepoRef(repo)
	sub := core.Subject{Title: title, Type: typ, WebURL: "https://github.com/" + repo + "/" + id}
	n, _ := strconv.Atoi(id)
	switch typ {
	case core.SubjectIssue, core.SubjectPullRequest:
		sub.Number = n
	case core.SubjectRelease:
		sub.ReleaseID = int64(n)
	case core.SubjectCommit:
		sub.SHA = "c0ffee" + id
	default:
	}
	return core.Notification{
		ID:        id,
		Repo:      r,
		Subject:   sub,
		Reason:    reason,
		Unread:    unread,
		UpdatedAt: now.Add(-ago),
	}
}

// opened describes what msg opens, such as "pull o/r#1" or a URL, and
// reports whether it opens anything.
func opened(msg tea.Msg) (string, bool) {
	switch msg := msg.(type) {
	case ui.OpenMsg:
		return msg.URL, true
	case ui.OpenPullMsg:
		return "pull " + msg.Repo.String() + "#" + strconv.Itoa(msg.Number), true
	case ui.OpenIssueMsg:
		return "issue " + msg.Repo.String() + "#" + strconv.Itoa(msg.Number), true
	case ui.OpenReleaseMsg:
		return "release " + msg.Repo.String() + " " + strconv.FormatInt(msg.ID, 10), true
	case ui.OpenCommitMsg:
		return "commit " + msg.Repo.String() + "@" + msg.SHA, true
	case ui.OpenActionsMsg:
		return "runs " + msg.Repo.String() + " " + msg.Filter.Branch, true
	}
	return "", false
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
		case ui.OpenMsg, ui.NotifyMsg, ui.OpenActionsMsg, ui.OpenPullMsg, ui.OpenIssueMsg, ui.OpenReleaseMsg, ui.OpenCommitMsg, ui.OpenFilterMsg:
			app = append(app, msg)
		case ui.OpenModalMsg:
			// The app opens the question over the section, and gives it
			// the keys until it closes.
			if m, ok := msg.Modal.(*ui.ConfirmModal); ok {
				asking[s] = m
			}
			app = append(app, msg)
		case ui.CloseModalMsg:
			if asking[s] == msg.Modal {
				delete(asking, s)
			}
		case ui.DoneMsg:
			app = append(app, msg)
			queue = append(queue, s.Update(msg))
		case ui.BulkDoneMsg:
			app = append(app, msg)
			queue = append(queue, s.Update(msg))
		default:
			queue = append(queue, s.Update(msg))
		}
	}
	return app
}

// asking holds the question each section opened, until it closes.
var asking = map[*Section]*ui.ConfirmModal{}

// question returns what the question open over s asks, or "".
func question(s *Section) string {
	if m, ok := asking[s]; ok {
		return m.Question()
	}
	return ""
}

// showAll stands for applying the filter of every thread, read or not,
// among the keys of press, as the filter modal of the app would.
const showAll = "filter:"

// press presses each key and runs the resulting commands. A key that
// starts with showAll applies the filter of the query after it.
// readAll stands in press for the read all command, which has no key.
const readAll = "read all"

func press(tb testing.TB, s *Section, keys ...string) []tea.Msg {
	tb.Helper()
	var app []tea.Msg
	for _, k := range keys {
		if q, ok := strings.CutPrefix(k, showAll); ok {
			app = append(app, run(tb, s, s.ApplyFilter(filterform.AppliedMsg{Query: q}))...)
			continue
		}
		if m, ok := asking[s]; ok {
			if k == readAll {
				// No key does it, and the question that is open takes
				// no other command.
				continue
			}
			app = append(app, run(tb, s, m.Update(keyPress(k)))...)
			continue
		}
		if k == readAll {
			app = append(app, run(tb, s, s.Update(ui.MarkAllReadMsg{}))...)
			continue
		}
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
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// errMark is the error glyph of the default icons, which mark what failed.
var errMark = ui.NewIcons(config.IconsNerd).Error
