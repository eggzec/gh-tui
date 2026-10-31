package github

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"

	"github.com/eggzec/gh-tui/internal/core"
)

// restVerifyIssue is the part of an issue or pull request that says where it
// lives and which it is. It has no type: an issue's "type" is an object.
type restVerifyIssue struct {
	HTMLURL     string    `json:"html_url"`
	PullRequest *struct{} `json:"pull_request"`
}

// restVerifyUser is the part of a user or organization that says where it
// lives and which it is.
type restVerifyUser struct {
	HTMLURL string `json:"html_url"`
	Type    string `json:"type"`
}

// restVerifyCommit is the part of a commit that says where it lives.
type restVerifyCommit struct {
	HTMLURL string `json:"html_url"`
}

// verify gets path and reports whether it exists. A 404 is an answer, not an
// error, and so is a status in miss. A 304 returns the Response alone, to be
// read as unchanged. read gives the address and kind of what was found.
func verify[T any](ctx context.Context, c *Client, path string, cond Conditional, miss []int, read func(T) (string, core.VerifiedKind)) (core.Verified, Response, error) {
	var v T
	res, err := c.Get(ctx, path, cond, &v)
	if errors.Is(err, core.ErrNotFound) {
		return core.Verified{}, res, nil
	}
	var he *Error
	if errors.As(err, &he) && slices.Contains(miss, he.StatusCode) {
		return core.Verified{}, res, nil
	}
	if err != nil || res.NotModified {
		return core.Verified{}, res, err
	}
	u, kind := read(v)
	return core.Verified{Exists: true, URL: u, Kind: kind}, res, nil
}

// VerifyIssue reports whether number is an issue or a pull request of repo,
// which one it is, and its address.
func (c *Client) VerifyIssue(ctx context.Context, repo core.RepoRef, number int, cond Conditional) (core.Verified, Response, error) {
	return verify(ctx, c, issuePath(repo, number), cond, nil, func(v restVerifyIssue) (string, core.VerifiedKind) {
		if v.PullRequest != nil {
			return v.HTMLURL, core.VerifiedPull
		}
		return v.HTMLURL, core.VerifiedIssue
	})
}

// VerifyUser reports whether login is a user or an organization, which one
// it is, and its address.
func (c *Client) VerifyUser(ctx context.Context, login string, cond Conditional) (core.Verified, Response, error) {
	return verify(ctx, c, "/users/"+url.PathEscape(login), cond, nil, func(v restVerifyUser) (string, core.VerifiedKind) {
		if v.Type == "Organization" {
			return v.HTMLURL, core.VerifiedOrg
		}
		return v.HTMLURL, core.VerifiedUser
	})
}

// VerifyCommit reports whether sha, in full or abbreviated, names a commit
// of repo, and its address. For a hash that names none, or an abbreviation
// that names several, GitHub answers 422 rather than 404.
func (c *Client) VerifyCommit(ctx context.Context, repo core.RepoRef, sha string, cond Conditional) (core.Verified, Response, error) {
	path := "/repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/commits/" + url.PathEscape(sha)
	return verify(ctx, c, path, cond, []int{http.StatusUnprocessableEntity}, func(v restVerifyCommit) (string, core.VerifiedKind) {
		return v.HTMLURL, core.VerifiedCommit
	})
}
