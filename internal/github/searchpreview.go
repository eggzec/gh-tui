package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// What the search preview of a user, an organization or a commit reads beyond
// the hit.

// diffAccept asks for a commit as one unified diff.
const diffAccept = "application/vnd.github.diff"

type restProfile struct {
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	Bio       string    `json:"bio"`
	Followers int       `json:"followers"`
	CreatedAt time.Time `json:"created_at"`
}

// GetUserProfile returns the profile of the user or organization login.
func (c *Client) GetUserProfile(ctx context.Context, login string) (core.UserProfile, error) {
	var r restProfile
	if _, err := c.Get(ctx, "users/"+url.PathEscape(login), Conditional{}, &r); err != nil {
		return core.UserProfile{}, fmt.Errorf("profile of %s: %w", login, err)
	}
	return core.UserProfile{Login: r.Login, Name: r.Name, Bio: r.Bio, Followers: r.Followers, CreatedAt: r.CreatedAt}, nil
}

// CommitPatch returns the whole patch of the commit sha. A patch over
// core.MaxCommitPatch is not read: the text is empty and tooLarge is set.
func (c *Client) CommitPatch(ctx context.Context, repo core.RepoRef, sha string) (patch string, tooLarge bool, err error) {
	b, _, err := c.getMedia(ctx, commitPath(repo, sha), diffAccept, Conditional{}, core.MaxCommitPatch)
	if _, ok := errors.AsType[*core.TooLargeError](err); ok {
		return "", true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("patch of %s: %w", sha, err)
	}
	return string(b), false, nil
}
