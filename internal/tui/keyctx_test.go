package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	dashsvc "github.com/eggzec/gh-tui/internal/service/dashboard"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	notifsvc "github.com/eggzec/gh-tui/internal/service/notifications"
	ownersvc "github.com/eggzec/gh-tui/internal/service/owners"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	searchsvc "github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/files"
	"github.com/eggzec/gh-tui/internal/tui/history"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/owner"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/releases"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// The services below serve one of everything the sections and modals of
// the app list, at once, so that every key of theirs has something to act
// on. What no context asks for, such as a change, is left to the embedded
// interface, which fails loudly if it is asked after all.

// keyTime is the time of everything the services serve.
var keyTime = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

var (
	keyIssue = core.Issue{
		ID: "I_1", Repo: testRepo, Number: 1, Title: "Keys collide", State: core.StateOpen,
		Author: core.User{Login: "octocat"}, CreatedAt: keyTime, UpdatedAt: keyTime,
		URL: "https://github.com/eggzec/gh-tui/issues/1",
	}
	keyPull = core.PullRequest{
		ID: "PR_2", Repo: testRepo, Number: 2, Title: "Match keys by order", State: core.StateOpen,
		Author: core.User{Login: "octocat"}, CreatedAt: keyTime, UpdatedAt: keyTime,
		URL: "https://github.com/eggzec/gh-tui/pull/2", HeadRef: "keys", BaseRef: "main",
	}
	keyRepo = core.Repo{
		ID: "R_1", Ref: testRepo, DefaultBranch: "main", UpdatedAt: keyTime,
		URL: "https://github.com/eggzec/gh-tui",
	}
	// keyRun failed, and its job has a log and an annotation.
	keyRun = core.Run{
		ID: 10, Attempt: 1, Name: "CI", DisplayTitle: "Match keys by order", Number: 7, Event: "push",
		Branch: "keys", HeadSHA: "abc123", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
		Actor: "octocat", WorkflowID: 1, CreatedAt: keyTime, UpdatedAt: keyTime, RunStartedAt: keyTime,
		URL: "https://github.com/eggzec/gh-tui/actions/runs/10",
	}
	keyJob = core.Job{
		ID: 20, RunID: 10, Attempt: 1, Name: "test", WorkflowName: "CI", Status: core.RunCompleted,
		Conclusion: core.ConclusionFailure, StartedAt: keyTime, CompletedAt: keyTime,
		Steps: []core.Step{{Number: 1, Name: "Run tests", Status: core.RunCompleted, Conclusion: core.ConclusionFailure}},
		URL:   "https://github.com/eggzec/gh-tui/actions/runs/10/job/20",
	}
	keyLog = core.Log{Lines: []core.LogLine{
		{Time: keyTime, Text: "##[group]Run tests", Step: 1},
		{Time: keyTime, Text: "##[error]want a frame, got none", Step: 1},
	}}
	keyCommit = core.Commit{
		SHA: "abc123", TreeSHA: "def456", Message: "Match keys by order", Subject: "Match keys by order",
		Author: core.Signature{Name: "Octo Cat", Email: "octo@example.com", Date: keyTime},
		URL:    "https://github.com/eggzec/gh-tui/commit/abc123",
	}
)

// keyPulls serves one open pull request and its detail.
type keyPulls struct{ pulls.Service }

func (keyPulls) List(context.Context, pullsvc.ListQuery) (core.Page[core.PullRequest], error) {
	return core.Page[core.PullRequest]{Items: []core.PullRequest{keyPull}}, nil
}
func (keyPulls) CachedList(pullsvc.ListQuery) (core.Page[core.PullRequest], bool) {
	return core.Page[core.PullRequest]{Items: []core.PullRequest{keyPull}}, true
}
func (keyPulls) FreshList(pullsvc.ListQuery) bool { return true }
func (keyPulls) CachedGet(core.RepoRef, int) (core.PullRequestDetail, bool) {
	return core.PullRequestDetail{PullRequest: keyPull}, true
}
func (keyPulls) Get(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
	return core.PullRequestDetail{PullRequest: keyPull}, nil
}
func (keyPulls) CachedComments(pullsvc.CommentsQuery) (core.Page[core.Comment], bool) {
	return core.Page[core.Comment]{}, true
}
func (keyPulls) Comments(context.Context, pullsvc.CommentsQuery) (core.Page[core.Comment], error) {
	return core.Page[core.Comment]{}, nil
}
func (keyPulls) CurrentGet(core.RepoRef, int) bool { return true }

