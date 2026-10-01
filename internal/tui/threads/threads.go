// Package threads opens what a notification thread is about in the app,
// in the modal of its issue, pull request, runs, commit or release, and
// reads it ahead, so that it opens at once. What the app has no view of,
// such as a discussion, opens in the browser. The notifications screen and
// the dashboard's inbox share one, so that a thread opens the same way from
// both, and one rate limit stops the reads ahead of both.
package threads

import (
	"context"
	"regexp"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Pulls is what the opener needs of the pull requests service to read a
// pull request ahead, as its modal reads it.
type Pulls interface {
	Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	Comments(ctx context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error)
	CurrentGet(repo core.RepoRef, number int) bool
	CurrentComments(q pulls.CommentsQuery) bool
	Changed(repo core.RepoRef, number int, updated time.Time)
}

// Issues is what the opener needs of the issues service to read an issue
// ahead, as its modal reads it.
type Issues interface {
	Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error)
	Comments(ctx context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error)
	CurrentGet(repo core.RepoRef, number int) bool
	CurrentComments(q issuesvc.CommentsQuery) bool
	Changed(repo core.RepoRef, number int, updated time.Time)
}

// Releases is what the opener needs of the releases service to read a
// release ahead.
type Releases interface {
	Get(ctx context.Context, repo core.RepoRef, id int64) (core.Release, error)
	Current(repo core.RepoRef, id int64) bool
}

// Option configures an Opener.
type Option func(*Opener)

// WithPulls reads the pull requests of notifications ahead through svc.
func WithPulls(svc Pulls) Option {
	return func(o *Opener) { o.pulls = svc }
}

// WithIssues reads the issues of notifications ahead through svc.
func WithIssues(svc Issues) Option {
	return func(o *Opener) { o.issues = svc }
}

// WithReleases reads the releases of notifications ahead through svc.
func WithReleases(svc Releases) Option {
	return func(o *Opener) { o.releases = svc }
}

// WithPrefetch reads ahead what the threads in the window around the
// cursor are about, each time it rests, and at once when a list loads, as
// p says for prefetch.notifications: details reads the pull request,
// issue or release, and comments the first comments of a pull request or
// issue. Each costs a request; what is cached and as recent as the
// notification is skipped. The default reads nothing ahead.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(o *Opener) { o.prefetch = &p }
}

// WithMarkRead sets whether opening a thread marks it read, which
// [Opener.MarksRead] tells the sections. Without it, it does as the
// config's default says.
func WithMarkRead(on bool) Option {
	return func(o *Opener) { o.markRead = on }
}

// Opener opens threads and reads them ahead. Create it with [New]; a nil
// *Opener opens everything in the browser and reads nothing ahead.
//
// The views that share an Opener read ahead only while they are on view,
// one at a time, and [Opener.Stop] it when they leave the screen.
type Opener struct {
	// ctx bounds every read ahead.
	ctx      context.Context
	pulls    Pulls
	issues   Issues
	releases Releases
	markRead bool

	// details reads the pull requests and issues, as the other lists do.
	// ahead reads them ahead, by kind; prefetch is the settings it starts
	// with.
	details  details.Reader
	prefetch *config.PrefetchLayers
	ahead    *ui.Aheads[target]
}

// New returns an Opener whose reads ahead ctx bounds.
func New(ctx context.Context, opts ...Option) *Opener {
	o := &Opener{ctx: ctx, markRead: config.Default().Notifications.MarkReadOnOpen}
	for _, opt := range opts {
		opt(o)
	}
	o.details = details.Reader{Pulls: o.pulls, Issues: o.issues}
	// What there is no service for counts as cached, so it isn't read.
	o.ahead = ui.NewAheads(ctx, "notifications",
		ui.AheadKind[target]{Name: "details", Log: "thread", Read: o.read, Current: o.current},
		ui.AheadKind[target]{Name: "comments", Log: "thread_comments", Read: o.readComments, Current: o.currentComments})
	if o.prefetch != nil {
		o.ahead.Configure(*o.prefetch)
	}
	return o
}

