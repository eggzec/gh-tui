package files

import (
	"context"
	"path"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
)

// Reading top-level files ahead costs a request each, so it is bounded.
const (
	// prefetchFiles is the most top-level files read ahead per listing.
	prefetchFiles = 32
	// prefetchWorkers is the most reads in flight at once.
	prefetchWorkers = 4
)

// prefetchTop reads the small top-level files of x ahead of the preview,
// into the service's cache. Their contents come back through CachedBlob, so
// the command reports nothing. The reads stop when ctx is done.
func (s *Section) prefetchTop(ctx context.Context, x *index) tea.Cmd {
	if s.prefetchMax <= 0 {
		return nil
	}
	var todo []filesvc.BlobQuery
	for _, e := range x.dirs[""] {
		if len(todo) == prefetchFiles {
			break
		}
		q := filesvc.BlobQuery{Repo: s.repo, SHA: e.SHA, Size: e.Size}
		if !worthReading(e, s.prefetchMax) {
			continue
		}
		if _, ok := s.svc.CachedBlob(q); ok {
			continue
		}
		todo = append(todo, q)
	}
	if len(todo) == 0 {
		return nil
	}
	svc := s.svc
	return func() tea.Msg {
		readAll(ctx, svc, todo, prefetchWorkers)
		return nil
	}
}

// readAll reads the blobs of qs with at most workers reads in flight, and
// ignores failures: the preview reports them if the file is opened.
func readAll(ctx context.Context, svc Service, qs []filesvc.BlobQuery, workers int) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, q := range qs {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		wg.Go(func() {
			defer func() { <-sem }()
			_, _ = svc.Blob(ctx, q)
		})
	}
}

// worthReading reports whether the file of e is worth reading ahead: a
// regular file of at most limit bytes that is likely text. Submodules have
// no content here, and the preview of a link shows only its target.
func worthReading(e core.TreeEntry, limit int64) bool {
	return e.Type == core.EntryBlob && !e.Symlink() && e.Size <= limit && !binaryExt[strings.ToLower(path.Ext(e.Name))]
}

// binaryExt holds the extensions of files that are rarely text, which the
// preview would only name as binary.
var binaryExt = map[string]bool{
	// Images.
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".ico": true,
	".icns": true, ".webp": true, ".tif": true, ".tiff": true, ".psd": true, ".avif": true, ".heic": true,
	// Archives and packages.
	".zip": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true, ".zst": true, ".7z": true,
	".rar": true, ".tar": true, ".jar": true, ".war": true, ".whl": true, ".deb": true, ".rpm": true,
	".dmg": true, ".iso": true, ".apk": true,
	// Fonts.
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	// Media.
	".mp3": true, ".mp4": true, ".m4a": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true,
	".wav": true, ".flac": true, ".ogg": true,
	// Compiled code and data.
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true, ".o": true, ".class": true,
	".pyc": true, ".wasm": true, ".bin": true, ".dat": true, ".db": true, ".sqlite": true, ".pdf": true,
}
