package ui

import (
	"cmp"

	"github.com/eggzec/gh-tui/internal/core"
)

// Selection is what the cursor of a section is on, such as a pull request
// or a file, which the copy command copies from.
type Selection struct {
	// What names it for the user, such as "pull request".
	What string
	// URL is its page on GitHub, and Repo its repository.
	URL  string
	Repo core.RepoRef
	// Number is that of an issue or pull request, SHA that of a commit,
	// and Path that of a file or directory, each where it has one.
	Number int
	SHA    string
	Path   string
	// Owner is the login of the person or organization behind it, whose
	// page the owner key shows: the author of an issue or pull request,
	// or the owner of a repository. It is empty where there is none.
	Owner string
}

// Selector is a Section that tells what its cursor is on, for the copy
// command. It reports false when the cursor is on nothing.
type Selector interface {
	Selected() (Selection, bool)
}

// RepoSelection is the selection of repository r, whose page is url.
func RepoSelection(r core.Repo, url string) Selection {
	return Selection{What: "repository", URL: url, Repo: r.Ref, Owner: r.Ref.Owner}
}

// HitSelection is the selection of a result of a search, whose page is
// repoURL for a repository. The owner of an issue or pull request is its
// author, or its repository's owner when the result names no author.
func HitSelection(hit core.SearchHit, repoURL string) Selection {
	if hit.Kind == core.SearchRepos {
		return RepoSelection(hit.Repo, repoURL)
	}
	what := "issue"
	if hit.Kind == core.SearchPulls {
		what = "pull request"
	}
	it := hit.Issue
	return Selection{What: what, URL: it.URL, Repo: it.Repo, Number: it.Number, Owner: cmp.Or(it.Author.Login, it.Repo.Owner)}
}

// subjects name the kinds of what notifications are about.
var subjects = map[core.SubjectType]string{
	core.SubjectIssue:       "issue",
	core.SubjectPullRequest: "pull request",
	core.SubjectRelease:     "release",
	core.SubjectDiscussion:  "discussion",
	core.SubjectCommit:      "commit",
	core.SubjectCheckSuite:  "workflow run",
}

// NotificationSelection is the selection of what notification n is
// about. Notifications name no author, so its owner is the repository's.
func NotificationSelection(n core.Notification) Selection {
	what, ok := subjects[n.Subject.Type]
	if !ok {
		what = "notification"
	}
	return Selection{What: what, URL: n.Subject.WebURL, Repo: n.Repo, Number: n.Subject.Number, SHA: n.Subject.SHA, Owner: n.Repo.Owner}
}
