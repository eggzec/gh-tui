// Package pulls is the Pull requests section: a list of the pull requests of
// the selected repository, in tabs by state and filtered in the filter
// modal, each opening into a modal with its detail and comments.
package pulls

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/actions"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Service is what the section needs of the pull requests service.
type Service interface {
	List(ctx context.Context, q pulls.ListQuery) (core.Page[core.PullRequest], error)
	// FreshList reports whether List returns the page of q without a
	// request. It may do I/O.
	FreshList(q pulls.ListQuery) bool
	CachedGet(repo core.RepoRef, number int) (core.PullRequestDetail, bool)
	Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	// Revalidate reads the detail again though it is cached and fresh,
	// which the modal does when it opens.
	Revalidate(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	CachedComments(q pulls.CommentsQuery) (core.Page[core.Comment], bool)
	Comments(ctx context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error)
	// Reviews reads a page of the reviews of a pull request, which the
	// conversation shows among its comments, and CachedReviews returns one
	// from memory, if it is there, without a request.
	Reviews(ctx context.Context, q pulls.ReviewsQuery) (core.Page[core.Review], error)
	CachedReviews(q pulls.ReviewsQuery) (core.Page[core.Review], bool)
	// Files reads a page of the files that a pull request changes, with
	// their patches.
	Files(ctx context.Context, q pulls.FilesQuery) (core.Page[core.CommitFile], error)
	// CachedFiles returns a page of them from memory, if it is there, without
	// a request.
	CachedFiles(q pulls.FilesQuery) (core.Page[core.CommitFile], bool)
	// RangeFiles reads the files that changed between two commits, by their
	// full SHAs, and CachedRange returns them from memory, if they are there,
	// without a request.
	RangeFiles(ctx context.Context, q pulls.RangeQuery) (core.CompareDetail, error)
	CachedRange(q pulls.RangeQuery) (core.CompareDetail, bool)
	// PullCommits reads the commits of a pull request at a head, with their
	// checks and the viewer's last review, and CachedPullCommits returns
	// them from memory, if they are there, without a request.
	PullCommits(ctx context.Context, q pulls.PullCommitsQuery) (core.PullCommits, error)
	CachedPullCommits(q pulls.PullCommitsQuery) (core.PullCommits, bool)
	// Threads reads a page of the review threads of a pull request, with
	// their comments, and CachedThreads returns one from memory, if it is
	// there, without a request.
	Threads(ctx context.Context, q pulls.ThreadsQuery) (core.ThreadPage, error)
	CachedThreads(q pulls.ThreadsQuery) (core.ThreadPage, bool)
	// Timeline reads a page of the timeline of a pull request, and
	// CachedTimeline returns one from memory, if it is there, without a
	// request.
	Timeline(ctx context.Context, q pulls.TimelineQuery) (core.Page[core.TimelineItem], error)
	CachedTimeline(q pulls.TimelineQuery) (core.Page[core.TimelineItem], bool)
	// CurrentGet and CurrentComments report whether Get and Comments
	// would answer without a request. They do no I/O.
	CurrentGet(repo core.RepoRef, number int) bool
	CurrentComments(q pulls.CommentsQuery) bool
	// CurrentFiles reports whether Files would answer without a request.
	// It does no I/O.
	CurrentFiles(q pulls.FilesQuery) bool
	// Invalidate marks what is cached of repo stale, so that the reads
	// after it ask GitHub.
	Invalidate(repo core.RepoRef)
	// SendReview sends a review of a pull request with a verdict and a
	// summary, on head. It is not optimistic: nothing changes until it
	// returns. The viewer's pending review on GitHub, if there is one, is the
	// review that goes.
	SendReview(ctx context.Context, repo core.RepoRef, number int, head string, event core.ReviewEvent, body string) error

	// MergeTexts reads the commit messages GitHub would use to merge a pull
	// request. It is not cached.
	MergeTexts(ctx context.Context, repo core.RepoRef, number int) (core.MergeTexts, error)

	// The changes are shown in the cache at once. The returned Op sends
	// them. A merge is pinned to head, the commit it was confirmed for, and
	// takes msg as its commit message.
	Merge(repo core.RepoRef, number int, method core.MergeMethod, msg core.MergeMessage, head string) *optimistic.Op
	// AutoMerge turns on merging once the checks pass, StopAutoMerge turns
	// it off, and Enqueue adds the pull request to the merge queue of its
	// base branch. Like Merge, they are pinned to head.
	AutoMerge(repo core.RepoRef, number int, method core.MergeMethod, msg core.MergeMessage, head string) *optimistic.Op
	StopAutoMerge(repo core.RepoRef, number int) *optimistic.Op
	Enqueue(repo core.RepoRef, number int, head string) *optimistic.Op
	Close(repo core.RepoRef, number int) *optimistic.Op
	Reopen(repo core.RepoRef, number int) *optimistic.Op
	MarkReady(repo core.RepoRef, number int) *optimistic.Op
	ConvertToDraft(repo core.RepoRef, number int) *optimistic.Op
	// EditPull sets the title and the description, and Comment adds a comment
	// by author to the conversation.
	EditPull(repo core.RepoRef, number int, title, body string) *optimistic.Op
	Comment(repo core.RepoRef, number int, body, author string) *optimistic.Op
	reviewRequester
	labelEditor
	viewedService
	convEditor
}

// ChecksService is what the section needs to show the checks of pull
// requests and to read them ahead.
type ChecksService interface {
	jobview.Service
	// CachedChecks returns the checks of a commit from memory, without a
	// request.
	CachedChecks(q actions.ChecksQuery) (core.Checks, bool)
	Checks(ctx context.Context, q actions.ChecksQuery) (core.Checks, error)
	// CachedRun returns a run from memory, without a request, as a change
	// or a poll left it.
	CachedRun(repo core.RepoRef, runID int64) (core.Run, bool)
	Run(ctx context.Context, repo core.RepoRef, runID int64) (core.Run, error)
	// CachedAllJobs returns the jobs of an attempt from memory, without a
	// request.
	CachedAllJobs(q actions.JobsQuery) (core.Page[core.Job], bool)
	// AllJobs returns the jobs of an attempt, every page of them up to
	// actions.MaxJobPages.
	AllJobs(ctx context.Context, q actions.JobsQuery) (core.Page[core.Job], error)
	// Invalidate marks what is cached of repo stale, so that the reads
	// after it ask GitHub.
	Invalidate(repo core.RepoRef)
	// RerunFailedJobs shows the failed jobs of a run queued in the cache
	// at once. The returned Op sends the re-run.
	RerunFailedJobs(repo core.RepoRef, runID int64) *optimistic.Op
	// FreshChecks reports whether Checks returns the checks of q without
	// a request. It does no I/O.
	FreshChecks(q actions.ChecksQuery) bool
	// CommitChecks reads every check of one commit, which the Files pane opens
	// under it, and CachedCommitChecks returns them from memory, if they are
	// there, without a request.
	CommitChecks(ctx context.Context, q actions.CommitChecksQuery) (core.CommitChecks, error)
	CachedCommitChecks(q actions.CommitChecksQuery) (core.CommitChecks, bool)
	// RerequestSuite shows the runs of a check suite pending at once, which
	// the page of an App's check run offers. The returned Op sends the request.
	RerequestSuite(repo core.RepoRef, suiteID int64) *optimistic.Op
}

// Section shows the pull requests of the selected repository. Create it with
// [New].
type Section struct {
	ctx  context.Context
	svc  Service
	keys keyMap
	// rawKeys are the configured keys, for the steps of the modal.
	rawKeys config.Keymap
	now     func() time.Time
	// checks reads the checks of the modal's panes, and those read ahead.
	// Without checks, the modal has no such panes.
	checks ChecksService
	// commits reads the checks of the head commit for the Merge pane.
	commits CommitChecks
	// commitWatch keeps those checks live while some are pending.
	commitWatch CommitWatch
	// codeOwners reads who owns the files that a pull request changes.
	codeOwners CodeOwners
	// runFollow keeps the log of a running job of the Merge pane current.
	runFollow RunFollow
	// dev reads what a pull request links, for the modal's Development pane.
	// Without it, the pane shows a placeholder.
	dev dev.Service
	// blobs reads a file at the end of a range for the diff to show whole.
	// Without it, the key says so.
	blobs Blobs
	// mergeMethod is how merge merges, if the repository allows it: the
	// method last merged with in this session, or else the configured
	// one. Without either, the repository's.
	mergeMethod core.MergeMethod
	// checking is the number of the pull request whose detail a merge key
	// waits for, or 0, so that pressing the key again waits for the same
	// read.
	checking int
	// repos reads what the viewer may do in the repositories of the
	// modals, and caps is what they may do in repo, as far as it is known.
	repos ui.Repos
	caps  core.RepoCaps
	// readViewer reads the login of the signed-in user, whom the filter
	// offers as @me.
	readViewer Viewer
	// drafts keeps the message of a merge being written, under a key that
	// starts with draftScope, which names the host and the account. Without
	// drafts, an edited message is lost when the dialog closes.
	drafts     composer.Drafts
	draftScope string
	// reviewDrafts keeps the review comments written in the diff of a modal
	// until the review is sent, in memory unless it is given a place on disk.
	reviewDrafts ReviewDrafts

	repo    core.RepoRef
	hasRepo bool
	// tab is the state shown, or empty for all of them, and query the
	// other filters, in GitHub's search syntax; chips are those for the
	// pane's title.
	tab   core.State
	query string
	chips string
	// toggle holds the query that the filter key switched off.
	toggle ui.FilterToggle[string]
	// sortToggle holds the query that the sort key switched off.
	sortToggle ui.SortToggle
	facets     Facets
	started    bool
	focused    bool

	// feed lists the pull requests of repo in tab that query selects. It is nil until the
	// section has started with a repository.
	feed       *feed.Model[core.PullRequest]
	cancelFeed context.CancelFunc
	// voice words the errors of the feed and of the comments.
	voice ui.Voice

	// ahead reads the details, the first comments and the checks of the
	// rows of feed before they are opened, as prefetch.pulls says. rowAt
	// returns the key of row i, which is how ahead knows a row, and keeps
	// the head of its pull request in heads, by key, for the checks.
	// others reads the first pages of the tabs not shown, if
	// prefetch.pulls.other_tabs is on. prefetch is the settings they start
	// with, and slots bound the reads of ahead with those of other pages.
	prefetch *config.PrefetchLayers
	ahead    *ui.Aheads[details.Key]
	slots    *ui.Slots
	rowAt    func(i int) (details.Key, bool)
	heads    sync.Map
	others   *ui.Filters[pulls.ListQuery]

	width, height int
	theme         ui.Theme
	st            styles
	icons         ui.Icons
	// dates tell when the pull requests were updated, in the rows and
	// the modal.
	dates ui.Dates
	// convOpt sets the kinds of events that the Conversation pane shows when
	// it opens, as ui.conversation.filter says; nil shows every kind.
	convOpt conv.Option
	// avatars draws the authors' avatars in the comments of the modal.
	avatars *ui.Images
	cols    columns
	// links keeps the links of the rows, which are drawn on every frame.
	links  termtext.Links
	header string
	// blank is the empty state shown until a repository is picked, and
	// hint what it tells the user to do.
	blank string
	hint  string
}

// Option configures a Section.
type Option func(*Section)

// WithClock sets the function that tells the time, from which ages such as
// "3d" are counted. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Section) { s.now = now }
}

