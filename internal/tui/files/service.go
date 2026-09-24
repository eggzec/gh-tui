package files

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
)

// Service is what the section needs from the files service.
type Service interface {
	// CachedTree returns a cached directory without a request.
	CachedTree(q filesvc.TreeQuery) (core.Tree, bool)
	Tree(ctx context.Context, q filesvc.TreeQuery) (core.Tree, error)
	// CachedAll returns the cached recursive listing without a request.
	CachedAll(q filesvc.TreeQuery) (core.Tree, bool)
	// All returns every entry of a tree in one request. A very large tree
	// comes back Truncated.
	All(ctx context.Context, q filesvc.TreeQuery) (core.Tree, error)
	// CachedBlob returns the cached content of a file without a request.
	CachedBlob(q filesvc.BlobQuery) (core.Blob, bool)
	// Blob returns the content of a file. A file too large to read fails
	// with an error that matches core.ErrTooLarge.
	Blob(ctx context.Context, q filesvc.BlobQuery) (core.Blob, error)
	// Invalidate marks the trees and listings that the refs of repo point
	// at stale, so the next read asks GitHub whether they moved.
	Invalidate(repo core.RepoRef)
}
