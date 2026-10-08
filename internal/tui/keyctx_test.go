package tui

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// The services below serve one of everything the sections and modals of
// the app list, at once, so that every key of theirs has something to act
// on. What no context asks for, such as a change, is left to the embedded
// interface, which fails loudly if it is asked after all.

// keyProse returns sixty paragraphs of markdown, enough to fill any window
// several times over, so that the keys that scroll a text have somewhere
// to go. keyWide returns n plain lines, each far wider than any window.
func keyProse() string {
	paras := make([]string, 60)
	for i := range paras {
		paras[i] = fmt.Sprintf("Paragraph %d. %s", i+1, strings.Repeat("Keys match by order. ", 12))
	}
	return strings.Join(paras, "\n\n") + "\n"
}

func keyWide(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d %s", i+1, strings.Repeat("wide ", 80))
	}
	return strings.Join(lines, "\n") + "\n"
}

// keyTime is the time of everything the services serve.
var keyTime = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

var (
	keyIssue = core.Issue{
		ID: "I_1", Repo: testRepo, Number: 1, Title: "Keys collide", State: core.StateOpen,
		Author: core.User{Login: "octocat"}, CreatedAt: keyTime, UpdatedAt: keyTime,
		URL: "https://github.com/eggzec/gh-tui/issues/1", Body: keyProse(),
	}
	keyPull = core.PullRequest{
		ID: "PR_2", Repo: testRepo, Number: 2, Title: "Match keys by order", State: core.StateOpen,
		Author: core.User{Login: "octocat"}, CreatedAt: keyTime, UpdatedAt: keyTime,
		URL: "https://github.com/eggzec/gh-tui/pull/2", HeadRef: "keys", BaseRef: "main", Body: keyProse(),
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
	keyLog    = keyLongLog()
	keyCommit = core.Commit{
		SHA: "abc123", TreeSHA: "def456", Message: "Match keys by order", Subject: "Match keys by order",
		Author: core.Signature{Name: "Octo Cat", Email: "octo@example.com", Date: keyTime},
		URL:    "https://github.com/eggzec/gh-tui/commit/abc123",
	}
)

// keyLongLog is a log of many wide lines in one group, with an error.
func keyLongLog() core.Log {
	lines := []core.LogLine{{Time: keyTime, Text: "##[group]Run tests", Step: 1}}
	for l := range strings.SplitSeq(strings.TrimSuffix(keyWide(100), "\n"), "\n") {
		lines = append(lines, core.LogLine{Time: keyTime, Text: l, Step: 1})
	}
	return core.Log{Lines: append(lines, core.LogLine{Time: keyTime, Text: "##[error]want a frame, got none", Step: 1})}
}

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

// keyFiles serves a tree of a directory holding a file and of a file, and
// the file.
type keyFiles struct{ files.Service }

var keyTree = core.Tree{SHA: "def456", Entries: []core.TreeEntry{
	{Path: "cmd", Name: "cmd", Type: core.EntryTree, SHA: "t1"},
	{Path: "cmd/main.go", Name: "main.go", Type: core.EntryBlob, SHA: "b2", Size: 4},
	{Path: "README.md", Name: "README.md", Type: core.EntryBlob, SHA: "b1", Size: 6},
}}

// keyReadme is a file of a heading, many paragraphs and a code block
// wider than any window.
var keyReadme = "# Keys\n\n" + keyProse() + "\n```\n" + keyWide(3) + "```\n"

func (keyFiles) CachedTree(filesvc.TreeQuery) (core.Tree, bool)             { return keyTree, true }
func (keyFiles) Tree(context.Context, filesvc.TreeQuery) (core.Tree, error) { return keyTree, nil }
func (keyFiles) CachedAll(filesvc.TreeQuery) (core.Tree, bool)              { return keyTree, true }
func (keyFiles) All(context.Context, filesvc.TreeQuery) (core.Tree, error)  { return keyTree, nil }
func (keyFiles) CachedBlob(filesvc.BlobQuery) (core.Blob, bool) {
	return core.Blob{SHA: "b1", Size: 6, Content: []byte(keyReadme)}, true
}
func (keyFiles) Blob(context.Context, filesvc.BlobQuery) (core.Blob, error) {
	return core.Blob{SHA: "b1", Size: 6, Content: []byte(keyReadme)}, nil
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
		JobID: keyJob.ID, RunID: keyRun.ID, Workflow: "CI", Annotations: 1, StartedAt: keyTime, CompletedAt: keyTime,
		DetailsURL: keyJob.URL,
	}, {
		ID: 31, Name: "coverage", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
		Title: "Coverage fell", Summary: keyProse(), StartedAt: keyTime, CompletedAt: keyTime,
		DetailsURL: "https://coverage.example.com/eggzec/gh-tui",
	}}}
}