func (keyPulls) CurrentComments(pullsvc.CommentsQuery) bool { return true }
func (keyPulls) Invalidate(core.RepoRef)                    {}

// keyIssues serves one open issue and its detail.
type keyIssues struct{ issues.Service }

func (keyIssues) List(context.Context, issuesvc.ListQuery) (core.Page[core.Issue], error) {
	return core.Page[core.Issue]{Items: []core.Issue{keyIssue}}, nil
}
func (keyIssues) CachedList(issuesvc.ListQuery) (core.Page[core.Issue], bool) {
	return core.Page[core.Issue]{Items: []core.Issue{keyIssue}}, true
}
func (keyIssues) FreshList(issuesvc.ListQuery) bool              { return true }
func (keyIssues) CachedGet(core.RepoRef, int) (core.Issue, bool) { return keyIssue, true }
func (keyIssues) Get(context.Context, core.RepoRef, int) (core.Issue, error) {
	return keyIssue, nil
}
func (keyIssues) CachedComments(issuesvc.CommentsQuery) (core.Page[core.Comment], bool) {
	return core.Page[core.Comment]{}, true
}
func (keyIssues) Comments(context.Context, issuesvc.CommentsQuery) (core.Page[core.Comment], error) {
	return core.Page[core.Comment]{}, nil
}
func (keyIssues) CurrentGet(core.RepoRef, int) bool { return true }

func (keyIssues) CurrentComments(issuesvc.CommentsQuery) bool { return true }
func (keyIssues) Invalidate(core.RepoRef)                     {}

// keyFiles serves a tree of a directory and a file, and the file.
type keyFiles struct{ files.Service }

var keyTree = core.Tree{SHA: "def456", Entries: []core.TreeEntry{
	{Path: "cmd", Name: "cmd", Type: core.EntryTree, SHA: "t1"},
	{Path: "README.md", Name: "README.md", Type: core.EntryBlob, SHA: "b1", Size: 6},
}}

func (keyFiles) CachedTree(filesvc.TreeQuery) (core.Tree, bool)             { return keyTree, true }
func (keyFiles) Tree(context.Context, filesvc.TreeQuery) (core.Tree, error) { return keyTree, nil }
func (keyFiles) CachedAll(filesvc.TreeQuery) (core.Tree, bool)              { return keyTree, true }
func (keyFiles) All(context.Context, filesvc.TreeQuery) (core.Tree, error)  { return keyTree, nil }
func (keyFiles) CachedBlob(filesvc.BlobQuery) (core.Blob, bool) {
	return core.Blob{SHA: "b1", Size: 6, Content: []byte("# Keys\n")}, true
}
func (keyFiles) Blob(context.Context, filesvc.BlobQuery) (core.Blob, error) {
	return core.Blob{SHA: "b1", Size: 6, Content: []byte("# Keys\n")}, nil
}
func (keyFiles) Invalidate(core.RepoRef) {}

// keyInbox serves one unread notification.
type keyInbox struct{ notifications.Service }

var keyThread = core.Notification{
	ID: "1", Repo: testRepo, Reason: "mention", Unread: true, UpdatedAt: keyTime,
	Subject: core.Subject{Title: "Keys collide", Type: core.SubjectIssue, Number: 1, WebURL: keyIssue.URL},
}

func (keyInbox) CachedList(notifsvc.ListQuery) (core.Page[core.Notification], bool) {
	return core.Page[core.Notification]{Items: []core.Notification{keyThread}}, true
}
func (keyInbox) List(context.Context, notifsvc.ListQuery) (core.Page[core.Notification], error) {
	return core.Page[core.Notification]{Items: []core.Notification{keyThread}}, nil
}
func (keyInbox) FreshList(notifsvc.ListQuery) bool { return true }
func (keyInbox) Invalidate()                       {}

// keyDash serves a profile with a pinned repository, a repository, and
// work waiting on the viewer.
type keyDash struct{ dashboard.Service }