// WithMergeMethod sets the method that a merge starts from, in the
// repositories that allow it. Without it, or where the repository refuses
// it, a merge starts from the viewer's last method there, else squash,
// else the first method the repository allows. A method confirmed in the
// session takes the place of this one.
func WithMergeMethod(m core.MergeMethod) Option {
	return func(s *Section) { s.mergeMethod = m }
}

// WithDrafts keeps the message of a merge being written in d. Its keys start
// with scope, which names the host and the account, so that no account reads
// the drafts of another.
func WithDrafts(d composer.Drafts, scope string) Option {
	return func(s *Section) { s.drafts, s.draftScope = d, scope }
}

// ReviewDrafts keeps the review comments that were written and not sent, under
// the key of their pull request. A [tray.Store] is one.
type ReviewDrafts interface {
	Load(key string) ([]tray.Draft, error)
	Put(key string, d tray.Draft) (tray.Draft, error)
	Remove(key, id string) error
}

// WithReviewDrafts keeps the review comments of the modals in d, so that they
// outlast the session. Without it they last until it ends.
func WithReviewDrafts(d ReviewDrafts) Option {
	return func(s *Section) { s.reviewDrafts = d }
}

// WithRepos reads what the viewer may do in the repository of a modal
// from r, when it isn't the selected one, whose caps the app sends in a
// ui.CapsMsg. Until they are known, every change is offered, and GitHub
// refuses what it doesn't allow.
func WithRepos(r ui.Repos) Option {
	return func(s *Section) { s.repos = r }
}

