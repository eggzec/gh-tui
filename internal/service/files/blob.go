package files

import (
	"context"
	"fmt"
	"strings"

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
// false if the content isn't in memory; it doesn't read the store.
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
		if b, ok := s.storedBlob(q.SHA); ok {
			return cache.Entry[core.Blob]{Value: b}, nil
		}
		b, err := s.api.GetBlob(ctx, q.Repo, q.SHA, s.maxBlob)
		if err != nil {
			return cache.Entry[core.Blob]{}, err
		}
		s.keepBlob(b)
		return cache.Entry[core.Blob]{Value: b}, nil
	})
	if err != nil {
		return core.Blob{}, fmt.Errorf("get blob %s of %s: %w", q.SHA, q.Repo, err)
	}
	return e.Value, nil
}

// storedBlob returns the blob named sha from the store. The name is the
// hash of the content, so content that doesn't match it is damaged: it is
// removed and read from GitHub again. The store is shared by every
// repository, which is safe for the same reason: a blob of that name is the
// same content wherever it is.
func (s *Service) storedBlob(sha string) (core.Blob, bool) {
	if !isSHA(sha) {
		return core.Blob{}, false
	}
	sha = strings.ToLower(sha)
	b, ok := s.store.Get(kindBlob, sha)
	if !ok {
		return core.Blob{}, false
	}
	if !isBlobID(sha, b) {
		s.store.Delete(kindBlob, sha)
		return core.Blob{}, false
	}
	if int64(len(b)) > s.maxBlob {
		// Kept by a session with a higher limit.
		return core.Blob{}, false
	}
	return core.Blob{SHA: sha, Size: int64(len(b)), Content: b, Binary: core.LooksBinary(b)}, true
}

// keepBlob puts b in the store if its content matches its name, so the
// store never holds content under the wrong name.
func (s *Service) keepBlob(b core.Blob) {
	sha := strings.ToLower(b.SHA)
	if isSHA(sha) && isBlobID(sha, b.Content) {
		_ = s.store.Put(kindBlob, sha, b.Content)
	}
}