var keyWork = core.Work{
	ReviewRequested: core.WorkList{Count: 1, Items: []core.SearchHit{{Kind: core.SearchPulls, Repo: keyRepo, Issue: keyPull.Issue}}},
	Authored:        core.WorkList{Count: 1, Items: []core.SearchHit{{Kind: core.SearchPulls, Repo: keyRepo, Issue: keyPull.Issue}}},
	Assigned:        core.WorkList{Count: 1, Items: []core.SearchHit{{Kind: core.SearchIssues, Repo: keyRepo, Issue: keyIssue}}},
}

// keyHeader's organizations: charmbracelet has the repository, and
// octo-org none.
func keyHeader() core.Header {
	return core.Header{
		Profile: core.Profile{Login: "octocat", CreatedAt: keyTime}, Pinned: []core.Repo{keyRepo},
		Orgs: []core.Org{{Login: "charmbracelet"}, {Login: "octo-org"}},
	}
}

// keyRepos returns the repositories of q: none of octo-org.
func keyRepos(q dashsvc.ReposQuery) core.Page[core.Repo] {
	if q.Owner == "octo-org" {
		return core.Page[core.Repo]{}
	}
	return core.Page[core.Repo]{Items: []core.Repo{keyRepo}}
}

func (keyDash) CachedHeader() (core.Header, bool) { return keyHeader(), true }
func (keyDash) Header(context.Context, dashsvc.HeaderQuery) (core.Header, error) {
	return keyHeader(), nil
}
func (keyDash) CachedWork(dashsvc.WorkQuery) (core.Work, bool)             { return keyWork, true }
func (keyDash) Work(context.Context, dashsvc.WorkQuery) (core.Work, error) { return keyWork, nil }
func (keyDash) CachedContributions() (core.Contributions, bool)            { return core.Contributions{}, true }
func (keyDash) Contributions(context.Context, dashsvc.ContributionsQuery) (core.Contributions, error) {
	return core.Contributions{}, nil
}
func (keyDash) CachedRepos(q dashsvc.ReposQuery) (core.Page[core.Repo], bool) {
	return keyRepos(q), true
}
func (keyDash) FreshHeader() bool                  { return true }
func (keyDash) FreshWork(dashsvc.WorkQuery) bool   { return true }
func (keyDash) FreshContributions() bool           { return true }
func (keyDash) FreshRepos(dashsvc.ReposQuery) bool { return true }
func (keyDash) Repos(_ context.Context, q dashsvc.ReposQuery) (core.Page[core.Repo], error) {
	return keyRepos(q), nil
}
func (keyDash) CachedAllRepos(q dashsvc.ReposQuery, _ int) (core.Page[core.Repo], bool) {
	return keyRepos(q), true
}
func (keyDash) AllRepos(_ context.Context, q dashsvc.ReposQuery, _ int) (core.Page[core.Repo], error) {
	return keyRepos(q), nil
}
func (keyDash) Invalidate() {}

// keyOwners serves the user octocat, with a pin, a repository, a star
// and a person in each list, and every other login as an organization
// the viewer belongs to, with a member and a team.
type keyOwners struct{ keyOwnerSide }

func keyOwner(login string) core.Owner {
	if login != "octocat" {
		return core.Owner{Kind: core.OwnerOrg, Profile: core.Profile{Login: login, Repos: 1}, Viewer: core.Relation{Member: true}}
	}
	return core.Owner{Profile: core.Profile{Login: "octocat", Repos: 1}, Pinned: []core.Repo{keyRepo}}
}

var (
	keyPerson = core.Person{Login: "mona", Name: "Mona Lisa"}
	keyTeam   = core.Team{Name: "Core", Slug: "core", URL: "https://github.com/orgs/github/teams/core"}
)

