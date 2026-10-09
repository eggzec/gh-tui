package pulls

import (
	"cmp"
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
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
	listErr  error
	// fresh are the pages List has served, which it serves again without
	// a request, as the service's cache does, until Invalidate. requests
	// are the lists that weren't fresh.
	fresh    map[pulls.ListQuery]bool
	requests []pulls.ListQuery
	listCtxs []context.Context
	// stateErrs fail the lists of a state.
	stateErrs map[core.State]error
	// cached are the numbers whose detail Get has fetched, which CachedGet
	// then serves.
	cached  map[int]bool
	gets    []int
	getCtxs []context.Context
	getErr  error
	// getKept serves each detail kept, in place of a read GitHub rate
	// limited, as the service does: with no error, telling the watch of
	// the read.
	getKept  bool
	comments []pulls.CommentsQuery
	// commentsErr fails the reads of comments.
	commentsErr error
	// commented are the comment pages Comments has served, which
	// CachedComments then serves.
	commented map[pulls.CommentsQuery]bool
	// ops are the changes asked for, such as "merge 142 squash", and
	// sendErr fails sending them.
	ops     []string
	sendErr error
	// listedAs keeps a changed pull request in the pages of its old state
	// until the change is sent, as the service's cached pages do.
	listedAs map[int]core.State
	// invalidated are the calls of Invalidate.
	invalidated []invalidation
	// thread, when set, are the comments on every pull request, served
	// in one page.
	thread []core.Comment
	// bare are the numbers whose detail counts no checks.
	bare map[int]bool
	// changed are the files every pull request changes, or those at the
	// head named by changedAt, served filePage at a time, or all at once
	// if it is 0. fileReads are the reads of them, and filesErr fails
	// them.
	changed   []core.CommitFile
	changedAt map[string][]core.CommitFile
	filePage  int
	fileReads []pulls.FilesQuery
	filesErr  error
	// filesKept serves the pages as kept for want of an answer from GitHub.
	filesKept bool
}

// invalidation is a call of Invalidate, with how many lists and gets were
// made before it.
type invalidation struct {
	repo        core.RepoRef
	lists, gets int
}

func newFakeService() *fakeService {
	return &fakeService{
		pulls: samplePulls(), pageSize: 30,
		cached: map[int]bool{}, fresh: map[pulls.ListQuery]bool{}, commented: map[pulls.CommentsQuery]bool{}, listedAs: map[int]core.State{},
	}
}

func (f *fakeService) find(number int) core.PullRequest {
	for i := range f.pulls {
		if f.pulls[i].Number == number {
			return f.pulls[i]
		}
	}
	return core.PullRequest{}
}

func (f *fakeService) detail(number int) core.PullRequestDetail {
	pr := f.find(number)
	if pr.Body == "" {
		pr.Body = "## Why\n\nCold starts read **every** page again. This keeps them on disk.\n\n- Pages expire with their TTL\n- `--no-disk` turns it off"
	}
	counts := core.CheckCounts{Passed: 2, Failed: 1, Pending: 1}
	if f.bare[number] {
		counts = core.CheckCounts{}
	}
	return core.PullRequestDetail{PullRequest: pr, CheckCounts: counts}
}

func (f *fakeService) CachedGet(_ core.RepoRef, number int) (core.PullRequestDetail, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cached[number] {
		return core.PullRequestDetail{}, false
	}
	return f.detail(number), true
}

func (f *fakeService) Get(ctx context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, number)
	f.getCtxs = append(f.getCtxs, ctx)
	if f.getErr != nil {
		return core.PullRequestDetail{}, f.getErr
	}
	if f.getKept {
		core.ServedLimited(ctx)
		return f.detail(number), nil
	}
	f.cached[number] = true
	return f.detail(number), nil
}

// Comments serves three comments on every pull request, two a page, or
// the thread if it is set.
func (f *fakeService) Comments(_ context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments = append(f.comments, q)
	if f.commentsErr != nil {
		return core.Page[core.Comment]{}, f.commentsErr
	}
	f.commented[q] = true
	return f.commentPage(q), nil
}

func (f *fakeService) CachedComments(q pulls.CommentsQuery) (core.Page[core.Comment], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.commented[q] {
		return core.Page[core.Comment]{}, false
	}
	return f.commentPage(q), true
}

func (f *fakeService) CurrentGet(_ core.RepoRef, number int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cached[number]
}

func (f *fakeService) CurrentComments(q pulls.CommentsQuery) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commented[q]
}

// readingAhead returns the option that reads the details and the first
// comments of the row under the cursor and of the after rows below it,
// once the cursor rests for rest, and the first pages of the other tabs if
// tabs is set.
func readingAhead(after int, rest time.Duration, tabs bool) Option {
	p := prefetchOf(after, rest)
	p.Pulls.OtherTabs.Enabled = new(tabs)
	return WithPrefetch(p)
}