// MarksRead reports whether opening a thread marks it read.
func (o *Opener) MarksRead() bool {
	return o == nil || o.markRead
}

// target is what a thread is about that can be read ahead, and when the
// notification says it last changed.
type target struct {
	kind    core.SubjectType
	repo    core.RepoRef
	number  int
	release int64
	updated time.Time
}

// detail returns the key of the pull request or issue t is about.
func (t target) detail() details.Key {
	return details.Key{Pull: t.kind == core.SubjectPullRequest, Repo: t.repo, Number: t.number}
}

// targetOf returns what n is about, if it can be read ahead.
func targetOf(n core.Notification) (target, bool) {
	t := target{kind: n.Subject.Type, repo: n.Repo, updated: n.UpdatedAt}
	switch n.Subject.Type {
	case core.SubjectIssue, core.SubjectPullRequest:
		t.number = n.Subject.Number
		return t, t.number > 0
	case core.SubjectRelease:
		t.release = n.Subject.ReleaseID
		return t, t.release > 0
	default:
		return target{}, false
	}
}

// Open returns the command that opens what n is about: its issue, pull
// request, runs, commit or release in its modal, or else its page in the
// browser, with a toast that says why. It doesn't mark n read.
func (o *Opener) Open(n core.Notification) tea.Cmd {
	sub := n.Subject
	// The modal of an issue or a pull request holds the reads ahead while
	// it loads, as those of the lists do.
	var pause ui.Pauser
	if o != nil && o.ahead.On() {
		pause = o.ahead
	}
	var msg tea.Msg
	switch sub.Type {
	case core.SubjectIssue:
		if sub.Number > 0 {
			msg = ui.OpenIssueMsg{Repo: n.Repo, Number: sub.Number, Pause: pause}
		}
	case core.SubjectPullRequest:
		if sub.Number > 0 {
			msg = ui.OpenPullMsg{Repo: n.Repo, Number: sub.Number, Pause: pause}
		}
	case core.SubjectCheckSuite:
		if f, ok := runFilter(sub); ok {
			msg = ui.OpenActionsMsg{Repo: n.Repo, Filter: f}
		}
	case core.SubjectCommit:
		if sub.SHA != "" {
			msg = ui.OpenCommitMsg{Repo: n.Repo, SHA: sub.SHA}
		}
	case core.SubjectRelease:
		if sub.ReleaseID > 0 {
			msg = ui.OpenReleaseMsg{Repo: n.Repo, ID: sub.ReleaseID, URL: sub.WebURL}
		}
	default:
	}
	if msg == nil {
		return tea.Batch(ui.Open(sub.WebURL), ui.Notify(toast.Info, browserText(sub.Type)))
	}
	if t, ok := targetOf(n); ok && o != nil {
		// What the notification says changed is read again behind the
		// modal, which shows what is cached at once.
		o.changed(t)
		o.ahead.Opened(t)
	}
	return func() tea.Msg { return msg }
}

// browserText tells the user why a thread opened in the browser.
func browserText(typ core.SubjectType) string {
	if typ == core.SubjectDiscussion {
		return "Opened in the browser — gh-tui has no discussion view yet"
	}
	return "Opened in the browser — gh-tui has no view of it yet"
}

// runTitle is the title GitHub gives a notification of a workflow run,
// such as "CI workflow run failed for main branch".
var runTitle = regexp.MustCompile(`^(.+) workflow run (\w+) for (.+) branch$`)

// runFilter selects the runs a notification of a check suite is about: its
// subject has no URL, but its title names the branch and how the run
// ended, which the runs list filters by without another request.
func runFilter(sub core.Subject) (core.RunFilter, bool) {
	m := runTitle.FindStringSubmatch(sub.Title)
	if m == nil {
		return core.RunFilter{}, false
	}
	f := core.RunFilter{Branch: m[3]}
	switch m[2] {
	case "failed":
		f.Status = string(core.ConclusionFailure)
	case "succeeded":
		f.Status = string(core.ConclusionSuccess)
	case "cancelled":
		f.Status = string(core.ConclusionCancelled)
	}
	return f, true
}

