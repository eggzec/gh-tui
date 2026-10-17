package pulls

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// patched returns changedFiles with a patch each, which the diff can show.
func patched() []core.CommitFile {
	fs := changedFiles()
	for i := range fs {
		fs[i].Patch = "@@ -1,2 +1,3 @@\n keep\n-old " + fs[i].Path + "\n+new " + fs[i].Path + "\n+more\n"
	}
	return fs
}

// longOID is a full commit id made of c.
func longOID(c string) string { return strings.Repeat(c, 40) }

// longCommits are the commits of sampleCommits with full ids.
func longCommits() core.PullCommits {
	pc := sampleCommits(nil)
	for i := range pc.Commits {
		pc.Commits[i].OID = longOID(string(rune('a' + i)))
	}
	return pc
}

// fullSHAs serves the compare of a range only for the full ids of its ends,
// as GitHub does.
type fullSHAs struct{ Service }

func (f fullSHAs) RangeFiles(ctx context.Context, q pulls.RangeQuery) (core.CompareDetail, error) {
	if len(q.Base) != 40 || len(q.Head) != 40 {
		return core.CompareDetail{}, errors.New("short sha in " + q.Base + "..." + q.Head)
	}
	return f.Service.RangeFiles(ctx, q)
}

// readAll reads the one page that a diff source serves.
func readAll(tb testing.TB, f diff.Fetch) []diff.File {
	tb.Helper()
	got, next, err := f(context.Background(), "")
	if err != nil || next != "" {
		tb.Fatalf("fetch = %v, %q, %v; want one page", got, next, err)
	}
	return got
}

func diffPaths(fs []diff.File) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Path)
	}
	return out
}

func TestDiffSourceServesTheFilesOfThePane(t *testing.T) {
	t.Parallel()
	_, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() })
	reads := len(svc.fileReads)
	src := m.diffSource()
	if got, want := diffPaths(readAll(t, src.fetch(""))), []string{"a.go", "docs/c.md", "internal/b.go"}; !slices.Equal(got, want) {
		t.Errorf("whole patch = %v, want the files in tree order %v", got, want)
	}
	if got := diffPaths(readAll(t, src.fetch("docs/c.md"))); !slices.Equal(got, []string{"docs/c.md"}) {
		t.Errorf("one file = %v, want only docs/c.md", got)
	}
	if got := src.paths(); !slices.Equal(got, []string{"a.go", "docs/c.md", "internal/b.go"}) {
		t.Errorf("paths = %v, want tree order", got)
	}
	if len(svc.fileReads) != reads {
		t.Errorf("files were read again: %+v", svc.fileReads[reads:])
	}
}

func TestDiffSourceReadsAgainWithoutPatches(t *testing.T) {
	t.Parallel()
	_, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = changedFiles() })
	reads := len(svc.fileReads)
	src := m.diffSource()
	if got := src.paths(); got != nil {
		t.Errorf("paths = %v before the read, want none", got)
	}
	readAll(t, src.fetch(""))
	readAll(t, src.fetch("a.go"))
	if n := len(svc.fileReads) - reads; n != 1 || svc.fileReads[reads].Head != "ddd4444" {
		t.Errorf("file reads = %+v, want one at the head", svc.fileReads[reads:])
	}
	if got := src.paths(); len(got) != 3 {
		t.Errorf("paths = %v after the read, want the three files", got)
	}
}

func TestDiffSourceReadsTheHeadWithoutARange(t *testing.T) {
	t.Parallel()
	_, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = changedFiles() })
	m.files.loaded = false
	reads := len(svc.fileReads)
	if got := diffPaths(readAll(t, m.diffSource().fetch(""))); len(got) != 3 {
		t.Errorf("files = %v, want the three the pull request changes", got)
	}
	if len(svc.fileReads) != reads+1 || svc.fileReads[reads].Head != m.detail.HeadSHA {
		t.Errorf("file reads = %+v, want one at the head of the pull request", svc.fileReads[reads:])
	}
}

func TestDiffSubRangeComparesFullSHAs(t *testing.T) {
	t.Parallel()
	h, m, svc := rangePanes(t, longCommits(), func(svc *fakeService) {
		svc.changed = patched()
		svc.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: changedFiles()}}
	})
	m.svc = fullSHAs{m.svc}
	m.files.rng = files.Range{First: 1, Last: 2}
	drain(t, h, m.readRange())
	if _, _, _, ok := m.filesRange(); !ok {
		t.Fatalf("the range was not read: %+v", m.files.read)
	}
	got := diffPaths(readAll(t, m.diffSource().fetch("")))
	if len(got) != 3 {
		t.Errorf("files = %v, want the three the range changes", got)
	}
	reads := svc.rangeFiles.reads
	if len(reads) < 2 {
		t.Fatalf("compare reads = %+v, want the pane's read and the diff's", reads)
	}
	if last := reads[len(reads)-1]; last.Base != longOID("a") || last.Head != longOID("c") {
		t.Errorf("the diff compared %q...%q, want the full ids of the two ends", last.Base, last.Head)
	}
}
