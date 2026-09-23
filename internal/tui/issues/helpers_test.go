package issues

import (
	"context"
	"errors"
	"fmt"
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
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ Service = (*issuesvc.Service)(nil)

var (
	testRepo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	testNow  = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
)

// fakeService serves issues in pages of pageSize from memory and records
// what it was asked. Like the service, it keeps the pages it served and
// changes issues in them at once, until a confirmed change or an edit on
// the "server" makes it read them again.
type fakeService struct {
	mu       sync.Mutex
	issues   []core.Issue
	pages    map[issuesvc.ListQuery]core.Page[core.Issue]
	pageSize int
	lists    []issuesvc.ListQuery
	listErr  error

	// cached holds the issues Get read, as the service's cache would.
	cached   map[int]core.Issue
	gets     []int
	getErr   error
	comments map[int][]core.Comment
	// commentQueries are the comment pages asked for.
	commentQueries []issuesvc.CommentsQuery
	// changes are the state changes asked for, such as "close 999", and
	// sendErr fails sending them.
	changes []string
	sendErr error
	// gate, if set, holds sending until it is closed.
	gate chan struct{}
	// invalidated are the calls of Invalidate.
	invalidated []invalidation
}

// invalidation is a call of Invalidate, with how many lists and gets were
// made before it.
type invalidation struct {
	repo        core.RepoRef
	lists, gets int
}

func (f *fakeService) Close(repo core.RepoRef, number int) *optimistic.Op {
	return f.setState(repo, number, core.StateClosed)
}

func (f *fakeService) Reopen(repo core.RepoRef, number int) *optimistic.Op {
	return f.setState(repo, number, core.StateOpen)
}

// setState changes the issue at once, as the service's cache would, and
// rolls it back if sending fails.
func (f *fakeService) setState(_ core.RepoRef, number int, state core.State) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	verb := "close "
	if state == core.StateOpen {
		verb = "reopen "
	}
	f.changes = append(f.changes, verb+strconv.Itoa(number))
	var prev core.State
	apply := func(st core.State) {
		for i := range f.issues {
			if f.issues[i].Number == number {
				prev, f.issues[i].State = f.issues[i].State, st
			}
		}
		if it, ok := f.cached[number]; ok {
			it.State = st
			f.cached[number] = it
		}
		for q, p := range f.pages {
			for i := range p.Items {
				if p.Items[i].Number == number {
					p.Items[i].State = st
				}
			}
			f.pages[q] = p
		}
	}
	apply(state)
	was, gate := prev, f.gate
	return optimistic.New(func(ctx context.Context) error {
		if gate != nil {
			select {
			case <-gate:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.sendErr == nil {
			clear(f.pages)
		}
		return f.sendErr
	}, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		apply(was)
	})
}

func (f *fakeService) changeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.changes)
}

func newFakeService(issues []core.Issue) *fakeService {
	return &fakeService{
		issues:   issues,
		pages:    map[issuesvc.ListQuery]core.Page[core.Issue]{},
		pageSize: 30,
		cached:   map[int]core.Issue{},
		comments: map[int][]core.Comment{},
	}
}

func (f *fakeService) Get(_ context.Context, repo core.RepoRef, number int) (core.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, number)
	if f.getErr != nil {
		return core.Issue{}, f.getErr
	}
	for i := range f.issues {
		if it := f.issues[i]; it.Repo == repo && it.Number == number {
			f.cached[number] = it
			return it, nil
		}
	}
	return core.Issue{}, core.ErrNotFound
}

func (f *fakeService) CachedGet(_ core.RepoRef, number int) (core.Issue, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it, ok := f.cached[number]
	return it, ok
}

func (f *fakeService) Comments(_ context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commentQueries = append(f.commentQueries, q)
	all := f.comments[q.Number]
	start, _ := strconv.Atoi(q.Cursor)
	start = min(start, len(all))
	end := min(start+f.pageSize, len(all))
	next := ""
	if end < len(all) {
		next = strconv.Itoa(end)
	}
	return core.Page[core.Comment]{Items: slices.Clone(all[start:end]), Next: next}, nil
}

