package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// History uses the REST commits API: it lists a commit's parents with it,
// which draws the graph without a second request, and answers conditional
// requests, which GraphQL doesn't.

// restCommit is the REST shape of a commit, in a list or alone. Author and
// Committer are GitHub accounts, and null when git's email matches none.
type restCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Author    commitSignature `json:"author"`
		Committer commitSignature `json:"committer"`
		Message   string          `json:"message"`
		Tree      struct {
			SHA string `json:"sha"`
		} `json:"tree"`
		Verification commitVerification `json:"verification"`
	} `json:"commit"`
	Author    *user `json:"author"`
	Committer *user `json:"committer"`
	Parents   []struct {
		SHA string `json:"sha"`
	} `json:"parents"`
	HTMLURL string `json:"html_url"`
}

// commitSignature is who made a commit, and when, as git recorded it.
type commitSignature struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// commitVerification is what GitHub found checking a commit's signature.
type commitVerification struct {
	Verified   bool       `json:"verified"`
	Reason     string     `json:"reason"`
	Signature  *string    `json:"signature"`
	VerifiedAt *time.Time `json:"verified_at"`
}

func (v commitVerification) core() core.Verification {
	out := core.Verification{Verified: v.Verified, Signed: v.Signature != nil && *v.Signature != "", Reason: v.Reason}
	if v.VerifiedAt != nil {
		out.VerifiedAt = *v.VerifiedAt
	}
	return out
}

func (c restCommit) core() core.Commit {
	parents := make([]string, len(c.Parents))
	for i, p := range c.Parents {
		parents[i] = p.SHA
	}
	subject, body, trailers := core.SplitMessage(c.Commit.Message)
	return core.Commit{
		SHA:          c.SHA,
		TreeSHA:      c.Commit.Tree.SHA,
		Parents:      parents,
		Message:      c.Commit.Message,
		Subject:      subject,
		Body:         body,
		Trailers:     trailers,
		Author:       commitUser(c.Author, c.Commit.Author),
		Committer:    commitUser(c.Committer, c.Commit.Committer),
		Verification: c.Commit.Verification.core(),
		URL:          c.HTMLURL,
	}
}

// commitUser is the signature git recorded, with the login of the account
// GitHub matched to its email, if any.
func commitUser(account *user, sig commitSignature) core.Signature {
	out := core.Signature{Name: sig.Name, Email: sig.Email, Date: sig.Date}
	if account != nil {
		out.Login = account.Login
	}
	return out
}

// restCommitDetail is the REST shape of one commit with its changes.
type restCommitDetail struct {
	restCommit
	Stats struct {
		Additions int `json:"additions"`
		Deletions int `json:"deletions"`
		Total     int `json:"total"`
	} `json:"stats"`
	Files []commitFile `json:"files"`
}

// commitFile is the REST shape of a file that a commit changed.
type commitFile struct {
	Filename         string  `json:"filename"`
	PreviousFilename string  `json:"previous_filename"`
	Status           string  `json:"status"`
	SHA              string  `json:"sha"`
	Additions        int     `json:"additions"`
	Deletions        int     `json:"deletions"`
	Changes          int     `json:"changes"`
	Patch            *string `json:"patch"`
}

func (f commitFile) core() core.CommitFile {
	out := core.CommitFile{
		Path:         f.Filename,
		PreviousPath: f.PreviousFilename,
		Status:       core.FileStatus(f.Status),
		SHA:          f.SHA,
		Additions:    f.Additions,
		Deletions:    f.Deletions,
	}
	if f.Patch != nil {
		out.Patch = *f.Patch
	} else {
		// A binary file changes no lines, so it has no patch to leave out.
		out.PatchTruncated = f.Changes > 0
	}
	return out
}

// commitFilesPerPage is how many files GitHub lists on each page of a
// commit, when asked for no other size.
const commitFilesPerPage = 300

// ListCommits returns a page of the history of ref, newest first, as git
// log walks it. Ref is a branch, a tag or a commit SHA, and empty means the
// default branch. Cursor and perPage work as in ListBranches. If cond is
// current, the Response has NotModified set and the page is empty. An empty
// repository, which GitHub answers with 409, has an empty history.
//
// The Next of the first page names its first commit, where ref pointed,
// rather than ref, so later pages continue that history even after ref
// moves, and a cursor names the same commits for good.
func (c *Client) ListCommits(ctx context.Context, repo core.RepoRef, ref, cursor string, perPage int, cond Conditional) (core.Page[core.Commit], Response, error) {
	path := cursor
	if path == "" {
		q := url.Values{}
		if ref != "" {
			q.Set("sha", ref)
		}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		path = commitsPath(repo)
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
	}
	var items []restCommit
	res, err := c.Get(ctx, path, cond, &items)
	switch {
	case emptyRepo(err):
		return core.Page[core.Commit]{}, res, nil
	case err != nil:
		return core.Page[core.Commit]{}, res, fmt.Errorf("list commits: %w", err)
	case res.NotModified:
		return core.Page[core.Commit]{}, res, nil
	}
	page := core.Page[core.Commit]{Items: convert(items, restCommit.core), Next: res.Next}
	if cursor == "" && len(page.Items) > 0 {
		page.Next = pinCommits(page.Next, page.Items[0].SHA)
	}
	return page, res, nil
}