// keyHistory serves the default branch, a commit and its detail, and
// keyFile, the file the commit changed.
type keyHistory struct{ history.Service }

var keyFile = core.CommitFile{
	Path: "README.md", Status: core.FileModified, SHA: "b1", Additions: 1, Deletions: 1,
	Patch: keyPatch(60),
}

// keyPatch returns a patch of n changed lines, each far wider than any
// window.
func keyPatch(n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "@@ -1,%d +1,%d @@\n", n, n)
	for i := range n {
		fmt.Fprintf(&b, "-old %d %s\n+new %d %s\n", i, strings.Repeat("wide ", 80), i, strings.Repeat("wide ", 80))
	}
	return b.String()
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
	ID: 40, Tag: "v1.0.0", Name: "v1.0.0", Author: core.User{Login: "octocat"}, Body: keyProse(),
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
	// steps are pressed in turn to reach the context, and then, if set,
	// msg is sent, and after is pressed in turn. Each is pressed as
	// press does: an action by its key in the config, so that a new
	// default key needs no change here. An action is named by its context
	// and its name, such as "pulls.merge" or "global.select".
	steps []string
	msg   tea.Msg
	after []string
	// want names the layers the context has, as layerNames does, so
	// that a context the keys no longer reach fails rather than passes.
	want string
	// context is the context of keys that has the focus, whose chain the
	// layers must make (TestContextChains), or empty for a state of what
	// takes every key that is no context of its own.
	context string
}

// typedStep starts a step that types text, which typed makes.
const typedStep = "type:"

// typed returns the step that types text, a key for each character.
func typed(text string) string { return typedStep + text }

// layerNames names layers by their contexts, or by their sources if they
// are of none, in order, and marks those that type what their keys don't
// take. The keys that the app always takes, and those that ctrl+c reaches
// while something takes every other key, are of no context.
func layerNames(layers []keyhelp.Layer) string {
	names := make([]string, len(layers))
	for i, l := range layers {
		names[i] = l.Context
		if names[i] == "" {
			names[i] = l.Source
		}
		if l.Typing {
			names[i] += " (types)"
		}
	}
	return strings.Join(names, ", ")
}

// layers returns the layers of keys the app has in the context.
func (c keyContext) layers(t *testing.T) []keyhelp.Layer {
	t.Helper()
	_, layers := c.reach(t)
	return layers
}

// reach returns the app in the context, and the layers of keys it has.
func (c keyContext) reach(t *testing.T) (*Model, []keyhelp.Layer) {
	t.Helper()
	m := newKeysApp(t, c.repo)
	var pressed []string
	press := func(steps []string) {
		for _, s := range steps {
			names := c.press(t, m.cfg.Keys, s)
			pressed = append(pressed, s+"="+strings.Join(names, " "))
			for _, name := range names {
				if strings.HasPrefix(s, typedStep) {
					if what := boundKey(m.keyLayers(), name); what != "" {
						t.Fatalf("%s: step %q types %q, which %s takes as a key; name its action", c.name, s, name, what)
					}
				}
				msg, _ := keyPress(name)
				driveKeys(t, m, m.key(msg))
			}
		}
	}
	press(c.steps)
	if c.msg != nil {
		pressed = append(pressed, fmt.Sprintf("%T", c.msg))
		driveKeys(t, m, func() tea.Msg { return c.msg })
	}
	press(c.after)
	layers := m.keyLayers()
	if got := layerNames(layers); got != c.want {
		t.Fatalf("%s: [%s] reach %q, want %q", c.name, strings.Join(pressed, ", "), got, c.want)
	}
	return m, layers
}

// boundKey returns the description of the enabled binding that takes the
// key name before any layer that types does, or "" if the key is typed
// there: text that a step types must be text, and not an action's key.
func boundKey(layers []keyhelp.Layer, name string) string {
	for _, l := range layers {
		for _, b := range l.Bindings {
			if b.Enabled() && slices.Contains(b.Keys(), name) {
				return fmt.Sprintf("%s (%s)", b.Help().Desc, layerNames([]keyhelp.Layer{l}))
			}
		}
		if l.Typing {
			return ""
		}
	}
	return ""
}

