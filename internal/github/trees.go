package github

import (
	"context"
	"fmt"
	"net/url"
	"path"

	"github.com/eggzec/gh-tui/internal/core"
)

// Trees and blobs use the REST git database API: it lists one directory at a
// time by SHA, which suits a tree that expands lazily, and answers
// conditional requests, which GraphQL doesn't.

type restTree struct {
	SHA       string          `json:"sha"`
	Tree      []restTreeEntry `json:"tree"`
	Truncated bool            `json:"truncated"`
}

type restTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	// Size is left out for trees and submodules, which decodes to 0.
	Size int64 `json:"size"`
}

func (e restTreeEntry) core() core.TreeEntry {
	return core.TreeEntry{
		Path: e.Path,
		Name: path.Base(e.Path),
		Type: core.EntryType(e.Type),
		Mode: e.Mode,
		SHA:  e.SHA,
		Size: e.Size,
	}
}

func (t restTree) core() core.Tree {
	return core.Tree{SHA: t.SHA, Entries: convert(t.Tree, restTreeEntry.core), Truncated: t.Truncated}
}

// GetTree returns the entries of one level of a git tree. Tree is the tree's
// SHA, a commit SHA, or a ref such as a branch, a tag or HEAD, which lists
// the root of the commit it points at. If cond is current, the Response has
// NotModified set and the tree is empty.
func (c *Client) GetTree(ctx context.Context, repo core.RepoRef, tree string, cond Conditional) (core.Tree, Response, error) {
	return c.getTree(ctx, treePath(repo, tree), cond)
}

// GetTreeRecursive returns every entry below a git tree, with paths relative
// to it. Tree is named as in GetTree. GitHub stops at 100,000 entries or
// 7 MB and then reports the tree as Truncated.
func (c *Client) GetTreeRecursive(ctx context.Context, repo core.RepoRef, tree string, cond Conditional) (core.Tree, Response, error) {
	return c.getTree(ctx, treePath(repo, tree)+"?recursive=1", cond)
}

func (c *Client) getTree(ctx context.Context, endpoint string, cond Conditional) (core.Tree, Response, error) {
	var t restTree
	res, err := c.Get(ctx, endpoint, cond, &t)
	if err != nil {
		return core.Tree{}, res, fmt.Errorf("get tree: %w", err)
	}
	if res.NotModified {
		return core.Tree{}, res, nil
	}
	return t.core(), res, nil
}

// GetBlob returns the content of the blob sha. A blob larger than limit
// bytes is not read, and fails with a *core.TooLargeError, which matches
// core.ErrTooLarge. The blob's Binary field reports content that isn't text.
// Blobs never change, so there is no conditional request.
func (c *Client) GetBlob(ctx context.Context, repo core.RepoRef, sha string, limit int64) (core.Blob, error) {
	b, err := c.getRaw(ctx, gitPath(repo)+"/blobs/"+url.PathEscape(sha), limit)
	if err != nil {
		return core.Blob{}, fmt.Errorf("get blob %s: %w", sha, err)
	}
	return core.Blob{SHA: sha, Size: int64(len(b)), Content: b, Binary: core.LooksBinary(b)}, nil
}

// treePath escapes tree, so a branch such as feat/x stays one path segment.
func treePath(repo core.RepoRef, tree string) string {
	return gitPath(repo) + "/trees/" + url.PathEscape(tree)
}

func gitPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/git"
}