func (f *fakeService) getCalls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.gets)
}

func (f *fakeService) addComments(number int, cs ...core.Comment) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments[number] = append(f.comments[number], cs...)
}

func (f *fakeService) List(_ context.Context, q issuesvc.ListQuery) (core.Page[core.Issue], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, q)
	if f.listErr != nil {
		return core.Page[core.Issue]{}, f.listErr
	}
	if q.Repo != testRepo {
		return core.Page[core.Issue]{}, errors.New("unknown repo " + q.Repo.String())
	}
	if p, ok := f.pages[q]; ok {
		return core.Page[core.Issue]{Items: slices.Clone(p.Items), Next: p.Next}, nil
	}
	var match []core.Issue
	for i := range f.issues {
		it := &f.issues[i]
		if q.State == core.FilterAll || string(it.State) == string(q.State) || (q.State == "" && it.State == core.StateOpen) {
			match = append(match, *it)
		}
	}
	start, _ := strconv.Atoi(q.Cursor)
	end := min(start+f.pageSize, len(match))
	if start > end {
		start = end
	}
	next := ""
	if end < len(match) {
		next = strconv.Itoa(end)
	}
	p := core.Page[core.Issue]{Items: slices.Clone(match[start:end]), Next: next}
	f.pages[q] = p
	return core.Page[core.Issue]{Items: slices.Clone(p.Items), Next: next}, nil
}

func (f *fakeService) Invalidate(repo core.RepoRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated = append(f.invalidated, invalidation{repo, len(f.lists), len(f.gets)})
}

func (f *fakeService) invalidations() []invalidation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.invalidated)
}

func (f *fakeService) listCalls() []issuesvc.ListQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.lists)
}

func (f *fakeService) set(number int, fn func(*core.Issue)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.issues {
		if f.issues[i].Number == number {
			fn(&f.issues[i])
		}
	}
	clear(f.pages)
}

var (
	titles = []string{
		"Crash when the config file is empty",
		"Support GitHub Enterprise hosts in the repository picker",
		"Notifications tab keeps polling while the terminal is unfocused",
		"Add a keybinding to copy the issue URL",
		"Markdown tables render with broken borders at narrow widths",
		"Rate limit toast shows the wrong reset time",
		"Docs: explain how themes pick light or dark",
		"Scrolling a long thread stutters on slow terminals",
	}
	labelSets = [][]core.Label{
		{{Name: "bug", Color: "d73a4a"}},
		{{Name: "enhancement", Color: "a2eeef"}, {Name: "help wanted", Color: "008672"}},
		{},
		{{Name: "good first issue", Color: "7057ff"}},
		{{Name: "bug", Color: "d73a4a"}, {Name: "ui", Color: "fbca04"}, {Name: "regression", Color: "b60205"}},
		{{Name: "question", Color: "not-a-color"}},
		{{Name: "documentation", Color: "0075ca"}},
		{{Name: "performance", Color: "e99695"}, {Name: "thread", Color: "c5def5"}},
	}
	authors = []string{"octocat", "hubot", "mona", "defunkt-the-longest", "pjhyett", "mojombo"}
)

// sampleIssues returns n realistic issues, newest first, every fifth one
// closed.
func sampleIssues(n int) []core.Issue {
	out := make([]core.Issue, 0, n)
	for i := range n {
		num := 1000 - i
		state := core.StateOpen
		if i%5 == 4 {
			state = core.StateClosed
		}
		out = append(out, core.Issue{
			ID:        "I_" + strconv.Itoa(num),
			Repo:      testRepo,
			Number:    num,
			Title:     titles[i%len(titles)],
			Body:      "Steps to reproduce:\n\n1. Open gh-tui\n2. Look at issue " + strconv.Itoa(num),
			State:     state,
			Author:    core.User{Login: authors[i%len(authors)]},
			Labels:    labelSets[i%len(labelSets)],
			Assignees: []core.User{{Login: authors[(i+1)%len(authors)]}},
			Comments:  (i * 7) % 23,
			CreatedAt: testNow.Add(-time.Duration(i+3) * 24 * time.Hour),
			UpdatedAt: testNow.Add(-time.Duration(i*i+1) * 37 * time.Minute),
			URL:       fmt.Sprintf("https://github.com/eggzec/gh-tui/issues/%d", num),
		})
	}
	return out
}