// press returns the names of the keys a step of c presses. An action
// presses the first of its keys in keys that can be pressed, as
// Model.press does; typed text presses a key for
// each of its characters. No action is named as a key is
// (TestStepsAreUnambiguous).
func (c keyContext) press(t *testing.T, keys config.Keymap, step string) []string {
	t.Helper()
	if text, ok := strings.CutPrefix(step, typedStep); ok {
		names := make([]string, 0, len(text))
		for _, r := range text {
			if _, ok := keyPress(string(r)); !ok {
				t.Fatalf("%s: can't type %q", c.name, r)
			}
			names = append(names, string(r))
		}
		return names
	}
	if action, ok := stepAction(keys, step); ok {
		bound := keys.Of(action)
		for _, name := range bound {
			if _, ok := keyPress(name); ok {
				return []string{name}
			}
		}
		t.Fatalf("%s: no key bound to %s can be pressed: %q", c.name, step, bound)
	}
	t.Fatalf("%s: %q is neither an action nor typed text; name its action, or type it with typed", c.name, step)
	return nil
}

// stepAction returns the action that step names, by its context and name
// joined by a dot, such as "pulls.merge", if there is one.
func stepAction(keys config.Keymap, step string) (string, bool) {
	if slices.Contains(keys.Actions(), step) {
		return step, true
	}
	return "", false
}

// TestStepsAreUnambiguous checks that no action is named as a key is, so
// that a step of a context is either an action or a key.
func TestStepsAreUnambiguous(t *testing.T) {
	for _, action := range config.Default().Keys.Actions() {
		if _, ok := keyPress(action); ok || strings.HasPrefix(action, typedStep) {
			t.Errorf("the action %s is named as a key is", action)
		}
	}
}

// TestContextChains checks that the keys of each context, as help lists
// them, make the layers of its chain: those of the app, of the screen or
// modal, and of the pane, or those of what takes every key, alone, and no
// others.
func TestContextChains(t *testing.T) {
	reached := map[string]bool{}
	for _, c := range keyContexts() {
		if c.context == "" {
			t.Errorf("%s: names no context; every state is of one", c.name)
			continue
		}
		t.Run(strings.NewReplacer(" ", "-", ":", "").Replace(c.name), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var got []string
				for _, l := range c.layers(t) {
					if l.Context != "" {
						got = append(got, l.Context)
						reached[l.Context] = true
					}
				}
				if want := config.Chain(c.context); !slices.Equal(got, want) {
					t.Errorf("the layers of %s have the contexts %q, want the chain %q", c.name, got, want)
				}
			})
		})
	}
	// Some context has no layer of its own to list, such as a screen whose
	// keys are all its panes'; it is reached by the chain of one of them.
	for _, c := range config.Contexts() {
		if folded, ok := foldedContexts[c.Name]; ok {
			if !reached[folded] {
				t.Errorf("no state of keyContexts reaches the context %s, which lists the keys of %s", folded, c.Name)
			}
			continue
		}
		if !reached[c.Name] {
			t.Errorf("no state of keyContexts reaches the context %s", c.Name)
		}
	}
}

// foldedContexts are the contexts whose keys help lists in the layer of
// another, because one widget holds both: the keys of a picker in normal
// mode are listed with the picker's.
var foldedContexts = map[string]string{"picker_normal": "picker"}

// idleRows are the rows of help whose key, pressed where the context is
// reached, has nothing to act on there: a motion at the end of what it
// moves in. Each is listed by its context and label, with the reason.
var idleRows = map[string]string{
	"actions pane left":  "the runs are the leftmost pane",
	"actions pane right": "the log is the rightmost pane",

	// The readers are reached scrolled down and across a text longer and
	// wider than their window, so only these have nothing to act on.
	"preview left":                 "the file is markdown, shown rendered and wrapped to the pane, so there is no sideways to scroll",
	"preview right":                "the file is markdown, shown rendered and wrapped to the pane, so there is no sideways to scroll",
	"pull_check_detail move left":  "what the app reported is markdown, wrapped to the pane, so there is no sideways to scroll",
	"pull_check_detail move right": "what the app reported is markdown, wrapped to the pane, so there is no sideways to scroll",
	"text left":                    "the config's lines are narrower than the modal, so there is no sideways to scroll",
	"text right":                   "the config's lines are narrower than the modal, so there is no sideways to scroll",
	"pull_check_log follow":        "a log follows its end from the start, so the key turns that off, which shows nothing until lines are appended",
	"actions_log follow":           "a log follows its end from the start, so the key turns that off, which shows nothing until lines are appended",

	"filter previous field":         "the form opens on its first row",
	"filter first field":            "the form opens on its first row",
	"actions_filter previous field": "the form opens on its first row",
	"actions_filter first field":    "the form opens on its first row",
	"filter up":                     "the dropdown lists no options where the services serve none",
	"filter down":                   "the dropdown lists no options where the services serve none",
	"filter top":                    "the dropdown lists no options where the services serve none",
	"filter bottom":                 "the dropdown lists no options where the services serve none",
}

