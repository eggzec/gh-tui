package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

func changedFiles() []core.CommitFile {
	return []core.CommitFile{
		{Path: "internal/b.go", Additions: 20, Deletions: 3},
		{Path: "a.go", Additions: 10, Deletions: 1},
		{Path: "docs/c.md", Additions: 0, Deletions: 0},
	}
}

func paths(fs []core.CommitFile) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Path)
	}
	return out
}

func TestFilesPaneAllUsesPullFiles(t *testing.T) {
	t.Parallel()
	h, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = changedFiles()
		svc.rangeFiles = &fakeRange{}
	})
	if len(svc.rangeFiles.reads) != 0 {
		t.Errorf("compare was read for every commit: %+v", svc.rangeFiles.reads)
	}
	if len(svc.fileReads) != 1 || svc.fileReads[0].Head != "ddd4444" {
		t.Errorf("file reads = %+v, want one at the head", svc.fileReads)
	}
	base, head, fs, ok := m.filesRange()
	if !ok || base != "base000" || head != "ddd4444" || len(fs) != 3 {
		t.Errorf("filesRange = %q %q %v %v, want the pull request's files from its base to its head", base, head, paths(fs), ok)
	}
	screen := modalScreen(t, h)
	// The files show as a tree: a row for each folder, its files under it by
	// base name, in this order.
	at := 0
	for _, want := range []string{"docs/", "c.md", "internal/", "b.go", "a.go"} {
		i := strings.Index(screen[at:], want)
		if i < 0 {
			t.Fatalf("the screen doesn't list %q after offset %d, in the tree's order:\n%s", want, at, screen)
		}
		at += i + len(want)
	}
}

func TestFilesPaneSubRangeComparesFromParent(t *testing.T) {
	t.Parallel()
	h, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = changedFiles()
		svc.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: []core.CommitFile{{Path: "only.go", Additions: 2}}}}
	})
	for _, c := range []struct {
		rng        files.Range
		base, head string
	}{
		{files.Range{First: 1, Last: 2}, "aaa1111", "ccc3333"},
		{files.Range{First: 0, Last: 1}, "base000", "bbb2222"},
	} {
		m.files.rng = c.rng
		drain(t, h, m.readRange())
		got := svc.rangeFiles.reads[len(svc.rangeFiles.reads)-1]
		if got.Base != c.base || got.Head != c.head {
			t.Errorf("range %+v compared %q...%q, want %q...%q", c.rng, got.Base, got.Head, c.base, c.head)
		}
		if _, _, fs, ok := m.filesRange(); !ok || !slices.Equal(paths(fs), []string{"only.go"}) {
			t.Errorf("range %+v lists %v (ok %v), want only.go", c.rng, paths(fs), ok)
		}
	}
	if screen := modalScreen(t, h); strings.Contains(screen, "internal/b.go") {
		t.Errorf("files outside the range show:\n%s", screen)
	}
}

func TestFilesPaneSubRangeTruncated(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: changedFiles(), Truncated: true}}
	})
	m.files.rng = files.Range{First: 1, Last: 1}
	drain(t, h, m.readRange())
	// The folders take rows too, so the pane is zoomed to show the note below
	// the last file.
	press(t, h, "z")
	if screen := modalScreen(t, h); !strings.Contains(screen, "first 3 files of this range") {
		t.Errorf("no note that the list is cut short:\n%s", screen)
	}
}