// Viewer returns the login of the signed-in user. It may do I/O.
type Viewer func(ctx context.Context) (string, error)

// WithViewer sets how the section learns who the user is, so that the
// filter offers them once, as @me, rather than by their login as well.
func WithViewer(v Viewer) Option {
	return func(s *Section) { s.readViewer = v }
}

// WithVoice sets how the section words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(s *Section) { s.voice = v }
}

// WithIcons sets the glyphs of the states of pull requests. Without it, the
// icons are the config's default.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(s *Section) { s.dates = d }
}

// WithConversation sets which kinds of events the Conversation pane shows when
// it opens, as ui.conversation.filter in c says. Without it, every kind.
func WithConversation(c config.Config) Option {
	return func(s *Section) { s.convOpt = conv.Configure(c) }
}

// WithAvatars draws the avatars of the authors of comments with a.
// Without it, or where the terminal shows no images, the comments show
// none and take no room for them.
func WithAvatars(a *ui.Images) Option {
	return func(s *Section) { s.avatars = a }
}

// WithChecks shows the checks of a pull request on the panes of its modal,
// which the checks key opens there and on the rows of the list, read from
// svc. The default has no such panes.
func WithChecks(svc ChecksService) Option {
	return func(s *Section) { s.checks = svc }
}

// WithDevelopment shows what a pull request closes and what is linked to it
// in the Development pane of its modal, read from svc. The default pane
// shows a placeholder.
func WithDevelopment(svc dev.Service) Option {
	return func(s *Section) { s.dev = svc }
}