// TestHelpRowsWork checks that every key help lists in a context does
// something when it is pressed there: it starts a command that sends a
// message, or changes what is on view or the keys that work. A row that lists a key which nothing
// takes is a lie in help.
func TestHelpRowsWork(t *testing.T) {
	for _, c := range keyContexts() {
		if c.context == "" {
			continue
		}
		t.Run(strings.NewReplacer(" ", "-", ":", "").Replace(c.name), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				_, layers := c.reach(t)
				for _, l := range layers {
					if l.Context == "" || l.Typing {
						continue
					}
					for _, b := range l.Bindings {
						if !b.Enabled() || b.Help().Desc == "" || !isAction(l.Context, b) {
							continue
						}
						var press tea.KeyPressMsg
						var name string
						for _, k := range b.Keys() {
							if msg, ok := keyPress(k); ok {
								press, name = msg, k
								break
							}
						}
						if name == "" {
							continue
						}
						id := l.Context + " " + b.Help().Desc
						if _, idle := idleRows[id]; idle {
							continue
						}
						m, _ := c.reach(t)
						before, was := ansi.Strip(m.View().Content), layerNames(m.keyLayers())
						cmd, reached := pressReaches(m, press)
						after, is := ansi.Strip(m.View().Content), layerNames(m.keyLayers())
						if !reached && !sends(cmd) && before == after && was == is {
							t.Errorf("%s: %s (%s) does nothing", c.name, name, id)
						}
					}
				}
			})
		})
	}
}

// motions are the actions that move the cursor or the view. Where the
// app has little to show, such as a list of one row, they have nothing to
// move over, so pressing one does nothing there.
var motions = []string{"up", "down", "left", "right", "page_up", "page_down", "half_page_up", "half_page_down", "top", "bottom"}

// sends reports whether cmd, once run, sends a message: a command that
// is only a placeholder, or that sends nothing, is no work.
func sends(cmd tea.Cmd) (sent bool) {
	if cmd == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			sent = true
		}
	}()
	return cmd() != nil
}

// isAction says whether b is the binding of an action of the config in
// the context ctx, by its keys, other than a motion.
func isAction(ctx string, b key.Binding) bool {
	keys := config.Default().Keys
	for _, action := range keys.Actions() {
		name, act, _ := strings.Cut(action, ".")
		if name == ctx && !slices.Contains(motions, act) && slices.Equal(keys.Of(action), b.Keys()) {
			return true
		}
	}
	return false
}

// pressReaches gives the key to m, and says whether it reached something
// the services here don't serve, which fails loudly (see above): such a key
// did reach its action.
func pressReaches(m *Model, press tea.KeyPressMsg) (cmd tea.Cmd, reached bool) {
	defer func() {
		if recover() != nil {
			reached = true
		}
	}()
	return m.key(press), false
}

// newKeysApp returns the app with every section and modal it has, over
// the services above, started on the dashboard, or on testRepo if repo is
// set, on a terminal wide enough to zoom.
func newKeysApp(t *testing.T, repo bool) *Model {
	t.Helper()
	return newKeysAppWith(t, repo, func(*config.Config) {})
}

