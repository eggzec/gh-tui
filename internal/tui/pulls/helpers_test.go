package pulls

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var (
	repo  = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	clock = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
)

// fakeService serves pull requests from memory, a page at a time, and
// records what it was asked.
type fakeService struct {
	mu       sync.Mutex
	pulls    []core.PullRequest
	pageSize int
	queries  []pulls.ListQuery
}

func newFakeService() *fakeService {
	return &fakeService{pulls: samplePulls(), pageSize: 30}
}

func (f *fakeService) List(_ context.Context, q pulls.ListQuery) (core.Page[core.PullRequest], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	var match []core.PullRequest
	for i := range f.pulls {
		if pr := &f.pulls[i]; pr.Repo == q.Repo && pr.State == q.State {
			match = append(match, *pr)
		}
	}
	start, _ := strconv.Atoi(q.Cursor)
	end := min(start+f.pageSize, len(match))
	if start >= end {
		return core.Page[core.PullRequest]{}, nil
	}
	p := core.Page[core.PullRequest]{Items: slices.Clone(match[start:end])}
	if end < len(match) {
		p.Next = strconv.Itoa(end)
	}
	return p, nil
}

func (f *fakeService) listed() []pulls.ListQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.queries)
}

// samplePulls are pull requests as a busy repository has them, in every
// state and review state, most recently updated first.
func samplePulls() []core.PullRequest {
	type spec struct {
		title, author string
		state         core.State
		draft         bool
		review        core.ReviewDecision
		checks        core.ChecksState
		adds, dels    int
		ago           time.Duration
	}
	specs := []spec{
		{"Add a disk layer to the cache so cold starts are fast", "octocat", core.StateOpen, false, core.ReviewApproved, core.ChecksSuccess, 482, 37, 25 * time.Minute},
		{"Retry GraphQL requests after secondary rate limits", "hubot", core.StateOpen, false, core.ReviewChangesRequested, core.ChecksFailure, 96, 12, 3 * time.Hour},
		{"WIP: render markdown tables in the thread view", "monalisa", core.StateOpen, true, core.ReviewNone, core.ChecksPending, 1204, 318, 26 * time.Hour},
		{"Fix the tab bar overflowing at 40 columns", "defunkt", core.StateOpen, false, core.ReviewRequired, core.ChecksSuccess, 18, 4, 2 * 24 * time.Hour},
		{"Bump charm.land/bubbles/v2 to v2.2.1", "dependabot", core.StateOpen, false, core.ReviewRequired, core.ChecksNone, 3, 3, 5 * 24 * time.Hour},
		{"Show a spinner while comments load", "mislav", core.StateOpen, false, core.ReviewApproved, core.ChecksPending, 57, 9, 12 * 24 * time.Hour},
		{"Support GitHub Enterprise hosts from gh's config", "a-very-long-login-name", core.StateOpen, false, core.ReviewNone, core.ChecksSuccess, 23_456, 1_789, 70 * 24 * time.Hour},
		{"Drop the unused REST client for notifications", "vilmibm", core.StateClosed, false, core.ReviewNone, core.ChecksFailure, 0, 640, 4 * 24 * time.Hour},
		{"Experiment: a sidebar for repositories", "octocat", core.StateClosed, true, core.ReviewNone, core.ChecksNone, 300, 20, 40 * 24 * time.Hour},
		{"Rename the watch package to sync", "hubot", core.StateMerged, false, core.ReviewApproved, core.ChecksSuccess, 150, 150, 6 * time.Hour},
		{"Cache ETags alongside list pages", "monalisa", core.StateMerged, false, core.ReviewApproved, core.ChecksSuccess, 210, 44, 9 * 24 * time.Hour},
	}
	prs := make([]core.PullRequest, 0, len(specs))
	for i, sp := range specs {
		n := 142 - i*7
		prs = append(prs, core.PullRequest{
			ID:             "PR_" + strconv.Itoa(n),
			Repo:           repo,
			Number:         n,
			Title:          sp.title,
			State:          sp.state,
			Author:         core.User{Login: sp.author},
			Labels:         []core.Label{{Name: "cache"}, {Name: "enhancement"}},
			Comments:       i % 4,
			CreatedAt:      clock.Add(-sp.ago - 48*time.Hour),
			UpdatedAt:      clock.Add(-sp.ago),
			URL:            "https://github.com/eggzec/gh-tui/pull/" + strconv.Itoa(n),
			Draft:          sp.draft,
			HeadRef:        "feat/change-" + strconv.Itoa(n),
			BaseRef:        "main",
			ReviewDecision: sp.review,
			Checks:         sp.checks,
			Additions:      sp.adds,
			Deletions:      sp.dels,
			ChangedFiles:   1 + sp.adds/50,
		})
	}
	return prs
}

// manyPulls returns n open pull requests, for benchmarks.
func manyPulls(n int) []core.PullRequest {
	base := samplePulls()
	prs := make([]core.PullRequest, 0, n)
	for i := range n {
		pr := base[i%len(base)]
		pr.Number = 5000 - i
		pr.State = core.StateOpen
		prs = append(prs, pr)
	}
	return prs
}

// newTest returns a sized, focused section over svc that has not started.
func newTest(tb testing.TB, svc Service, width, height int) *Section {
	tb.Helper()
	s := New(tb.Context(), svc, config.Default().Keys, WithClock(func() time.Time { return clock }))
	s.SetSize(width, height)
	s.Focus()
	return s
}

// started returns a section that has started on repo and loaded its list.
func started(tb testing.TB, svc Service, width, height int) *Section {
	tb.Helper()
	s := newTest(tb, svc, width, height)
	drain(tb, s, s.Update(ui.RepoMsg{Repo: repo}))
	drain(tb, s, s.Init())
	return s
}

// drain runs cmd and the commands that follow, and feeds their messages to
// s. It skips spinner ticks, which never end, and returns the messages that
// s doesn't produce for itself, such as the app messages.
func drain(tb testing.TB, s *Section, cmd tea.Cmd) []tea.Msg {
	tb.Helper()
	var out []tea.Msg
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 1000 {
			tb.Fatal("drain: too many steps")
		}
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
		case ui.OpenMsg, ui.NotifyMsg:
			out = append(out, msg)
		default:
			out = append(out, msg)
			queue = append(queue, s.Update(msg))
		}
	}
	return out
}

// press sends the key k to s and runs what it starts.
func press(tb testing.TB, s *Section, k string) []tea.Msg {
	tb.Helper()
	return drain(tb, s, s.Update(keyMsg(k)))
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
	r := []rune(k)
	return tea.KeyPressMsg{Code: r[0], Text: k}
}
