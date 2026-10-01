package files

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// prefetch is what the section reads ahead, as the prefetch settings of
// the files tree and the finder resolve.
type prefetch struct {
	// preview reads the content of the files around the tree's cursor,
	// tree the listings of its folders, and finder the content of the
	// files around the finder's cursor.
	preview, tree, finder config.Resolved
	// previewMax and finderMax are the largest files read around the
	// cursors, and cursorMax the largest under them: the file under the
	// cursor is likely opened next, or the finder shows it, so it is read
	// up to the size the preview reads.
	previewMax, finderMax, cursorMax int64
}

// newPrefetch resolves the settings of the files tree and the finder in
// p, where previewMax is the largest file the preview reads.
func newPrefetch(p config.PrefetchLayers, previewMax config.Size) prefetch {
	var r prefetch
	// The names are those of the settings, so these can't fail.
	r.preview, _ = p.Resolve("files", "preview")
	r.tree, _ = p.Resolve("files", "tree")
	r.finder, _ = p.Resolve("finder", "preview")
	r.previewMax, r.finderMax = int64(p.Files.Preview.MaxSize), int64(p.Finder.Preview.MaxSize)
	r.cursorMax = int64(previewMax)
	return r
}

// noPrefetch reads nothing ahead, but for the file under the finder's
// cursor, which the finder shows, after the default rest.
func noPrefetch() prefetch {
	d := config.Default()
	p := newPrefetch(d.Prefetch, d.Files.Preview.MaxSize)
	p.preview.Enabled, p.tree.Enabled, p.finder.Enabled = false, false, false
	return p
}

// fileAhead returns reads ahead of the content of files, as r says.
func (s *Section) fileAhead(r config.Resolved) *ui.Ahead[filesvc.BlobQuery] {
	svc := s.svc
	a := ui.NewAhead("file",
		func(ctx context.Context, q filesvc.BlobQuery) error {
			_, err := svc.Blob(ctx, q)
			return err
		},
		func(q filesvc.BlobQuery) bool {
			_, ok := svc.CachedBlob(q)
			return ok
		}, 0, 0)
	a.Configure(r)
	return a
}

// newAheads makes the reads ahead of the tree: of the content of files,
// and of the listings of folders.
func (s *Section) newAheads() {
	svc := s.svc
	s.ahead = s.fileAhead(s.prefetch.preview)
	s.dirs = ui.NewAhead("dir",
		func(ctx context.Context, q filesvc.TreeQuery) error {
			_, err := svc.Tree(ctx, q)
			return err
		},
		func(q filesvc.TreeQuery) bool {
			_, ok := svc.CachedTree(q)
			return ok
		}, 0, 0)
	s.dirs.Configure(s.prefetch.tree)
}

// readAhead reads, once the cursor rests, the files in a window around it
// in the order of the tree, and the listings of the folders in a window
// when the listing of the repository was too large to read at once; a
// whole listing expands its folders without requests.
func (s *Section) readAhead() tea.Cmd {
	i := s.tree.Index()
	// Until the tree shows rows, it has no window.
	files, dirs := s.fileAt, s.dirAt
	if s.tree.Len() == 0 {
		files, dirs = nil, nil
	}
	if s.idx == nil || !s.idx.truncated {
		dirs = nil
	}
	return tea.Batch(s.ahead.Window(files, i), s.dirs.Window(dirs, i))
}

// fileAt returns the blob of the file at row i of the tree, if it is
// worth reading ahead.
func (s *Section) fileAt(i int) (filesvc.BlobQuery, bool) {
	n, ok := s.tree.At(i)
	e, isEntry := entryOf(n)
	limit := s.prefetch.previewMax
	if i == s.tree.Index() {
		limit = s.prefetch.cursorMax
	}
	if !ok || !isEntry || !worthReading(e, limit) {
		return filesvc.BlobQuery{}, false
	}
	return s.blobQuery(e), true
}

// dirAt returns the listing of the folder at row i of the tree, if it is
// a folder.
func (s *Section) dirAt(i int) (filesvc.TreeQuery, bool) {
	n, ok := s.tree.At(i)
	e, isEntry := entryOf(n)
	if !ok || !isEntry || !e.Dir() {
		return filesvc.TreeQuery{}, false
	}
	return filesvc.TreeQuery{Repo: s.repo, Ref: e.SHA}, true
}

// resetAheads cancels the reads ahead of the tree shown, for a new tree
// whose reads ctx bounds.
func (s *Section) resetAheads(ctx context.Context) {
	s.ahead.Reset(ctx)
	s.dirs.Reset(ctx)
	// Another repository may not be rate limited.
	s.ahead.Resume()
	s.dirs.Resume()
}

// readBlob reads the blob of q ahead, records what came of it, and returns
// it. A failure is for the preview to report, if the file is opened.
func readBlob(ctx context.Context, svc Service, seen *obs.Prefetched[filesvc.BlobQuery], q filesvc.BlobQuery) (core.Blob, error) {
	seen.Count(obs.PrefetchSent)
	start := time.Now()
	b, err := svc.Blob(ctx, q)
	outcome := "read"
	switch {
	case err == nil:
		seen.Read(q)
	case errors.Is(err, core.ErrRateLimited):
		outcome = "rate_limited"
		seen.Count(obs.PrefetchRateLimited)
	case ctx.Err() != nil:
		outcome = "canceled"
		seen.Count(obs.PrefetchCanceled)
	default:
		outcome = "failed"
		seen.Count(obs.PrefetchFailed)
	}
	if obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "prefetch read", "span", "prefetch", "kind", seen.Kind(), "sha", q.SHA,
			"size", q.Size, "outcome", outcome, "duration_ms", obs.Millis(time.Since(start)))
	}
	return b, err
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

func (s *Section) blobQuery(e core.TreeEntry) filesvc.BlobQuery {
	return filesvc.BlobQuery{Repo: s.repo, SHA: e.SHA, Size: e.Size}
}