func TestFilesPaneDropsStaleRangeAnswer(t *testing.T) {
	t.Parallel()
	_, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.rangeFiles = &fakeRange{byBase: map[string]core.CompareDetail{
			"aaa1111": {Files: []core.CommitFile{{Path: "early.go"}}},
			"bbb2222": {Files: []core.CommitFile{{Path: "late.go"}}},
		}}
	})
	m.files.rng = files.Range{First: 1, Last: 1}
	early := m.readRange()
	m.files.rng = files.Range{First: 2, Last: 2}
	late := m.readRange()
	m.Update(late())
	m.Update(early())
	if _, _, fs, ok := m.filesRange(); !ok || !slices.Equal(paths(fs), []string{"late.go"}) {
		t.Errorf("files = %v (ok %v), want those of the range now: late.go", paths(fs), ok)
	}
	// The reads ran in the order late, early.
	if len(svc.rangeFiles.ctxs) != 2 || svc.rangeFiles.ctxs[0].Err() != nil {
		t.Fatalf("contexts: %d, want two, the read of the range now live", len(svc.rangeFiles.ctxs))
	}
	if svc.rangeFiles.ctxs[1].Err() == nil {
		t.Error("the read of the range that was left kept running")
	}
	m.Discard()
	if svc.rangeFiles.ctxs[0].Err() == nil {
		t.Error("closing the modal didn't end the read")
	}
}

func TestFilesPaneRewritten(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = changedFiles()
		svc.rangeFiles = &fakeRange{err: core.ErrRewritten}
	})
	m.files.rng = files.Range{First: 1, Last: 2}
	drain(t, h, m.readRange())
	if got := m.files.read.err; got != "these commits are no longer on the branch" {
		t.Errorf("error = %q, want the commits no longer on the branch", got)
	}
	// The pane is narrow, so the row is cut.
	if screen := modalScreen(t, h); !strings.Contains(screen, "these commits are no longer") {
		t.Errorf("no inline error:\n%s", screen)
	}
	if _, _, _, ok := m.filesRange(); ok {
		t.Error("filesRange is ok after a failed read")
	}
}

func TestFilesPaneFailedReadRecoversOnline(t *testing.T) {
	t.Parallel()
	h, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = changedFiles()
		svc.rangeFiles = &fakeRange{err: errors.New("offline")}
	})
	m.files.rng = files.Range{First: 1, Last: 2}
	drain(t, h, m.readRange())
	if _, _, _, ok := m.filesRange(); ok || m.files.read.err == "" {
		t.Fatalf("the read didn't fail: err = %q", m.files.read.err)
	}
	svc.mu.Lock()
	svc.rangeFiles.err = nil
	svc.rangeFiles.detail = core.CompareDetail{Files: []core.CommitFile{{Path: "only.go", Additions: 2}}}
	svc.mu.Unlock()
	drain(t, h, m.Update(ui.OnlineMsg{}))
	if _, _, fs, ok := m.filesRange(); !ok || !slices.Equal(paths(fs), []string{"only.go"}) {
		t.Errorf("files = %v (ok %v), want only.go once GitHub answers", paths(fs), ok)
	}
}

func TestFilesPaneTitleTotals(t *testing.T) {
	t.Parallel()
	h, _, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = changedFiles() })
	if screen := modalScreen(t, h); !strings.Contains(screen, "3 files") || !strings.Contains(screen, "+30") {
		t.Errorf("the title doesn't total the files:\n%s", screen)
	}
}

func TestFilesPaneFoundSelectsFile(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = changedFiles() })
	m.chooseFile("internal/b.go")
	modalScreen(t, h)
	if path, ok := m.hoveredFile(); !ok || path != "internal/b.go" {
		t.Errorf("hovered = %q %v, want the file found", path, ok)
	}
	if m.found != "" {
		t.Errorf("found = %q, want it taken", m.found)
	}
	m.chooseFile("gone.go")
	modalScreen(t, h)
	if m.found != "" {
		t.Errorf("found = %q, want a file the range doesn't have forgotten", m.found)
	}
}

func BenchmarkFilesContent(b *testing.B) {
	_, m, _ := rangePanes(b, sampleCommits(nil), func(svc *fakeService) {
		for i := range 200 {
			svc.changed = append(svc.changed, core.CommitFile{Path: "pkg/dir" + string(rune('a'+i%20)) + "/file" + string(rune('a'+i/20)) + ".go", Additions: i, Deletions: i / 2})
		}
	})
	box, _ := m.layout().Box(paneFiles, panes.List)
	b.ReportAllocs()
	for b.Loop() {
		m.filesContent(box)
	}
}