// sampleComments returns n comments, oldest first.
func sampleComments(n int) []core.Comment {
	bodies := []string{
		"I can reproduce this on **v0.3** with an empty file.",
		"Looks like `config.Load` returns early. A fix:\n\n```go\nif len(data) == 0 {\n\treturn Default(), nil\n}\n```",
		"Thanks! Fixed on main.",
	}
	out := make([]core.Comment, 0, n)
	for i := range n {
		out = append(out, core.Comment{
			ID:        "IC_" + strconv.Itoa(i),
			Author:    core.User{Login: authors[(i+2)%len(authors)]},
			Body:      bodies[i%len(bodies)],
			CreatedAt: testNow.Add(-time.Duration(n-i) * 5 * time.Hour),
		})
	}
	return out
}

func testTheme() ui.Theme {
	p, _ := config.Default().Palette(true)
	return ui.NewTheme(p, true)
}

// newSection returns a focused section at width×height over svc, with the
// default keys and a pinned clock. It is not started.
func newSection(tb testing.TB, svc Service, width, height int) *Section {
	tb.Helper()
	s := New(tb.Context(), svc, config.Default().Keys, WithNow(func() time.Time { return testNow }))
	s.SetTheme(testTheme())
	s.SetSize(width, height)
	s.Focus()
	return s
}

// started returns a section that was sent the test repository and started,
// with its first page loaded.
func started(tb testing.TB, svc Service, width, height int) *Section {
	tb.Helper()
	s := newSection(tb, svc, width, height)
	run(tb, s, s.Update(ui.RepoMsg{Repo: testRepo}))
	run(tb, s, s.Init())
	return s
}

// run runs cmd and every command that follows from it, feeding their
// messages to s, as the program would. Spinner ticks are dropped so it ends.
// It returns the messages that were fed.
func run(tb testing.TB, s *Section, cmd tea.Cmd) []tea.Msg {
	tb.Helper()
	fed, _ := drive(tb, s, cmd, func(tea.Msg) bool { return false })
	return fed
}

// runHolding runs cmd like run, but holds back the DoneMsgs, so a test can
// look at the optimistic state before GitHub answers.
func runHolding(tb testing.TB, s *Section, cmd tea.Cmd) []ui.DoneMsg {
	tb.Helper()
	_, held := drive(tb, s, cmd, func(msg tea.Msg) bool {
		_, ok := msg.(ui.DoneMsg)
		return ok
	})
	done := make([]ui.DoneMsg, 0, len(held))
	for _, m := range held {
		done = append(done, m.(ui.DoneMsg))
	}
	return done
}

// drive runs cmd and what follows, feeding s every message but those hold
// reports true for, which it returns apart.
func drive(tb testing.TB, s *Section, cmd tea.Cmd, hold func(tea.Msg) bool) (fed, held []tea.Msg) {
	tb.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case spinner.TickMsg:
		default:
			if hold(msg) {
				held = append(held, msg)
				continue
			}
			fed = append(fed, msg)
			queue = append(queue, s.Update(msg))
		}
		if len(fed) > 10_000 {
			tb.Fatal("commands don't settle")
		}
	}
	return fed, held
}

// press presses each key and runs what follows. It returns the messages
// that were fed.
func press(tb testing.TB, s *Section, keys ...string) []tea.Msg {
	tb.Helper()
	msgs := make([]tea.Msg, 0, len(keys))
	for _, k := range keys {
		msgs = append(msgs, run(tb, s, s.Update(keyMsg(k)))...)
	}
	return msgs
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

func has[M tea.Msg](msgs []tea.Msg) (M, bool) {
	for _, m := range msgs {
		if m, ok := m.(M); ok {
			return m, true
		}
	}
	var zero M
	return zero, false
}
