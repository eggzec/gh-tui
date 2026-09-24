package files

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// BlobQuery selects the content of a file.
type BlobQuery struct {
	Repo core.RepoRef
	SHA  string
	// Size is the size from the file's tree entry, if known. A file known
	// to be too large then fails without a request. It is not part of the
	// cache key.
	Size int64
}

// CachedBlob returns the cached content for q without a request. It reports
// false if the content isn't cached.
func (s *Service) CachedBlob(q BlobQuery) (core.Blob, bool) {
	e, st := s.blobs.Get(blobKey(q.Repo, q.SHA))
	return e.Value, st != cache.Miss
}

// Blob returns the content of a file. Content that isn't text is returned
// with Binary set. A file larger than the size limit fails with a
// *core.TooLargeError, which matches core.ErrTooLarge, so the tui can offer
// to open it in the browser instead.
func (s *Service) Blob(ctx context.Context, q BlobQuery) (core.Blob, error) {
	if q.Size > s.maxBlob {
		return core.Blob{}, fmt.Errorf("get blob %s of %s: %w", q.SHA, q.Repo, &core.TooLargeError{Size: q.Size, Limit: s.maxBlob})
	}
	e, err := s.blobs.Fetch(ctx, blobKey(q.Repo, q.SHA), func(ctx context.Context, _ cache.Entry[core.Blob], _ bool) (cache.Entry[core.Blob], error) {
		b, err := s.api.GetBlob(ctx, q.Repo, q.SHA, s.maxBlob)
		if err != nil {
			return cache.Entry[core.Blob]{}, err
		}
		return cache.Entry[core.Blob]{Value: b}, nil
	})
	if err != nil {
		return core.Blob{}, fmt.Errorf("get blob %s of %s: %w", q.SHA, q.Repo, err)
	}
	return e.Value, nil
}