func (keyOwners) CachedHeader(login string) (core.Owner, bool) { return keyOwner(login), true }
func (keyOwners) FreshHeader(string) bool                      { return true }
func (keyOwners) Header(_ context.Context, q ownersvc.HeaderQuery) (core.Owner, error) {
	return keyOwner(q.Login), nil
}
func (keyOwners) FreshStars(ownersvc.StarsQuery) bool { return true }
func (keyOwners) Stars(context.Context, ownersvc.StarsQuery) (core.Page[core.Repo], error) {
	return core.Page[core.Repo]{Items: []core.Repo{keyRepo}}, nil
}
func (keyOwners) FreshPeople(ownersvc.PeopleQuery) bool { return true }
func (keyOwners) People(context.Context, ownersvc.PeopleQuery) (core.Page[core.Person], error) {
	return core.Page[core.Person]{Items: []core.Person{keyPerson}}, nil
}
func (keyOwners) FreshTeams(ownersvc.TeamsQuery) bool { return true }
func (keyOwners) Teams(context.Context, ownersvc.TeamsQuery) (core.Page[core.Team], error) {
	return core.Page[core.Team]{Items: []core.Team{keyTeam}}, nil
}
func (keyOwners) FreshRepos(ownersvc.ReposQuery) bool { return true }
func (keyOwners) Repos(context.Context, ownersvc.ReposQuery) (core.Page[core.Repo], error) {
	return core.Page[core.Repo]{Items: []core.Repo{keyRepo}}, nil
}
func (keyOwners) CachedAllRepos(ownersvc.ReposQuery, int) (core.Page[core.Repo], bool) {
	return core.Page[core.Repo]{Items: []core.Repo{keyRepo}}, true
}
func (keyOwners) AllRepos(context.Context, ownersvc.ReposQuery, int) (core.Page[core.Repo], error) {
	return core.Page[core.Repo]{Items: []core.Repo{keyRepo}}, nil
}
func (keyOwners) InvalidateLogin(string) {}

// keySearch finds one issue and one file.
type keySearch struct{ searchpage.Service }

func keyResult() searchsvc.Result {
	return searchsvc.Result{SearchPage: core.SearchPage[core.SearchHit]{
		Items: []core.SearchHit{{Kind: core.SearchIssues, Repo: keyRepo, Issue: keyIssue}},
	}, Counts: map[core.SearchKind]int{core.SearchIssues: 1}}
}

func (keySearch) CachedSearch(searchsvc.Query) (searchsvc.Result, bool) { return keyResult(), true }
func (keySearch) Search(context.Context, searchsvc.Query) (searchsvc.Result, error) {
	return keyResult(), nil
}
func (keySearch) Prefetch(context.Context, searchsvc.Query) error { return nil }
func (keySearch) CachedCode(searchsvc.CodeQuery) (core.SearchPage[core.CodeHit], bool) {
	return core.SearchPage[core.CodeHit]{Items: []core.CodeHit{{Repo: testRepo, Path: "README.md", SHA: "b1"}}}, true
}
func (keySearch) Code(context.Context, searchsvc.CodeQuery) (core.SearchPage[core.CodeHit], error) {
	return core.SearchPage[core.CodeHit]{Items: []core.CodeHit{{Repo: testRepo, Path: "README.md", SHA: "b1"}}}, nil
}
func (keySearch) Invalidate() {}

// keyActions serves the failed run, its job and the job's log, to the
// actions and to the checks of the pull request, whose checks are the job
// and one an app reported.
type keyActions struct{ actions.Service }

var _ checks.Service = keyActions{}

func (keyActions) Runs(context.Context, actionssvc.RunsQuery) (core.Page[core.Run], error) {
	return core.Page[core.Run]{Items: []core.Run{keyRun}}, nil
}
func (keyActions) CachedRun(core.RepoRef, int64) (core.Run, bool)             { return keyRun, true }
func (keyActions) Run(context.Context, core.RepoRef, int64) (core.Run, error) { return keyRun, nil }
func (keyActions) Workflows(context.Context, actionssvc.WorkflowsQuery) (core.Page[core.Workflow], error) {
	return core.Page[core.Workflow]{Items: []core.Workflow{{ID: 1, Name: "CI"}}}, nil
}
func (keyActions) CachedAllJobs(actionssvc.JobsQuery) (core.Page[core.Job], bool) {
	return core.Page[core.Job]{Items: []core.Job{keyJob}}, true
}
func (keyActions) AllJobs(context.Context, actionssvc.JobsQuery) (core.Page[core.Job], error) {
	return core.Page[core.Job]{Items: []core.Job{keyJob}}, nil
}
func (keyActions) CachedLog(core.RepoRef, int64) (core.Log, bool)             { return keyLog, true }
func (keyActions) Log(context.Context, core.RepoRef, int64) (core.Log, error) { return keyLog, nil }
func (keyActions) CachedPartialLog(core.RepoRef, int64) (core.PartialLog, bool) {
	return core.PartialLog{}, false
}
func (keyActions) PartialLog(context.Context, core.RepoRef, int64) (core.PartialLog, error) {
	return core.PartialLog{}, nil
}
func (keyActions) WatchLog(core.RepoRef, int64, int64) func() { return func() {} }
func (keyActions) CachedAnnotations(actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool) {
	return core.Page[core.Annotation]{Items: []core.Annotation{{Path: "keys_test.go", StartLine: 1, Level: core.AnnotationFailure, Message: "want a frame"}}}, true
}
func (keyActions) Annotations(context.Context, actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error) {
	return core.Page[core.Annotation]{Items: []core.Annotation{{Path: "keys_test.go", StartLine: 1, Level: core.AnnotationFailure, Message: "want a frame"}}}, nil
}
func (keyActions) CachedChecks(actionssvc.ChecksQuery) (core.Checks, bool) { return keyChecks(), true }
func (keyActions) FreshChecks(actionssvc.ChecksQuery) bool                 { return true }
func (keyActions) Checks(context.Context, actionssvc.ChecksQuery) (core.Checks, error) {
	return keyChecks(), nil
}
func (keyActions) Invalidate(core.RepoRef) {}

