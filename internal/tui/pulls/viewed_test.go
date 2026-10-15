package pulls

import (
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/visits"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// blobbed returns a service whose files each have a blob named after their path.
func blobbed() *fakeService {
	svc := newFakeService()
	svc.changed = sampleFiles()
	for i := range svc.changed {
		svc.changed[i].SHA = "sha-" + svc.changed[i].Path
	}
	return svc
}

func TestViewedInTheDiffFoldsAndMovesOn(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	press(t, h, "v")
	if got := mustPos(t, m); !strings.Contains(got, "disk.go") {
		t.Errorf("v left the diff at %q, want the next file", got)
	}
	st, saves := store.state()
	if saves != 1 || st.Viewed["cmd/gh-tui/main.go"] != "sha-cmd/gh-tui/main.go" {
		t.Errorf("kept %+v after %d saves, want the first file with its blob", st, saves)
	}
	if !m.files.diff.Collapsed("cmd/gh-tui/main.go") {
		t.Error("the file viewed is not folded")
	}
	if title := plain(m)[0]; !strings.Contains(title, "Files 1/7 viewed") {
		t.Errorf("the tree title is %q, want the count", title)
	}
	if !strings.Contains(strings.Join(plain(m), "\n"), "1 viewed") {
		t.Error("the status line lacks the count of the files viewed")
	}
}

func TestViewedHoldsOnlyWhileTheBlobStays(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{kept: map[string]visits.State{
		visits.Key(repo.String(), 142): {Viewed: map[string]string{
			"cmd/gh-tui/main.go":     "sha-cmd/gh-tui/main.go",
			"internal/cache/disk.go": "older",
		}},
	}}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	if !m.files.diff.Collapsed("cmd/gh-tui/main.go") {
		t.Error("a file viewed as it is now is not folded")
	}
	if m.files.diff.Collapsed("internal/cache/disk.go") {
		t.Error("a file that changed since it was viewed is folded")
	}
	if m.files.viewed != 1 {
		t.Errorf("%d files count as viewed, want 1", m.files.viewed)
	}
	if got := m.viewingOf(m.files.set.list()[1]); got != changedFile {
		t.Errorf("the changed file is %v, want changedFile", got)
	}
}

// treeFile returns the path of the file the cursor of the tree is on.
func treeFile(t *testing.T, m *detailModal) string {
	t.Helper()
	n, ok := m.files.tree.Selected()
	if !ok {
		t.Fatal("the tree has no row under the cursor")
	}
	file, ok := n.Value.(diff.File)
	if !ok {
		t.Fatalf("the tree row is %v, want a file", n.Value)
	}
	return file.Path
}

func TestViewedInTheTreeMarksAndUnmarks(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	press(t, h, "1")
	path := treeFile(t, m)
	press(t, h, "v")
	if got := treeFile(t, m); got != path {
		t.Errorf("v moved the tree from %q to %q, want it to stay", path, got)
	}
	if !m.files.diff.Collapsed(path) {
		t.Error("the file viewed in the tree is not folded in the diff")
	}
	at, _ := m.files.set.index(path)
	if m.files.viewMark(m.files.set.list()[at]) == "" {
		t.Error("the file viewed in the tree shows no mark")
	}
	if st, _ := store.state(); st.Viewed[path] != "sha-"+path {
		t.Errorf("kept %v, want %q with its blob", st.Viewed, path)
	}
	if title := plain(m)[0]; !strings.Contains(title, "Files 1/7 viewed") {
		t.Errorf("the tree title is %q, want the count", title)
	}

	press(t, h, "v")
	if m.files.diff.Collapsed(path) {
		t.Error("the file is still folded after v took the mark off")
	}
	if st, _ := store.state(); len(st.Viewed) != 0 {
		t.Errorf("kept %v after the mark was taken off, want none", st.Viewed)
	}
	if title := plain(m)[0]; strings.Contains(title, "viewed") {
		t.Errorf("the tree title is %q, want no count", title)
	}
	if got := treeFile(t, m); got != path {
		t.Errorf("the tree moved to %q, want it on %q", got, path)
	}
}

func TestViewedInTheDiffTakesTheMarkOff(t *testing.T) {
	t.Parallel()
	const first = "cmd/gh-tui/main.go"
	store := &memVisitStore{kept: map[string]visits.State{
		visits.Key(repo.String(), 142): {Viewed: map[string]string{first: "sha-" + first}},
	}}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	if !m.files.diff.Collapsed(first) || mustPos(t, m) != first {
		t.Fatalf("the diff is on %q with the first file folded %v, want it folded and under the cursor", mustPos(t, m), m.files.diff.Collapsed(first))
	}
	press(t, h, "v")
	if m.files.diff.Collapsed(first) {
		t.Error("the file is still folded after v took the mark off")
	}
	if got := mustPos(t, m); got != first {
		t.Errorf("v moved the diff to %q, want it to stay on %q", got, first)
	}
	if st, _ := store.state(); len(st.Viewed) != 0 {
		t.Errorf("kept %v after the mark was taken off, want none", st.Viewed)
	}
}

func TestViewedIsIgnoredWhileARangeIsOpen(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	down(t, h, rowContext)
	press(t, h, "V")
	if m.tray.from == nil {
		t.Fatal("V started no range")
	}
	press(t, h, "v")
	if _, saves := store.state(); saves != 0 || m.files.viewed != 0 {
		t.Errorf("v with a range open marked a file: %d saves, %d viewed", saves, m.files.viewed)
	}
}

func TestViewedSavesLandInOrder(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	m.visit.prev.Viewed = map[string]string{"a.go": "1"}
	first := m.saveViewed()
	m.visit.prev.Viewed = map[string]string{"a.go": "1", "b.go": "2"}
	second := m.saveViewed()
	// Commands run on goroutines of their own, so the later one may go first.
	second()
	first()
	st, saves := store.state()
	if saves != 1 || len(st.Viewed) != 2 {
		t.Errorf("kept %v after %d saves, want the later marks in one save", st.Viewed, saves)
	}
}

func TestViewedSaveAfterTheVisitLeavesTheLastSeen(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{kept: map[string]visits.State{
		visits.Key(repo.String(), 142): {LastSeen: visits.LastSeen{Head: "old", At: clock.Add(-time.Hour)}},
	}}
	_, m, advance := visiting(t, store, nil)
	m.visit.prev.Viewed = map[string]string{"a.go": "1"}
	late := m.saveViewed()
	advance(4 * time.Second)
	if err := m.writeVisit(); err != nil {
		t.Fatal(err)
	}
	late()
	st, saves := store.state()
	if saves != 1 || !st.LastSeen.At.Equal(clock) || st.Viewed["a.go"] != "1" {
		t.Errorf("kept %+v after %d saves, want the new last seen with the marks", st, saves)
	}
}