// Blobs is what the diff needs of the files service to show a file whole.
type Blobs interface {
	Blob(ctx context.Context, q filesvc.BlobQuery) (core.Blob, error)
}

// WithBlobs lets the diff of a pull request show a file whole, as read from
// svc. Without it, the key that does so only says that it can't.
func WithBlobs(svc Blobs) Option {
	return func(s *Section) { s.blobs = svc }
}

// WithPrefetch reads ahead as p says for prefetch.pulls, so that what the
// user opens next opens at once:
//   - details and comments: the detail and the first comments of the rows
//     in the window around the cursor, each time it rests, and at once
//     when a list loads. Each costs a request; what is cached is skipped.
//   - checks: the full list of checks of the same rows, which the
//     modal's header counts and its Checks step shows, if WithChecks set
//     how to read them. Each costs a GraphQL query; checks read within
//     their cache TTL are skipped, even while some are pending, unless
//     they are of another head than the row shows.
//   - other_tabs: the first page of each state not shown, once the user
//     switched tabs in a repository, with the next or previous tab key,
//     and the list shown loaded, so that switching further shows them at
//     once. It reads them once per repository and session. Each costs a
//     request; pages cached fresh are skipped, and so is a list the user
//     filtered.
//   - files: the first page of the files a pull request changes, once its
//     modal rests on a tab other than Files, so that the tab opens at
//     once. It is the modal's own read, of the one pull request that is
//     open, and costs a request; a page cached fresh is skipped.
//
// The default reads nothing ahead.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(s *Section) { s.prefetch = &p }
}

// WithSlots bounds the reads ahead of the section with those of every page
// and modal that shares s, so that together they keep to
// prefetch.parallel. Without it, each kind it reads has slots of its own.
func WithSlots(s *ui.Slots) Option {
	return func(x *Section) { x.slots = s }
}