// newKeysAppWith is newKeysApp over the config that edit changes.
func newKeysAppWith(t *testing.T, repo bool, edit func(*config.Config)) *Model {
	t.Helper()
	ctx, cfg := t.Context(), config.Default()
	edit(&cfg)
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
	octocat, github := ui.OwnerMsg{Login: "octocat"}, ui.OwnerMsg{Login: "github"}
	return []keyContext{
		{name: "owner: repositories", msg: octocat, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: pinned", msg: octocat, after: []string{"global.pane_1"}, context: "owner_pinned", want: "global, owner, owner_pinned"},
		{name: "owner: zoomed", msg: octocat, after: []string{"global.zoom"}, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: filter", msg: octocat, after: []string{"owner_list.filter"}, context: "filter", want: "global, filter"},
		{name: "owner: sort", msg: octocat, after: []string{"owner_list.sort"}, context: "filter", want: "global, filter"},
		{name: "owner: stars", msg: octocat, after: []string{"global.next_tab"}, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: followers", msg: octocat, after: []string{"global.next_tab", "global.next_tab"}, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: following", msg: octocat, after: []string{"global.next_tab", "global.next_tab", "global.next_tab"}, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: organizations", msg: octocat, after: []string{"global.prev_tab"}, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: organization repositories", msg: github, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: members", msg: github, after: []string{"global.next_tab"}, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: teams", msg: github, after: []string{"global.next_tab", "global.next_tab"}, context: "owner_list", want: "global, owner, owner_list"},
		{name: "owner: readme", msg: octocat, after: []string{"global.pane_3", "owner_readme.half_page_down"}, context: "owner_readme", want: "global, owner, owner_readme"},
		{name: "owner: calendar", msg: octocat, after: []string{"global.pane_4"}, context: "owner_calendar", want: "global, owner, owner_calendar"},
		{name: "dashboard: repositories", context: "dashboard_repos", want: "global, dashboard, dashboard_repos"},
		{name: "dashboard: pinned", steps: []string{"global.pane_1"}, context: "dashboard_pinned", want: "global, dashboard, dashboard_pinned"},
		{name: "dashboard: work", steps: []string{"global.pane_3"}, context: "dashboard_work", want: "global, dashboard, dashboard_work"},
		{name: "dashboard: calendar", steps: []string{"global.pane_4"}, context: "dashboard_calendar", want: "global, dashboard, dashboard_calendar"},
		{name: "dashboard: inbox", steps: []string{"global.pane_5"}, context: "dashboard_inbox", want: "global, dashboard, dashboard_inbox"},
		{name: "dashboard: zoomed", steps: []string{"global.zoom"}, context: "dashboard_repos", want: "global, dashboard, dashboard_repos"},
		{name: "dashboard: filter", steps: []string{"dashboard_repos.filter"}, context: "filter", want: "global, filter"},
		{name: "dashboard: sort", steps: []string{"dashboard_repos.sort"}, context: "filter", want: "global, filter"},
		{name: "notifications", steps: []string{"global.notifications"}, context: "notifications", want: "global, notifications"},
		{name: "notifications: filter", steps: []string{"global.notifications", "notifications.filter"}, context: "filter", want: "global, filter"},
		{name: "notifications: mark read", steps: []string{"global.notifications", "notifications.read"}, context: "confirm", want: "always, confirm"},
		{name: "search: normal mode", steps: []string{"global.search", "search.insert", typed("key"), "search_query.cancel", "global.pane_1"}, context: "search", want: "global, search"},
		{name: "search: query", steps: []string{"global.search", "search.insert"}, context: "search_query", want: "always, global, search_query (types)"},
		{name: "search: results", steps: []string{"global.search", "search.insert", typed("key"), "search_query.submit"}, context: "search_results", want: "global, search, search_results"},
		{name: "search: filter", steps: []string{"global.search", "search.insert", typed("key"), "search_query.submit", "search_results.filter"}, context: "filter", want: "global, filter"},
		{name: "search: sort", steps: []string{"global.search", "search.insert", typed("key"), "search_query.submit", "search_results.sort"}, context: "filter", want: "global, filter"},
		{name: "files", repo: true, steps: []string{"files.expand", "files.down"}, context: "files", want: "global, repo, files"},
		{name: "files: zoomed", repo: true, steps: []string{"files.expand", "files.down", "global.zoom"}, context: "files", want: "global, repo, files"},
		{name: "files: error toast", repo: true, steps: []string{"files.expand", "files.down"}, msg: ui.NotifyMsg{Level: toast.Error, Text: "Keys collide."}, context: "files", want: "global, repo, files"},
		{name: "files: preview", repo: true, steps: []string{"files.down", "global.select"}, after: []string{"preview.half_page_down", "preview.right"}, context: "preview", want: "global, preview"},
		{name: "files: preview search", repo: true, steps: []string{"files.down", "global.select", "preview.find"}, context: "search_prompt", want: "always, search_prompt (types)"},
		{name: "files: preview option", repo: true, steps: []string{"files.down", "global.select", "preview.option"}, context: "pager_option", want: "always, pager_option (types)"},
		{name: "files: preview command line", repo: true, steps: []string{"files.down", "global.select", "global.command"}, context: "command_line", want: "always, command_line (types)"},
		{name: "files: finder", repo: true, steps: []string{"global.find_file"}, context: "finder", want: "always, finder (types)"},
		{name: "files: finder preview", repo: true, steps: []string{"global.find_file", typed("R"), "finder.choose"}, after: []string{"preview.half_page_down", "preview.right"}, context: "preview", want: "global, preview"},
		{name: "pull requests", repo: true, steps: []string{"global.pane_2"}, context: "pulls", want: "global, repo, pulls"},
		{name: "pull requests: filter", repo: true, steps: []string{"global.pane_2", "pulls.filter"}, context: "filter", want: "global, filter"},
		{name: "pull requests: sort", repo: true, steps: []string{"global.pane_2", "pulls.sort"}, context: "filter", want: "global, filter"},
		{name: "pull requests: filter list", repo: true, steps: []string{"global.pane_2", "pulls.filter", "filter.down", "filter.toggle"}, context: "filter", want: "global, filter"},
		{name: "pull requests: filter list typing", repo: true, steps: []string{"global.pane_2", "pulls.filter", "filter.down", "filter.toggle", "picker_normal.insert"}, context: "picker", want: "always, picker (types)"},
		{name: "pull requests: filter insert", repo: true, steps: []string{"global.pane_2", "pulls.filter", "filter.bottom", "filter.insert"}, context: "filter_query", want: "always, filter_query (types)"},
		{name: "pull requests: merge", repo: true, steps: []string{"global.pane_2", "pulls.merge"}, context: "confirm", want: "always, confirm"},
		{name: "pull request", repo: true, steps: []string{"global.pane_2", "global.select"}, after: []string{"pull_conversation.half_page_down"}, context: "pull_conversation", want: "global, pull_modal, pull_conversation"},
		{name: "pull request: close", repo: true, steps: []string{"global.pane_2", "global.select", "pull_modal.close"}, context: "confirm", want: "always, confirm"},
		{name: "pull request: checks", repo: true, steps: []string{"global.pane_2", "pulls.checks"}, context: "pull_check_list", want: "global, pull_modal, pull_check_list"},
		{name: "pull request: job", repo: true, steps: []string{"global.pane_2", "pulls.checks", "global.select"}, after: []string{"pull_check_log.half_page_down", "pull_check_log.right"}, context: "pull_check_log", want: "global, pull_modal, pull_check_log"},
		{name: "pull request: check detail", repo: true, steps: []string{"global.pane_2", "pulls.checks", "pull_check_list.down", "global.select"}, after: []string{"pull_check_detail.half_page_down", "pull_check_detail.right"}, context: "pull_check_detail", want: "global, pull_modal, pull_check_detail"},
		{name: "pull request: annotations", repo: true, steps: []string{"global.pane_2", "pulls.checks", "global.select", "pull_check_log.annotations"}, context: "pull_check_annotations", want: "global, pull_modal, pull_check_annotations"},
		{name: "pull request: job search", repo: true, steps: []string{"global.pane_2", "pulls.checks", "global.select", "pull_check_log.find"}, context: "search_prompt", want: "always, search_prompt (types)"},
		{name: "issues", repo: true, steps: []string{"global.pane_3"}, context: "issues", want: "global, repo, issues"},
		{name: "issues: filter", repo: true, steps: []string{"global.pane_3", "issues.filter"}, context: "filter", want: "global, filter"},
		{name: "issues: sort", repo: true, steps: []string{"global.pane_3", "issues.sort"}, context: "filter", want: "global, filter"},
		{name: "issues: close", repo: true, steps: []string{"global.pane_3", "issues.close"}, context: "confirm", want: "always, confirm"},
		{name: "issue", repo: true, steps: []string{"global.pane_3", "global.select"}, after: []string{"issue_modal.half_page_down"}, context: "issue_modal", want: "global, issue_modal"},
		{name: "issue: comment", repo: true, steps: []string{"global.pane_3", "global.select", "issue_modal.comment"}, context: "prompt", want: "always, prompt (types)"},
		{name: "issue: labels", repo: true, steps: []string{"global.pane_3", "global.select", "issue_modal.labels"}, context: "prompt", want: "always, prompt (types)"},
		{name: "history", repo: true, steps: []string{"repo.history"}, context: "history_graph", want: "global, history, history_graph"},
		{name: "history: zoomed", repo: true, steps: []string{"repo.history", "global.zoom"}, context: "history_graph", want: "global, history, history_graph"},
		{name: "history: branches", repo: true, steps: []string{"repo.history", "global.prev_pane"}, context: "history_branches", want: "global, history, history_branches"},
		{name: "history: branch filter", repo: true, steps: []string{"repo.history", "global.prev_pane", "history_branches.filter"}, context: "picker", want: "always, picker (types)"},
		{name: "history: files", repo: true, steps: []string{"repo.history", "global.select"}, context: "history_files", want: "global, history, history_files"},
		{name: "history: patch", repo: true, steps: []string{"repo.history", "global.select", "global.select"}, after: []string{"history_patch.half_page_down", "history_patch.right"}, context: "history_patch", want: "global, history, history_patch"},
		{name: "history: patch search", repo: true, steps: []string{"repo.history", "global.select", "global.select", "history_patch.find"}, context: "search_prompt", want: "always, search_prompt (types)"},
		{name: "commit", repo: true, msg: ui.OpenCommitMsg{Repo: testRepo, SHA: keyCommit.SHA}, context: "history_files", want: "global, history, history_files"},
		{name: "release", repo: true, msg: ui.OpenReleaseMsg{Repo: testRepo, ID: keyRelease.ID, URL: keyRelease.URL}, after: []string{"release_modal.half_page_down"}, context: "release_modal", want: "global, release_modal"},
		{name: "actions: runs", repo: true, steps: []string{"repo.actions"}, context: "actions_runs", want: "global, actions, actions_runs"},
		{name: "actions: jobs", repo: true, steps: []string{"repo.actions", "global.next_pane"}, context: "actions_jobs", want: "global, actions, actions_jobs"},
		{name: "actions: log", repo: true, steps: []string{"repo.actions", "global.next_pane", "global.next_pane"}, after: []string{"actions_log.half_page_down", "actions_log.right"}, context: "actions_log", want: "global, actions, actions_log"},
		{name: "actions: log option", repo: true, steps: []string{"repo.actions", "global.next_pane", "global.next_pane", "actions_log.option"}, context: "log_option", want: "always, log_option"},
		{name: "pull request: job option", repo: true, steps: []string{"global.pane_2", "pulls.checks", "global.select", "pull_check_log.option"}, context: "log_option", want: "always, log_option"},
		{name: "actions: annotations", repo: true, steps: []string{"repo.actions", "global.next_pane", "global.next_pane", "actions_log.annotations"}, context: "actions_annotations", want: "global, actions, actions_annotations"},
		{name: "actions: log search", repo: true, steps: []string{"repo.actions", "global.next_pane", "global.next_pane", "actions_log.find"}, context: "search_prompt", want: "always, search_prompt (types)"},
		{name: "actions: filter", repo: true, steps: []string{"repo.actions", "actions_runs.filter"}, context: "actions_filter", want: "global, actions_filter"},
		{name: "actions: rerun", repo: true, steps: []string{"repo.actions", "actions.rerun_failed"}, context: "confirm", want: "always, confirm"},
		{name: "actions: rerun job", repo: true, steps: []string{"repo.actions", "global.next_pane", "actions_jobs.rerun_job"}, context: "confirm", want: "always, confirm"},
		{name: "auth", repo: true, steps: []string{"global.command", typed("auth"), "command_line.run"}, context: "text", want: "global, text"},
		{name: "config", repo: true, steps: []string{"global.command", typed("config"), "command_line.run"}, after: []string{"text.half_page_down", "text.right"}, context: "text", want: "global, text"},
		{name: "help", repo: true, steps: []string{"global.help"}, context: "help", want: "always, help (types)"},
		{name: "command line", repo: true, steps: []string{"global.command"}, context: "command_line", want: "always, command_line (types)"},
	}
}

// reserved are the actions of default.yaml that nothing reads yet, each
// with its reason.
var reserved = map[string]string{
	"repo.star": "reserved without a key until the repository can be starred from its screen",
}

// TestEveryActionRead checks that the code reads every action of
// default.yaml, and only those: walking every state of keyContexts, which
// opens every modal so that the key maps built on opening are built, it
// records each action the app asks the config for. An action nothing reads
// is a key that does nothing, and one that is read but isn't in the config
// is a typo or an action that was never added.
func TestEveryActionRead(t *testing.T) {
	var mu sync.Mutex
	read, rendering, drawn := map[string]bool{}, false, map[string]bool{}
	stop := config.WatchReads(func(action string) {
		mu.Lock()
		defer mu.Unlock()
		if rendering {
			drawn[action] = true
		}
		read[action] = true
	})
	defer stop()
	for _, c := range keyContexts() {
		synctest.Test(t, func(t *testing.T) {
			m, _ := c.reach(t)
			// Drawing the screen and listing the keys read the keys
			// they hold, not the config.
			mu.Lock()
			rendering = true
			mu.Unlock()
			m.View()
			m.keyLayers()
			mu.Lock()
			rendering = false
			mu.Unlock()
		})
	}
	stop()
	for action := range drawn {
		t.Errorf("drawing reads the action %s from the config", action)
	}

	known := map[string]bool{}
	for _, action := range config.Default().Keys.Actions() {
		known[action] = true
		if _, ok := reserved[action]; ok {
			if read[action] {
				t.Errorf("the code reads %s, which is no longer reserved; remove it from reserved", action)
			}
			continue
		}
		if !read[action] {
			t.Errorf("nothing reads the action %s of default.yaml", action)
		}
	}
	for action := range read {
		if !known[action] {
			t.Errorf("the code reads %s, which default.yaml doesn't have", action)
		}
	}
}

// unnamedKeys are the keys whose bindings are no action of the config,
// each with its reason.
var unnamedKeys = map[string]string{
	"ctrl+c": "always quits, so the config has no action for it",
}

// TestBindingsHavePaths checks that help can name where each binding it
// lists is set in the config: in every state of keyContexts, each binding
// is known by actions that default.yaml has, and by ones whose keys it
// holds, unless it has no keys.
func TestBindingsHavePaths(t *testing.T) {
	// Drawing a state that ctrl+c reaches must not name the key.
	defer func() {
		if got := keymap.Actions(forceQuit); got != nil {
			t.Errorf("ctrl+c answers for %v", got)
		}
	}()
	keys := config.Default().Keys
	var mu sync.Mutex
	seen := map[string]bool{}
	for _, c := range keyContexts() {
		t.Run(strings.NewReplacer(" ", "-", ":", "").Replace(c.name), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				_, layers := c.reach(t)
				for _, l := range layers {
					for _, b := range l.Bindings {
						id := fmt.Sprintf("%s: %q (%s) of %s", c.name, b.Help().Desc, b.Help().Key, layerNames([]keyhelp.Layer{l}))
						named := keymap.Actions(b)
						if _, ok := unnamedKeys[strings.Join(b.Keys(), " ")]; ok {
							// What ctrl+c reaches is the row of the action
							// that quits, with that key alone.
							continue
						}
						if len(named) == 0 {
							t.Errorf("%s has no config path", id)
							continue
						}
						for _, a := range named {
							mu.Lock()
							seen[a] = true
							mu.Unlock()
							if !slices.Contains(keys.Actions(), a) {
								t.Errorf("%s is known by %s, which default.yaml lacks", id, a)
								continue
							}
							if len(b.Keys()) > 0 && !slices.ContainsFunc(keys.Of(a), func(k string) bool { return slices.Contains(b.Keys(), k) }) {
								t.Errorf("%s is known by %s, whose keys %v it doesn't hold (%v)", id, a, keys.Of(a), b.Keys())
							}
						}
					}
				}
			})
		})
	}
	// Rows of other kinds than a context's own are named by the action
	// they were made for: those of a picker in normal mode, one that
	// merges two actions, the prompt of a search, and the keys of the log
	// that the annotations show.
	for _, a := range []string{"picker_normal.insert", "global.pane_1", "global.dismiss", "global.quit", "search_prompt.run", "pull_check_log.right"} {
		if !seen[a] {
			t.Errorf("no binding of a state is known by %s", a)
		}
	}
}

// TestTypedTextIsNotAKey checks that typing a key an action is bound to
// is caught where it is bound, and that a key a typing layer takes is not.
func TestTypedTextIsNotAKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := keyContext{name: "filter list", repo: true, steps: []string{"global.pane_2", "pulls.filter"}, context: "filter", want: "global, filter"}
		if got := boundKey(c.layers(t), "j"); got == "" {
			t.Errorf("j is the filter's down key, but boundKey finds no binding")
		}
		c = keyContext{name: "search query", steps: []string{"global.search", "search.insert"}, context: "search_query", want: "always, global, search_query (types)"}
		if got := boundKey(c.layers(t), "k"); got != "" {
			t.Errorf("k is typed into the query, but boundKey finds %s", got)
		}
	})
}
