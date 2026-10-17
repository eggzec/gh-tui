package pulls

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// diffSource reads the files of a diff once, in tree order, and serves them to
// every diff view made over it, so that switching between a file and the whole
// patch reads nothing again.
type diffSource struct {
	// load reads the files; it is not asked again after it succeeded.
	load func(ctx context.Context) ([]core.CommitFile, error)

	// loading lets one read at a time call load, and is held across it. mu only
	// guards the fields below and is never held across a read, since paths is
	// called from View and Update.
	loading sync.Mutex

	mu     sync.Mutex
	files  []core.CommitFile
	order  []string
	loaded bool
}

// newDiffSource returns a source that reads with load.
func newDiffSource(load func(ctx context.Context) ([]core.CommitFile, error)) *diffSource {
	return &diffSource{load: load}
}

// keptDiffSource returns a source that already has fs.
func keptDiffSource(fs []core.CommitFile) *diffSource {
	s := &diffSource{}
	s.keep(fs)
	return s
}

// keep stores fs, sorted by path, which is the order the diff shows them in.
func (s *diffSource) keep(fs []core.CommitFile) {
	fs = slices.Clone(fs)
	slices.SortStableFunc(fs, func(a, b core.CommitFile) int { return strings.Compare(a.Path, b.Path) })
	order := diffview.Order(fs)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files, s.order, s.loaded = fs, order, true
}

// kept returns the files and whether they are read.
func (s *diffSource) kept() ([]core.CommitFile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.files, s.loaded
}

// read returns the files, reading them the first time.
func (s *diffSource) read(ctx context.Context) ([]core.CommitFile, error) {
	if fs, ok := s.kept(); ok {
		return fs, nil
	}
	s.loading.Lock()
	defer s.loading.Unlock()
	// Another read may have finished while this one waited.
	if fs, ok := s.kept(); ok {
		return fs, nil
	}
	fs, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	s.keep(fs)
	fs, _ = s.kept()
	return fs, nil
}

// paths returns the paths of the files in tree order, or nil until they are
// read.
func (s *diffSource) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order
}

// fetch returns what a diff view reads: the files of the range as one page, or
// only the file at path when path is not empty.
func (s *diffSource) fetch(path string) diff.Fetch {
	return func(ctx context.Context, _ string) ([]diff.File, string, error) {
		fs, err := s.read(ctx)
		if err != nil {
			return nil, "", err
		}
		if path != "" {
			fs = slices.DeleteFunc(slices.Clone(fs), func(f core.CommitFile) bool { return f.Path != path })
		}
		return diffFiles(fs), "", nil
	}
}

// diffSource returns where the diff of the range of the Files pane reads its
// files. The files that the pane has read serve it when they carry their
// patches. Otherwise they are read again: a part of the commits compares its
// two ends by their full ids, and every commit reads the files of the pull
// request at its head. Without a range yet, as when the finder chose a file
// before the commits were read, the head of the pull request is read.
func (m *panesModal) diffSource() *diffSource {
	if _, _, fs, ok := m.filesRange(); ok && holdsPatches(fs) {
		return keptDiffSource(fs)
	}
	svc, repo, number := m.svc, m.repo, m.number
	base, head, all, ok := m.rangeEnds()
	if !ok || all {
		if !ok {
			head = m.detail.HeadSHA
		}
		return newDiffSource(func(ctx context.Context) ([]core.CommitFile, error) {
			return pullFiles(ctx, svc, repo, number, head)
		})
	}
	return newDiffSource(func(ctx context.Context) ([]core.CommitFile, error) {
		d, err := svc.RangeFiles(ctx, pulls.RangeQuery{Repo: repo, Base: base, Head: head})
		return d.Files, err
	})
}

// holdsPatches reports whether any of the files carries its patch.
func holdsPatches(fs []core.CommitFile) bool {
	return slices.ContainsFunc(fs, func(f core.CommitFile) bool { return f.Patch != "" })
}

// diffFiles returns the files as the diff shows them.
func diffFiles(items []core.CommitFile) []diff.File {
	files := make([]diff.File, len(items))
	for i, f := range items {
		files[i] = diff.File{
			Path: f.Path, OldPath: f.PreviousPath, Status: diff.Status(f.Status), SHA: f.SHA,
			Additions: f.Additions, Deletions: f.Deletions, Patch: f.Patch, Truncated: f.PatchTruncated,
		}
	}
	return files
}