// New returns the section, reading from svc with the configured keys. ctx
// bounds every request it makes.
func New(ctx context.Context, svc Service, keys config.Keymap, opts ...Option) *Section {
	s := &Section{
		ctx:     ctx,
		svc:     svc,
		voice:   ui.NewVoice(keys, ""),
		keys:    newKeyMap(keys),
		rawKeys: keys,
		now:     time.Now,
		tab:     tabs[0].state,
		icons:   ui.NewIcons(config.Default().UI.Icons),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.reviewDrafts == nil {
		s.reviewDrafts = &tray.Memory{}
	}
	// Bubbles copy the voice, and read the icons through it.
	s.voice.Icons = &s.icons
	s.keys.Checks.SetEnabled(s.keys.Checks.Enabled() && s.checks != nil)
	s.rowAt = func(i int) (details.Key, bool) {
		pr, ok := s.feed.Item(i)
		if ok {
			s.heads.Store(detailKey(s.repo, pr.Number), pr.HeadSHA)
		}
		return detailKey(s.repo, pr.Number), ok
	}
	kinds := details.Reader{Pulls: svc}.Kinds("pull")
	if s.checks != nil {
		kinds = append(kinds, ui.AheadKind[details.Key]{Name: "checks", Log: "pull_checks", Read: s.readChecks, Current: s.freshChecks})
	}
	s.ahead = ui.NewAheads(ctx, "pulls", kinds...)
	s.ahead.Share(s.slots)
	if p := s.prefetch; p != nil {
		s.setPrefetch(*p)
	}
	s.hint = "Search for a repository to see its pull requests."
	if k := ui.Binding(keys, config.ActionSearch, "search").Help().Key; k != "" {
		s.hint = "Press " + k + " to search for one."
	}
	// The app sets the theme of the terminal soon after; until then assume
	// a dark one.
	p, _ := config.Default().Palette(true)
	s.SetTheme(ui.NewTheme(p, true))
	return s
}

// Title implements ui.Section.
func (s *Section) Title() string { return ui.PullsTitle }

// Init lists the pull requests of the repository, if one is selected.
func (s *Section) Init() tea.Cmd {
	s.started = true
	if !s.hasRepo {
		return nil
	}
	return s.newFeed()
}

// newFeed replaces the feed with one for the current repository, tab and
// filter, and returns the command that loads it.
func (s *Section) newFeed() tea.Cmd {
	if s.cancelFeed != nil {
		s.cancelFeed()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.ahead.Reset(ctx)
	s.heads.Clear()
	q := s.listQuery(s.tab)
	svc := s.svc
	query := func(cursor string) pulls.ListQuery {
		q := q
		q.Cursor = cursor
		return q
	}
	fetch := ui.FeedPages("list.pulls", query, func(ctx context.Context, q pulls.ListQuery, again bool) (core.Page[core.PullRequest], error) {
		q.Again = again
		return svc.List(ctx, q)
	})
	f := feed.New(fetch, s.renderRow,
		feed.WithContext(ctx),
		feed.WithKey(pullKey),
		feed.WithKeyMap(s.keys.feed),
		feed.WithMarkKeys(s.keys.mark),
		feed.WithPromptKeys(s.keys.search),
		feed.WithStyles(s.theme.Feed(s.icons)),
		feed.WithFocused(s.focused),
		feed.WithEmptyText(s.emptyText()),
		feed.WithErrorText(ui.ErrorText("load the pull requests", s.repo.String(), s.voice)),
	)
	s.feed, s.cancelFeed = &f, cancel
	s.layout()
	s.renderHeader()
	return f.Init()
}

// listQuery is the query of the first page of the pull requests of the
// repository in state that the filter selects.
func (s *Section) listQuery(state core.State) pulls.ListQuery {
	return pulls.ListQuery{Repo: s.repo, State: state, Filter: s.query}
}

// SetSize implements ui.Section.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.layout()
	s.renderHeader()
	s.blank = s.st.noRepo(s.width, s.height, s.hint)
}

func (s *Section) layout() {
	if s.feed != nil {
		s.feed.SetSize(s.width, max(s.height-headerHeight-headsHeight, 0))
	}
}

// SetTheme implements ui.Section.
func (s *Section) SetTheme(t ui.Theme) {
	s.voice.Icons = &s.icons
	s.theme = t
	s.st = newStyles(t, s.icons)
	if s.feed != nil {
		s.feed.SetStyles(t.Feed(s.icons))
	}
	s.renderHeader()
	s.blank = s.st.noRepo(s.width, s.height, s.hint)
}

// Focus implements ui.Section.
func (s *Section) Focus() {
	s.focused = true
	if s.feed != nil {
		s.feed.Focus()
	}
}

// Blur implements ui.Section.
func (s *Section) Blur() {
	s.focused = false
	if s.feed != nil {
		s.feed.Blur()
	}
}

// View implements ui.Section.
func (s *Section) View() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	if !s.hasRepo || s.feed == nil {
		return s.blank
	}
	if s.height <= headerHeight {
		return s.header
	}
	if s.height == headerHeight+headsHeight {
		return s.header + "\n" + s.renderHeads()
	}
	return s.header + "\n" + s.renderHeads() + "\n" + s.feed.View()
}