func keyChecks() core.Checks {
	return core.Checks{SHA: "abc123", State: core.ChecksFailure, Total: 2, Runs: []core.Check{{
		ID: 30, Name: "test", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
		JobID: keyJob.ID, RunID: keyRun.ID, Workflow: "CI", StartedAt: keyTime, CompletedAt: keyTime,
		DetailsURL: keyJob.URL,
	}, {
		ID: 31, Name: "coverage", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
		Title: "Coverage fell", Summary: "Coverage fell by 2%.", StartedAt: keyTime, CompletedAt: keyTime,
		DetailsURL: "https://coverage.example.com/eggzec/gh-tui",
	}}}
}

// keyHistory serves the default branch, a commit and its detail, and
// keyFile, the file the commit changed.
type keyHistory struct{ history.Service }

var keyFile = core.CommitFile{
	Path: "README.md", Status: core.FileModified, SHA: "b1", Additions: 1, Deletions: 1,
	Patch: "@@ -1 +1 @@\n-# Keys\n+# Keys by order\n",
}

func (keyHistory) Branches(context.Context, historysvc.BranchesQuery) (core.Page[core.Branch], error) {
	return core.Page[core.Branch]{Items: []core.Branch{{Name: "main", SHA: "abc123"}}}, nil
}
func (keyHistory) Commits(context.Context, historysvc.CommitsQuery) (core.Page[core.Commit], error) {
	return core.Page[core.Commit]{Items: []core.Commit{keyCommit}}, nil
}
func (keyHistory) CachedCommit(core.RepoRef, string) (core.CommitDetail, bool) {
	return core.CommitDetail{Commit: keyCommit, Files: []core.CommitFile{keyFile}}, true
}
func (keyHistory) Commit(context.Context, core.RepoRef, string) (core.CommitDetail, error) {
	return core.CommitDetail{Commit: keyCommit, Files: []core.CommitFile{keyFile}}, nil
}
func (keyHistory) CommitFiles(context.Context, historysvc.CommitFilesQuery) (core.Page[core.CommitFile], error) {
	return core.Page[core.CommitFile]{Items: []core.CommitFile{keyFile}}, nil
}

func (keyHistory) CachedCompare(core.RepoRef, string, string) (core.Compare, bool) {
	return core.Compare{}, false
}
func (keyHistory) Compare(context.Context, core.RepoRef, string, string) (core.Compare, error) {
	return core.Compare{}, nil
}

// keyReleases serves a release with one file.
type keyReleases struct{ releases.Service }

var keyRelease = core.Release{
	ID: 40, Tag: "v1.0.0", Name: "v1.0.0", Author: core.User{Login: "octocat"}, Body: "Keys by order.",
	URL: "https://github.com/eggzec/gh-tui/releases/tag/v1.0.0", CreatedAt: keyTime, PublishedAt: keyTime,
	Assets: []core.ReleaseAsset{{Name: "gh-tui.tar.gz", Size: 1024}},
}