// prefetchOf returns the settings that read the row under the cursor and
// the after rows below it, once the cursor rests for rest.
func prefetchOf(after int, rest time.Duration) config.PrefetchLayers {
	p := config.Default().Prefetch
	p.Window, p.Rest = config.Window{After: after}, rest
	return p
}

// readingTabs returns the option that reads only the first pages of the
// other tabs ahead.
func readingTabs() Option {
	p := config.Default().Prefetch
	p.Enabled = false
	p.Pulls.OtherTabs.Enabled = new(true)
	return WithPrefetch(p)
}

func (f *fakeService) commentPage(q pulls.CommentsQuery) core.Page[core.Comment] {
	if f.thread != nil {
		return core.Page[core.Comment]{Items: slices.Clone(f.thread)}
	}
	all := []core.Comment{
		{ID: "c1", Author: core.User{Login: "hubot"}, Body: "Does this survive a crash halfway through a write?", CreatedAt: clock.Add(-20 * time.Hour)},
		{ID: "c2", Author: core.User{Login: "octocat"}, Body: "It writes to a temporary file and renames it, so a crash leaves the old page in place.\r\n\r\nI added a test for it.", CreatedAt: clock.Add(-2 * time.Hour)},
		{ID: "c3", Author: core.User{Login: "monalisa"}, Body: "LGTM", CreatedAt: clock.Add(-10 * time.Minute)},
	}
	if q.Cursor == "" {
		return core.Page[core.Comment]{Items: all[:2], Next: "2"}
	}
	return core.Page[core.Comment]{Items: all[2:]}
}

func (f *fakeService) got() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.gets)
}

func (f *fakeService) List(ctx context.Context, q pulls.ListQuery) (core.Page[core.PullRequest], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Again doesn't select another page.
	q.Again = false
	f.queries = append(f.queries, q)
	if !f.fresh[q] {
		f.requests = append(f.requests, q)
		f.listCtxs = append(f.listCtxs, ctx)
	}
	if err := cmp.Or(f.listErr, f.stateErrs[q.State]); err != nil {
		return core.Page[core.PullRequest]{}, err
	}
	f.fresh[q] = true
	var match []core.PullRequest
	for i := range f.pulls {
		pr := &f.pulls[i]
		state, ok := f.listedAs[pr.Number]
		if !ok {
			state = pr.State
		}
		if pr.Repo == q.Repo && (q.State == "" || state == q.State) && matches(pr, q.Filter) {
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

// matches reports whether pr has what the author: and label: qualifiers
// of filter ask for, the only ones the fake knows.
func matches(pr *core.PullRequest, filter string) bool {
	for word := range strings.FieldsSeq(filter) {
		k, v, _ := strings.Cut(word, ":")
		switch k {
		case "author":
			if pr.Author.Login != v {
				return false
			}
		case "label":
			if !slices.ContainsFunc(pr.Labels, func(l core.Label) bool { return l.Name == v }) {
				return false
			}
		}
	}
	return true
}

func (f *fakeService) Invalidate(repo core.RepoRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated = append(f.invalidated, invalidation{repo, len(f.queries), len(f.gets)})
	clear(f.fresh)
}

func (f *fakeService) FreshList(q pulls.ListQuery) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fresh[q]
}

// requested returns the lists that weren't fresh.
func (f *fakeService) requested() []pulls.ListQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func (f *fakeService) invalidations() []invalidation {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.invalidated)
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

// host stands in for the app: it sends keys to the modal opened last, or
// else to the section, every other message to both, and opens and closes
// the modals. The modals are as large as the section.
type host struct {
	*Section
	modals []ui.Modal
}

// Update routes msg as the app does.
func (h *host) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.OpenModalMsg:
		msg.Modal.SetTheme(h.theme)
		msg.Modal.SetSize(h.width, h.height)
		h.modals = append(h.modals, msg.Modal)
		return nil
	case ui.CloseModalMsg:
		if i := slices.Index(h.modals, msg.Modal); i >= 0 {
			h.modals = h.modals[:i]
		}
		return nil
	case tea.KeyPressMsg:
		if len(h.modals) > 0 {
			return h.modals[len(h.modals)-1].Update(msg)
		}
		return h.Section.Update(msg)
	}
	cmds := []tea.Cmd{h.Section.Update(msg)}
	for _, m := range h.modals {
		cmds = append(cmds, m.Update(msg))
	}
	return tea.Batch(cmds...)
}

// modal returns the detail open on top, or nil.
func (h *host) modal() *detailModal {
	if len(h.modals) == 0 {
		return nil
	}
	m, _ := h.modals[len(h.modals)-1].(*detailModal)
	return m
}