// pinCommits returns the cursor next with the history it lists started at
// sha.
func pinCommits(next, sha string) string {
	if next == "" {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil {
		return next
	}
	q := u.Query()
	q.Set("sha", sha)
	u.RawQuery = q.Encode()
	return u.String()
}

// emptyRepo reports whether err is how GitHub refuses to list the commits
// of a repository that has none.
func emptyRepo(err error) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && e.StatusCode == http.StatusConflict
}

// GetCommit returns a commit with its stats and the first page of the
// files it changed, with their patches. A commit named by its SHA never
// changes, so there is no conditional request. Pass FilesNext to
// ListCommitFiles for the rest of the files.
func (c *Client) GetCommit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitDetail, error) {
	var d restCommitDetail
	res, err := c.Get(ctx, commitPath(repo, sha), Conditional{}, &d)
	if err != nil {
		return core.CommitDetail{}, fmt.Errorf("get commit %s: %w", sha, err)
	}
	return core.CommitDetail{
		Commit: d.core(),
		Stats: core.CommitStats{
			Additions: d.Stats.Additions,
			Deletions: d.Stats.Deletions,
			Total:     d.Stats.Total,
		},
		Files:          convert(d.Files, commitFile.core),
		FilesNext:      res.Next,
		FilesTruncated: filesTruncated(res),
	}, nil
}

// ListCommitFiles returns a page of the files that the commit sha changed.
// Cursor is the FilesNext of GetCommit, or the Next of the previous page;
// empty reads the first page. Each page is a request for the whole commit,
// of which only the files are kept.
func (c *Client) ListCommitFiles(ctx context.Context, repo core.RepoRef, sha, cursor string) (core.Page[core.CommitFile], error) {
	path := cursor
	if path == "" {
		path = commitPath(repo, sha)
	}
	var d struct {
		Files []commitFile `json:"files"`
	}
	res, err := c.Get(ctx, path, Conditional{}, &d)
	if err != nil {
		return core.Page[core.CommitFile]{}, fmt.Errorf("list files of commit %s: %w", sha, err)
	}
	return core.Page[core.CommitFile]{Items: convert(d.Files, commitFile.core), Next: res.Next}, nil
}

// filesTruncated reports whether the pages of files that res starts reach
// the most GitHub lists, so that files are missing. GitHub doesn't say how
// many files a commit changed, so a commit of exactly that many reads as
// truncated too.
func filesTruncated(res Response) bool {
	if res.Last == "" {
		return false
	}
	u, err := url.Parse(res.Last)
	if err != nil {
		return false
	}
	q := u.Query()
	last, err := strconv.Atoi(q.Get("page"))
	if err != nil {
		return false
	}
	size := commitFilesPerPage
	if n, err := strconv.Atoi(q.Get("per_page")); err == nil && n > 0 {
		size = min(n, commitFilesPerPage)
	}
	return last*size >= core.MaxCommitFiles
}

// restCompare is the part of a comparison that says how two commits
// relate.
type restCompare struct {
	Status   string `json:"status"`
	AheadBy  int    `json:"ahead_by"`
	BehindBy int    `json:"behind_by"`
}

// Compare returns how far head is from base. Each is a branch, a tag or a
// commit SHA, and head may name a fork's branch as owner:branch. If cond
// is current, the Response has NotModified set and the comparison is
// empty.
//
// It reads the second page of one commit: GitHub lists the changed files
// only on the first, and the counts on every page, so the response stays
// small however far apart the commits are.
func (c *Client) Compare(ctx context.Context, repo core.RepoRef, base, head string, cond Conditional) (core.Compare, Response, error) {
	path := commitsBase(repo) + "/compare/" + url.PathEscape(base) + "..." + url.PathEscape(head) + "?per_page=1&page=2"
	var cmp restCompare
	res, err := c.Get(ctx, path, cond, &cmp)
	if err != nil {
		return core.Compare{}, res, fmt.Errorf("compare %s...%s: %w", base, head, err)
	}
	if res.NotModified {
		return core.Compare{}, res, nil
	}
	return core.Compare{Status: core.CompareStatus(cmp.Status), AheadBy: cmp.AheadBy, BehindBy: cmp.BehindBy}, res, nil
}

func commitsBase(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name)
}

func commitsPath(repo core.RepoRef) string {
	return commitsBase(repo) + "/commits"
}

func commitPath(repo core.RepoRef, sha string) string {
	return commitsPath(repo) + "/" + url.PathEscape(sha)
}