func (keyReleases) CachedGet(core.RepoRef, int64) (core.Release, bool) { return keyRelease, true }
func (keyReleases) Get(context.Context, core.RepoRef, int64) (core.Release, error) {
	return keyRelease, nil
}

// keyContext is somewhere keys reach in the app: a screen and its focused
// pane, a modal and each of its steps and panes, what takes every key
// while it types, the help and the command line.
type keyContext struct {
	name string
	// repo opens the app on testRepo; otherwise it opens on the dashboard.
	repo bool
	// keys are pressed in turn to reach the context, and then, if set,
	// msg is sent, and after is pressed in turn.
	keys  []string
	msg   tea.Msg
	after []string
	// want names the layers the context has, as layerNames does, so
	// that a context the keys no longer reach fails rather than passes.
	want string
}

// layerNames names layers by their sources, in order, and marks those that
// type what their keys don't take.
func layerNames(layers []keyhelp.Layer) string {
	names := make([]string, len(layers))
	for i, l := range layers {
		names[i] = l.Source
		if l.Typing {
			names[i] += " (types)"
		}
	}
	return strings.Join(names, ", ")
}

// layers returns the layers of keys the app has in the context.
func (c keyContext) layers(t *testing.T) []keyhelp.Layer {
	t.Helper()
	m := newKeysApp(t, c.repo)
	for _, k := range c.keys {
		msg, ok := keyPress(k)
		if !ok {
			t.Fatalf("%s: can't press %q", c.name, k)
		}
		driveKeys(t, m, m.key(msg))
	}
	if c.msg != nil {
		driveKeys(t, m, func() tea.Msg { return c.msg })
	}
	for _, k := range c.after {
		msg, ok := keyPress(k)
		if !ok {
			t.Fatalf("%s: can't press %q", c.name, k)
		}
		driveKeys(t, m, m.key(msg))
	}
	layers := m.keyLayers()
	if got := layerNames(layers); got != c.want {
		t.Fatalf("%s: the keys reach %q, want %q", c.name, got, c.want)
	}
	return layers
}

// newKeysApp returns the app with every section and modal it has, over
// the services above, started on the dashboard, or on testRepo if repo is
// set, on a terminal wide enough to zoom.
func newKeysApp(t *testing.T, repo bool) *Model {
	t.Helper()
	ctx, cfg := t.Context(), config.Default()
	acc := newFakeAccess(uitest.Classic("repo", "workflow", "notifications", "read:org", "gist"))
	v := ui.NewVoice(cfg.Keys, "")
	v.Token = ui.NewToken(acc, cfg.Keys)
	layout := Layout{
		Files: files.New(ctx, keyFiles{}, cfg.Keys, files.WithVoice(v)),
		Pulls: pulls.New(ctx, keyPulls{}, cfg.Keys, pulls.WithVoice(v),
			pulls.WithChecks(keyActions{}, checks.WithVoice(v))),
		Issues:        issues.New(ctx, keyIssues{}, cfg.Keys, issues.WithVoice(v)),
		Notifications: notifications.New(ctx, keyInbox{}, cfg.Keys, notifications.WithVoice(v)),
		Search:        searchpage.New(ctx, keySearch{}, cfg.Keys, searchpage.WithVoice(v)),
		Dashboard: dashboard.New(ctx, keyDash{}, cfg.Keys, dashboard.WithVoice(v),
			dashboard.WithInbox(keyInbox{}), dashboard.WithHere(testRepo, nil)),
		Owner: owner.New(ctx, keyOwners{}, cfg.Keys, owner.WithVoice(v)),
	}
	opts := []Option{
		WithVoice(v), WithAccess(acc),
		WithHistory(history.Opener(keyHistory{}, cfg.Keys, history.WithVoice(v))),
		WithCommit(history.CommitOpener(keyHistory{}, cfg.Keys, history.WithVoice(v))),
		WithRelease(releases.Opener(keyReleases{}, cfg.Keys, releases.WithVoice(v))),
		WithActions(actions.Opener(keyActions{}, cfg.Keys, actions.WithVoice(v))),
	}
	if repo {
		opts = append(opts, WithRepo(testRepo))
	}
	m := New(ctx, cfg, layout, opts...)
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	// The changes of what the token may do never end.
	m.accessChanges = nil
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	driveKeys(t, m, m.Init())
	return m
}