// Reset cancels the reads ahead in flight, for a new list whose reads
// parent bounds.
func (o *Opener) Reset(parent context.Context) {
	if o != nil {
		o.ahead.Reset(parent)
	}
}

// Stop cancels the reads ahead in flight and forgets the rows and the
// cursor they were for, such as when the view that asked for them leaves
// the screen. The next ReadAhead starts over, skipping what is cached.
func (o *Opener) Stop() {
	if o != nil {
		o.ahead.Reset(o.ctx)
	}
}

// Resume reads ahead again after GitHub reported the rate limit, such as
// after a refresh.
func (o *Opener) Resume() {
	if o != nil {
		o.ahead.Resume()
	}
}

// ReadAhead tells the reads ahead that the cursor is on row i of a list,
// which item returns by index, and false for a row not loaded; a nil item
// says the list hasn't loaded. Once the cursor rests, they read what the
// threads in the window around it are about. Call it after every update
// of the list: it waits again only when the window changed.
func (o *Opener) ReadAhead(item func(i int) (core.Notification, bool), i int) tea.Cmd {
	if o == nil {
		return nil
	}
	if item == nil {
		return o.ahead.Window(nil, i)
	}
	return o.ahead.Window(func(j int) (target, bool) {
		n, ok := item(j)
		if !ok {
			return target{}, false
		}
		// A thread about what has nothing to read counts toward the
		// window, and costs nothing.
		return targetOf(n)
	}, i)
}

// Rested reads what the thread the cursor rested on is about, unless it
// moved since.
func (o *Opener) Rested(msg ui.AheadMsg) tea.Cmd {
	if o == nil {
		return nil
	}
	return o.ahead.Rested(msg)
}

// changed tells the service of t that it changed when the notification
// says, which marks stale what is cached from before. It does no I/O.
func (o *Opener) changed(t target) {
	switch {
	case t.kind == core.SubjectPullRequest && o.pulls != nil:
		o.pulls.Changed(t.repo, t.number, t.updated)
	case t.kind == core.SubjectIssue && o.issues != nil:
		o.issues.Changed(t.repo, t.number, t.updated)
	}
}

// current reports whether the detail of what t is about, or its release,
// is cached and as recent as the notification, so that reading it costs
// no request. Without a service to read it, there is nothing to read. It
// does no I/O.
func (o *Opener) current(t target) bool {
	o.changed(t)
	switch t.kind {
	case core.SubjectPullRequest, core.SubjectIssue:
		return o.details.CurrentDetail(t.detail())
	case core.SubjectRelease:
		return o.releases == nil || o.releases.Current(t.repo, t.release)
	default:
		return true
	}
}

// currentComments reports whether the first comments of what t is about
// are cached and as recent as the notification, or it has none to read.
// It does no I/O.
func (o *Opener) currentComments(t target) bool {
	o.changed(t)
	switch t.kind {
	case core.SubjectPullRequest, core.SubjectIssue:
		return o.details.CurrentComments(t.detail())
	default:
		return true
	}
}

// readComments reads the first comments of the pull request or issue t is
// about into the cache its modal reads from.
func (o *Opener) readComments(ctx context.Context, t target) error {
	switch t.kind {
	case core.SubjectPullRequest, core.SubjectIssue:
		return o.details.ReadComments(ctx, t.detail())
	default:
		return nil
	}
}

// read reads what t is about into the cache its modal reads from: the
// detail of a pull request or issue, or the release.
func (o *Opener) read(ctx context.Context, t target) error {
	switch t.kind {
	case core.SubjectPullRequest, core.SubjectIssue:
		return o.details.ReadDetail(ctx, t.detail())
	case core.SubjectRelease:
		_, err := o.releases.Get(ctx, t.repo, t.release)
		return err
	default:
		return nil
	}
}
