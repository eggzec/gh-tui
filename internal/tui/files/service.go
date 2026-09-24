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
	// CachedBlob returns the cached content of a file without a request.
	CachedBlob(q filesvc.BlobQuery) (core.Blob, bool)
	// Blob returns the content of a file. A file too large to read fails
	// with an error that matches core.ErrTooLarge.
	Blob(ctx context.Context, q filesvc.BlobQuery) (core.Blob, error)
	// Invalidate marks the trees that the refs of repo point at stale, so
	// the next read asks GitHub whether they moved.
	Invalidate(repo core.RepoRef)
}