// driveKeys runs cmd and the commands that follow, and gives their
// messages to m, in synctest's bubble, where timers fire at once. Spinner
// ticks are dropped, as they never end.
func driveKeys(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 2000 {
			t.Fatalf("the commands don't end: %d left", len(queue))
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if cmds, ok := cmdsOf(msg); ok {
			queue = append(queue, cmds...)
			continue
		}
		switch msg.(type) {
		case nil, spinner.TickMsg, tea.QuitMsg:
			continue
		}
		_, next := m.Update(msg)
		queue = append(queue, next)
	}
}

// cmdsOf returns the commands of a batch or a sequence, whose message
// type bubbletea doesn't export.
func cmdsOf(msg tea.Msg) ([]tea.Cmd, bool) {
	if b, ok := msg.(tea.BatchMsg); ok {
		return b, true
	}
	v := reflect.ValueOf(msg)
	if msg == nil || v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	return v.Convert(reflect.TypeFor[[]tea.Cmd]()).Interface().([]tea.Cmd), true
}

// keyContexts returns every context the collisions test walks.
func keyContexts() []keyContext {
	return []keyContext{
		{name: "owner: repositories", msg: ui.OwnerMsg{Login: "octocat"}, want: "app, app, profile, list"},
		{name: "owner: pinned", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"1"}, want: "app, app, profile"},
		{name: "owner: zoomed", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"z"}, want: "app, app, profile, list"},
		{name: "owner: filter", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"f"}, want: "app, filter"},
		{name: "owner: sort", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"s"}, want: "app, filter"},
		{name: "owner: stars", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"]"}, want: "app, app, profile, list"},
		{name: "owner: followers", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"]", "]"}, want: "app, app, profile, list"},
		{name: "owner: members", msg: ui.OwnerMsg{Login: "github"}, after: []string{"]"}, want: "app, app, profile, list"},
		{name: "owner: teams", msg: ui.OwnerMsg{Login: "github"}, after: []string{"]", "]"}, want: "app, app, profile, list"},
		{name: "owner: readme", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"3"}, want: "app, app, profile, readme"},
		{name: "owner: calendar", msg: ui.OwnerMsg{Login: "octocat"}, after: []string{"4"}, want: "app, app, profile, calendar"},
		{name: "dashboard: repositories", want: "app, app, dashboard, list"},
		{name: "dashboard: pinned", keys: []string{"1"}, want: "app, app, dashboard"},
		{name: "dashboard: work", keys: []string{"3"}, want: "app, app, dashboard"},
		{name: "dashboard: calendar", keys: []string{"4"}, want: "app, app, dashboard, calendar"},
		{name: "dashboard: inbox", keys: []string{"5"}, want: "app, app, dashboard"},
		{name: "dashboard: zoomed", keys: []string{"z"}, want: "app, app, dashboard, list"},
		{name: "dashboard: filter", keys: []string{"f"}, want: "app, filter"},
		{name: "dashboard: sort", keys: []string{"s"}, want: "app, filter"},
		{name: "notifications", keys: []string{"n"}, want: "app, app, Notifications, list"},
		{name: "notifications: filter", keys: []string{"n", "f"}, want: "app, filter"},
		{name: "notifications: mark read", keys: []string{"n", "m"}, want: "app, confirm"},
		{name: "search: query", keys: []string{"/"}, want: "app, app, query (types), query"},
		{name: "search: kinds", keys: []string{"/", "up"}, want: "app, app, search"},
		{name: "search: results", keys: []string{"/", "k", "e", "y", "enter"}, want: "app, app, search, results"},
		{name: "files", repo: true, want: "app, app, Files, tree"},
		{name: "files: zoomed", repo: true, keys: []string{"z"}, want: "app, app, Files, tree"},
		{name: "files: preview", repo: true, keys: []string{"down", "enter"}, want: "app, file, pager"},
		{name: "files: preview search", repo: true, keys: []string{"down", "enter", "/"}, want: "app, file, pager (types)"},
		{name: "files: preview option", repo: true, keys: []string{"down", "enter", "-"}, want: "app, file, pager (types)"},
		{name: "files: preview count", repo: true, keys: []string{"down", "enter", "5"}, want: "app, file, pager (types)"},
		{name: "files: finder", repo: true, keys: []string{"t"}, want: "app, finder, find (types)"},
		{name: "pull requests", repo: true, keys: []string{"2"}, want: "app, Pull requests, app, Pull requests, list"},
		{name: "pull requests: filter", repo: true, keys: []string{"2", "f"}, want: "app, filter"},
		{name: "pull requests: sort", repo: true, keys: []string{"2", "s"}, want: "app, filter"},
		{name: "pull requests: filter field", repo: true, keys: []string{"2", "f", "down", "enter"}, want: "app, filter (types)"},
		{name: "pull requests: merge", repo: true, keys: []string{"2", "m"}, want: "app, confirm"},
		{name: "pull request", repo: true, keys: []string{"2", "enter"}, want: "app, pull request, thread"},
		{name: "pull request: close", repo: true, keys: []string{"2", "enter", "x"}, want: "app, confirm"},
		{name: "pull request: checks", repo: true, keys: []string{"2", "C"}, want: "app, checks"},
		{name: "pull request: job", repo: true, keys: []string{"2", "C", "enter"}, want: "app, checks, annotations, log"},
		{name: "pull request: check detail", repo: true, keys: []string{"2", "C", "down", "enter"}, want: "app, checks, detail"},
		{name: "pull request: job search", repo: true, keys: []string{"2", "C", "enter", "/"}, want: "app, annotations, log (types)"},
		{name: "issues", repo: true, keys: []string{"3"}, want: "app, Issues, app, Issues, list"},
		{name: "issues: close", repo: true, keys: []string{"3", "x"}, want: "app, confirm"},
		{name: "issue", repo: true, keys: []string{"3", "enter"}, want: "app, issue, thread"},
		{name: "issue: comment", repo: true, keys: []string{"3", "enter", "c"}, want: "app, prompt (types)"},
		{name: "issue: labels", repo: true, keys: []string{"3", "enter", "l"}, want: "app, prompt (types)"},
		{name: "history", repo: true, keys: []string{"B"}, want: "app, history, graph"},
		{name: "history: zoomed", repo: true, keys: []string{"B", "z"}, want: "app, history, graph"},
		{name: "history: branches", repo: true, keys: []string{"B", "shift+tab"}, want: "app, history, branches"},
		{name: "history: branch filter", repo: true, keys: []string{"B", "shift+tab", "/"}, want: "app, filter (types)"},
		{name: "history: files", repo: true, keys: []string{"B", "enter"}, want: "app, history, files"},
		{name: "history: patch", repo: true, keys: []string{"B", "enter", "enter"}, want: "app, history, pager"},
		{name: "history: patch search", repo: true, keys: []string{"B", "enter", "enter", "/"}, want: "app, pager (types)"},
		{name: "commit", repo: true, msg: ui.OpenCommitMsg{Repo: testRepo, SHA: keyCommit.SHA}, want: "app, history, files"},
		{name: "release", repo: true, msg: ui.OpenReleaseMsg{Repo: testRepo, ID: keyRelease.ID, URL: keyRelease.URL}, want: "app, release, thread"},
		{name: "actions: runs", repo: true, keys: []string{"a"}, want: "app, actions, runs"},
		{name: "actions: jobs", repo: true, keys: []string{"a", "tab"}, want: "app, actions, jobs"},
		{name: "actions: log", repo: true, keys: []string{"a", "tab", "tab"}, want: "app, actions, annotations, log"},
		{name: "actions: log search", repo: true, keys: []string{"a", "tab", "tab", "/"}, want: "app, annotations, log (types)"},
		{name: "actions: filter", repo: true, keys: []string{"a", "f"}, want: "app, filter"},
		{name: "actions: rerun", repo: true, keys: []string{"a", "ctrl+r"}, want: "app, confirm"},
		{name: "actions: rerun all", repo: true, keys: []string{"a", "R"}, want: "app, confirm"},
		{name: "actions: rerun job", repo: true, keys: []string{"a", "tab", "J"}, want: "app, confirm"},
		{name: "auth", repo: true, keys: []string{":", "a", "u", "t", "h", "enter"}, want: "app, token"},
		{name: "help", repo: true, keys: []string{"?"}, want: "app, help (types)"},
		{name: "command line", repo: true, keys: []string{":"}, want: "command line (types)"},
	}
}