// newTest returns a sized, focused section over svc that has not started.
func newTest(tb testing.TB, svc Service, width, height int, opts ...Option) *host {
	tb.Helper()
	opts = append([]Option{WithClock(func() time.Time { return clock })}, opts...)
	s := New(tb.Context(), svc, config.Default().Keys, opts...)
	s.SetSize(width, height)
	s.Focus()
	return &host{Section: s}
}

// started returns a section that has started on repo and loaded its list.
func started(tb testing.TB, svc Service, width, height int, opts ...Option) *host {
	tb.Helper()
	h := newTest(tb, svc, width, height, opts...)
	drain(tb, h, h.Update(ui.RepoMsg{Repo: repo}))
	drain(tb, h, h.Init())
	return h
}

// drain runs cmd and the commands that follow, and feeds their messages to
// h. It skips spinner ticks, which never end, and keeps the messages for
// the app that h doesn't handle, such as OpenMsg. It returns every message.
func drain(tb testing.TB, h *host, cmd tea.Cmd) []tea.Msg {
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
		msg := c()
		if seq, ok := sequence(msg); ok {
			// Run each command of a sequence to the end before the next.
			for _, sc := range seq {
				out = append(out, drain(tb, h, sc)...)
			}
			continue
		}
		switch msg := msg.(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case spinner.TickMsg:
		case ui.OpenMsg, ui.NotifyMsg:
			out = append(out, msg)
		default:
			out = append(out, msg)
			queue = append(queue, h.Update(msg))
		}
	}
	return out
}

// sequence returns the commands of the message tea.Sequence sends, whose
// type bubbletea doesn't export.
func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	if _, ok := msg.(tea.BatchMsg); ok || msg == nil {
		return nil, false
	}
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	return v.Convert(reflect.TypeFor[[]tea.Cmd]()).Interface().([]tea.Cmd), true
}

// press sends the key k to h and runs what it starts.
func press(tb testing.TB, h *host, k string) []tea.Msg {
	tb.Helper()
	return drain(tb, h, h.Update(keyMsg(k)))
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	r := []rune(k)
	return tea.KeyPressMsg{Code: r[0], Text: k}
}

// change applies edit to pull request number at once, as the service does
// to its cache, and returns the Op that sends it and rolls it back if
// sending fails.
func (f *fakeService) change(what string, number int, edit func(*core.PullRequest)) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, what+" "+strconv.Itoa(number))
	i := slices.IndexFunc(f.pulls, func(pr core.PullRequest) bool { return pr.Number == number })
	if i < 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	before := f.pulls[i]
	f.listedAs[number] = before.State
	edit(&f.pulls[i])
	return optimistic.New(func(context.Context) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.sendErr == nil {
			delete(f.listedAs, number)
		}
		return f.sendErr
	}, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.pulls[i] = before
	})
}

func (f *fakeService) Merge(_ core.RepoRef, number int, method core.MergeMethod, head string) *optimistic.Op {
	what := "merge " + string(method)
	if head != "" {
		what += " at " + head
	}
	return f.change(what, number, func(pr *core.PullRequest) { pr.State = core.StateMerged })
}

func (f *fakeService) Close(_ core.RepoRef, number int) *optimistic.Op {
	return f.change("close", number, func(pr *core.PullRequest) { pr.State = core.StateClosed })
}

func (f *fakeService) Reopen(_ core.RepoRef, number int) *optimistic.Op {
	return f.change("reopen", number, func(pr *core.PullRequest) { pr.State = core.StateOpen })
}

func (f *fakeService) MarkReady(_ core.RepoRef, number int) *optimistic.Op {
	return f.change("ready", number, func(pr *core.PullRequest) { pr.Draft = false })
}

func (f *fakeService) ConvertToDraft(_ core.RepoRef, number int) *optimistic.Op {
	return f.change("draft", number, func(pr *core.PullRequest) { pr.Draft = true })
}

func (f *fakeService) changes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ops)
}

func (f *fakeService) state(number int) core.PullRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.find(number)
}

// errMark is the error glyph of the default icons, which mark what failed.
var errMark = ui.NewIcons(config.IconsNerd).Error

func (f *fakeService) Files(_ context.Context, q pulls.FilesQuery) (core.Page[core.CommitFile], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fileReads = append(f.fileReads, q)
	if f.filesErr != nil {
		return core.Page[core.CommitFile]{}, f.filesErr
	}
	all, ok := f.changedAt[q.Head]
	if !ok {
		all = f.changed
	}
	start, _ := strconv.Atoi(q.Cursor)
	end := min(start+cmp.Or(f.filePage, len(all)), len(all))
	p := core.Page[core.CommitFile]{Items: slices.Clone(all[min(start, end):end]), Offline: f.filesKept}
	if end < len(all) {
		p.Next = strconv.Itoa(end)
	}
	return p, nil
}

// fileReadCount returns how many pages of files were read.
func (f *fakeService) fileReadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.fileReads)
}
