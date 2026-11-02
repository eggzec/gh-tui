package pulls

import (
	"errors"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
)

// filesPanes opens the Files pane over the three changed files, which the tree
// lists as docs/c.md, internal/b.go and a.go, with the marks read.
func filesPanes(t *testing.T, setup func(*fakeService)) (*host, *panesModal, *fakeService) {
	t.Helper()
	h, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = changedFiles()
		if setup != nil {
			setup(svc)
		}
	})
	modalScreen(t, h)
	return h, m, svc
}

// hoveredPath returns the path of the file under the cursor, or "".
func hoveredPath(m *panesModal) string {
	path, _ := m.hoveredFile()
	return path
}

func TestFilesPaneReadsTheMarksOnce(t *testing.T) {
	t.Parallel()
	_, m, svc := filesPanes(t, func(svc *fakeService) { svc.holding(core.Viewed, "a.go") })
	if !m.files.vw.in || m.files.vw.viewing("a.go") != viewedFile {
		t.Errorf("the pane holds %+v, want a.go viewed", m.files.vw)
	}
	if svc.viewedReads != 1 {
		t.Errorf("marks read %d times, want once", svc.viewedReads)
	}
}

func TestFilesPaneNextAndPreviousFile(t *testing.T) {
	t.Parallel()
	h, m, _ := filesPanes(t, nil)
	for _, step := range []struct{ key, want string }{
		{"J", "docs/c.md"},
		{"J", "internal/b.go"},
		{"J", "a.go"},
		{"J", "a.go"},
		{"K", "internal/b.go"},
		{"K", "docs/c.md"},
		{"K", "docs/c.md"},
	} {
		press(t, h, step.key)
		modalScreen(t, h)
		if got := hoveredPath(m); got != step.want {
			t.Fatalf("after %s the cursor is on %q, want %q", step.key, got, step.want)
		}
	}
}

func TestFilesPanePreviousFileFromACommitGoesToTheLast(t *testing.T) {
	t.Parallel()
	h, m, _ := filesPanes(t, nil)
	// The commits start folded: open them, so that a commit has a row to stand on.
	m.files.tree.SetFolded(files.IDCommits, false)
	modalScreen(t, h)
	m.files.tree.Reveal(files.CommitID("ccc3333"))
	modalScreen(t, h)
	if n, _ := m.files.tree.Selected(); n.ID != files.CommitID("ccc3333") {
		t.Fatalf("the cursor is on %q, want the commit row", n.ID)
	}
	press(t, h, "K")
	modalScreen(t, h)
	if got := hoveredPath(m); got != "a.go" {
		t.Errorf("K from a commit went to %q, want the last file", got)
	}
}

func TestFilesPaneNextFileOpensItsFolder(t *testing.T) {
	t.Parallel()
	h, m, _ := filesPanes(t, nil)
	press(t, h, "J")
	modalScreen(t, h)
	m.files.tree.SetFolded(files.DirID("internal"), true)
	press(t, h, "J")
	modalScreen(t, h)
	if got := hoveredPath(m); got != "internal/b.go" {
		t.Errorf("J onto a folded folder went to %q, want internal/b.go", got)
	}
	if m.files.tree.Folded(files.DirID("internal")) {
		t.Error("the folder of the file is still folded")
	}
}

func TestFilesPaneSpaceMarksTheFileAndMovesOn(t *testing.T) {
	t.Parallel()
	h, m, svc := filesPanes(t, nil)
	press(t, h, "J")
	press(t, h, "space")
	modalScreen(t, h)
	if got := svc.sets(); !slices.Equal(got, []string{"docs/c.md=true"}) {
		t.Errorf("marks sent = %v, want docs/c.md marked", got)
	}
	if m.files.vw.viewing("docs/c.md") != viewedFile {
		t.Error("the file marked shows no mark")
	}
	if got := hoveredPath(m); got != "internal/b.go" {
		t.Errorf("space left the cursor on %q, want the next file", got)
	}
	press(t, h, "K")
	press(t, h, "space")
	if got := svc.sets(); !slices.Equal(got, []string{"docs/c.md=true", "docs/c.md=false"}) {
		t.Errorf("marks sent = %v, want docs/c.md marked then unmarked", got)
	}
	if m.files.vw.viewing("docs/c.md") != notViewed {
		t.Error("the file unmarked still shows its mark")
	}
}

func TestFilesPaneSpaceOnTheLastFileStays(t *testing.T) {
	t.Parallel()
	h, m, svc := filesPanes(t, nil)
	press(t, h, "K")
	press(t, h, "space")
	if got := svc.sets(); !slices.Equal(got, []string{"a.go=true"}) {
		t.Errorf("marks sent = %v, want a.go marked", got)
	}
	if got := hoveredPath(m); got != "a.go" {
		t.Errorf("space on the last file moved to %q, want it to stay", got)
	}
}

func TestFilesPaneMarkSharedWithTheDiff(t *testing.T) {
	t.Parallel()
	h, m, _, _ := viewedPanes(t, "a.go", nil)
	press(t, h, "v")
	if m.files.vw.viewing("a.go") != viewedFile {
		t.Error("a mark set in the diff doesn't show in the Files pane")
	}
}

func TestFilesPaneSpaceThatGitHubRefusesComesBack(t *testing.T) {
	t.Parallel()
	h, m, svc := filesPanes(t, func(svc *fakeService) { svc.viewedSendErr = errors.New("nope") })
	press(t, h, "J")
	press(t, h, "space")
	if got := svc.sets(); len(got) != 1 {
		t.Errorf("marks sent = %v, want one", got)
	}
	if m.files.vw.viewing("docs/c.md") != notViewed {
		t.Error("a mark that GitHub refused stays")
	}
	if len(m.files.vw.sending) != 0 {
		t.Errorf("%v still sending after the answer", m.files.vw.sending)
	}
}

func TestFilesPaneSpaceOnAServerWithoutMarksSaysSo(t *testing.T) {
	t.Parallel()
	h, m, svc := filesPanes(t, func(svc *fakeService) { svc.viewedOff = true })
	press(t, h, "J")
	cmd, ok := m.pressFiles(keyMsg("space"))
	if !ok || cmd == nil {
		t.Fatalf("space on a file without marks gave %v, %v, want a hint", cmd, ok)
	}
	if got := svc.sets(); len(got) != 0 {
		t.Errorf("marks sent = %v, want none", got)
	}
	if got := hoveredPath(m); got != "docs/c.md" {
		t.Errorf("the cursor moved to %q, want it to stay", got)
	}
}

func TestFilesPaneSpaceOnACommitKeepsTheRange(t *testing.T) {
	t.Parallel()
	_, m, svc := filesPanes(t, nil)
	m.files.rng = files.Range{First: 1, Last: 1}
	markCommit(m, "ddd4444")
	if m.files.rng != (files.Range{First: 1, Last: 3}) {
		t.Errorf("range = %+v, want 1..3", m.files.rng)
	}
	if got := svc.sets(); len(got) != 0 {
		t.Errorf("marks sent = %v, want none", got)
	}
}
