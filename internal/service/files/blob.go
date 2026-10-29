package files

import (
	"context"
	"errors"
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
	// Limit raises the size limit for this read above the warning limit,
	// up to the hard limit. 0 keeps the warning limit. It is not part of the
	// cache key.
	Limit int64
}

// limit returns the size above which q is refused, and whether a larger
// limit may be asked for.
func (s *Service) limit(q BlobQuery) (lim int64, warn bool) {
	lim = s.warnBlob
	if q.Limit > lim {
		lim = min(q.Limit, s.maxBlob)
	}
	return lim, lim < s.maxBlob
}

// CachedBlob returns the cached content for q without a request. It reports
// false if the content isn't in memory, or is over the limit of q; it doesn't
// read the store.
func (s *Service) CachedBlob(q BlobQuery) (core.Blob, bool) {
	e, st := s.blobs.Get(blobKey(q.Repo, q.SHA))
	if st == cache.Miss {
		return core.Blob{}, false
	}
	lim, _ := s.limit(q)
	if int64(len(e.Value.Content)) > lim {
		return core.Blob{}, false
	}
	return e.Value, true
}

// Blob returns the content of a file. Content that isn't text is returned
// with Binary set. A file larger than the size limit fails with a
// *core.TooLargeError, which matches core.ErrTooLarge, so the tui can offer
// to open it in the browser instead.
func (s *Service) Blob(ctx context.Context, q BlobQuery) (core.Blob, error) {
	lim, warn := s.limit(q)
	tooLarge := func(size int64) error {
		return fmt.Errorf("get blob %s of %s: %w", q.SHA, q.Repo, &core.TooLargeError{Size: size, Limit: lim, Warn: warn})
	}
	if q.Size > lim {
		return core.Blob{}, tooLarge(q.Size)
	}
	e, err := s.blobs.Fetch(ctx, blobKey(q.Repo, q.SHA), func(ctx context.Context, _ cache.Entry[core.Blob], _ bool) (cache.Entry[core.Blob], error) {
		if b, ok := s.storedBlob(q.SHA, lim); ok {
			return cache.Entry[core.Blob]{Value: b}, nil
		}
		b, err := s.api.GetBlob(ctx, q.Repo, q.SHA, lim)
		if err != nil {
			return cache.Entry[core.Blob]{}, err
		}
		s.keepBlob(b)
		return cache.Entry[core.Blob]{Value: b}, nil
	})
	if err != nil {
		if tl, ok := errors.AsType[*core.TooLargeError](err); ok {
			tl.Warn = warn
		}
		return core.Blob{}, fmt.Errorf("get blob %s of %s: %w", q.SHA, q.Repo, err)
	}
	if int64(len(e.Value.Content)) > lim {
		// Cached by a read that asked for a higher limit.
		return core.Blob{}, tooLarge(int64(len(e.Value.Content)))
	}
	return e.Value, nil
}

// storedBlob returns the blob named sha from the store. The name is the
// hash of the content, so content that doesn't match it is damaged: it is
// removed and read from GitHub again. The store is shared by every
// repository, which is safe for the same reason: a blob of that name is the
// same content wherever it is.
func (s *Service) storedBlob(sha string, limit int64) (core.Blob, bool) {
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
	if int64(len(b)) > limit {
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
