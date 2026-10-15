package pulls

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/visits"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
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
	if title := plain(m)[0]; !strings.Contains(title, "1/7 viewed") {
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
	if title := plain(m)[0]; !strings.Contains(title, "1/7 viewed") {
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

func TestReadOnPagesInTheMiddleOfAFile(t *testing.T) {
	t.Parallel()
	svc := blobbed()
	extra := make([]string, 40)
	for i := range extra {
		extra[i] = "+\tx()"
	}
	svc.changed[0].Patch = mainPatch(extra...)
	store := &memVisitStore{}
	h, m := filedWith(t, svc, 100, 12, WithVisits(store))
	press(t, h, "[")
	before := m.files.diff.Cursor()
	press(t, h, "space")
	if got := mustPos(t, m); !strings.Contains(got, "main.go") {
		t.Errorf("space left the diff at %q, want the same file", got)
	}
	if after := m.files.diff.Cursor(); after <= before {
		t.Errorf("space left the cursor at row %d, want it below row %d", after, before)
	}
	if _, saves := store.state(); saves != 0 || m.files.viewed != 0 {
		t.Errorf("space in the middle of a file marked it: %d saves, %d viewed", saves, m.files.viewed)
	}
}

func TestReadOnMarksAFileOfSeveralWindows(t *testing.T) {
	t.Parallel()
	// Lengths that put the end of the file at every phase of a page, among them
	// tails shorter than a window.
	for n := 40; n < 56; n++ {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			svc := blobbed()
			extra := make([]string, n)
			for i := range extra {
				extra[i] = "+\tx()"
			}
			svc.changed[0].Patch = mainPatch(extra...)
			store := &memVisitStore{}
			h, m := filedWith(t, svc, 100, 12, WithVisits(store))
			press(t, h, "[")
			for presses := 1; m.files.viewed == 0; presses++ {
				if presses > 30 {
					t.Fatal("space never marked the first file")
				}
				press(t, h, "space")
				got := mustPos(t, m)
				if m.files.viewed == 0 && !strings.Contains(got, "main.go") {
					t.Fatalf("space left the first file at %q before marking it", got)
				}
			}
			if got := mustPos(t, m); !strings.Contains(got, "disk.go") {
				t.Errorf("after marking, the diff is at %q, want the next file", got)
			}
			if st, saves := store.state(); saves != 1 || st.Viewed["cmd/gh-tui/main.go"] == "" {
				t.Errorf("kept %+v after %d saves, want the first file", st, saves)
			}
		})
	}
}

func TestReadOnMarksAtTheEndAndMovesToTheNextUnviewed(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	press(t, h, "space")
	if got := mustPos(t, m); !strings.Contains(got, "disk.go") {
		t.Errorf("space left the diff at %q, want the next file", got)
	}
	st, saves := store.state()
	if saves != 1 || st.Viewed["cmd/gh-tui/main.go"] != "sha-cmd/gh-tui/main.go" {
		t.Errorf("kept %+v after %d saves, want the first file with its blob", st, saves)
	}
	if !m.files.diff.Collapsed("cmd/gh-tui/main.go") {
		t.Error("the file read is not folded")
	}
}

func TestReadOnWrapsToAnUnviewedFile(t *testing.T) {
	t.Parallel()
	viewed := map[string]string{}
	for _, f := range blobbed().changed[1:] {
		viewed[f.Path] = "sha-" + f.Path
	}
	store := &memVisitStore{kept: map[string]visits.State{visits.Key(repo.String(), 142): {Viewed: viewed}}}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	press(t, h, "G")
	last := mustPos(t, m)
	if last == "cmd/gh-tui/main.go" {
		t.Fatal("G stayed in the first file")
	}
	press(t, h, "space")
	if got := mustPos(t, m); got != "cmd/gh-tui/main.go" {
		t.Errorf("space left the diff at %q, want the first file, the only one not viewed", got)
	}
	if want := len(m.files.set.list()) - 1; m.files.viewed != want {
		t.Errorf("%d files viewed, want %d: all but the first", m.files.viewed, want)
	}
}

// allViewed returns a store that has every file of blobbed viewed.
func allViewed() *memVisitStore {
	viewed := map[string]string{}
	for _, f := range blobbed().changed {
		viewed[f.Path] = "sha-" + f.Path
	}
	return &memVisitStore{kept: map[string]visits.State{visits.Key(repo.String(), 142): {Viewed: viewed}}}
}

func TestReadOnWithEveryFileViewedGoesToTheFirst(t *testing.T) {
	t.Parallel()
	store := allViewed()
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	press(t, h, "G")
	if got := mustPos(t, m); got == "cmd/gh-tui/main.go" {
		t.Fatal("G stayed in the first file")
	}
	press(t, h, "space")
	if got := mustPos(t, m); got != "cmd/gh-tui/main.go" {
		t.Errorf("space left the diff at %q, want the first file", got)
	}
	if _, saves := store.state(); saves != 0 {
		t.Errorf("reading on over viewed files saved %d times, want none", saves)
	}
}

func TestReadOnWithEveryFileViewedPagesOnAndWraps(t *testing.T) {
	t.Parallel()
	store := allViewed()
	// A window of 3 rows pages through the folded headers a few at a time.
	h, m := filedWith(t, blobbed(), 100, 5, WithVisits(store))
	press(t, h, "[")
	if got := mustPos(t, m); got != "cmd/gh-tui/main.go" {
		t.Fatalf("the diff is at %q, want the first file", got)
	}
	last := m.files.set.list()[len(m.files.set.list())-1].Path
	moved, wrapped := false, false
	for range 20 {
		before := m.files.diff.Cursor()
		press(t, h, "space")
		got := mustPos(t, m)
		if got != "cmd/gh-tui/main.go" {
			moved = true
		}
		if moved && got == "cmd/gh-tui/main.go" {
			wrapped = true
			break
		}
		if m.files.diff.Cursor() <= before && got != last {
			t.Fatalf("space stuck at row %d in %q", before, got)
		}
	}
	if !moved || !wrapped {
		t.Errorf("space moved on %v and wrapped to the first file %v, want both", moved, wrapped)
	}
	if _, saves := store.state(); saves != 0 {
		t.Errorf("reading on over viewed files saved %d times, want none", saves)
	}
}

func TestReadOnOnlyPagesBeforeTheVisitLoads(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	m.visit.loaded = false
	press(t, h, "space")
	if _, saves := store.state(); saves != 0 || m.files.viewed != 0 {
		t.Errorf("space before the visit loaded marked a file: %d saves, %d viewed", saves, m.files.viewed)
	}
	if m.files.diff.Collapsed("cmd/gh-tui/main.go") {
		t.Error("the first file is folded before the visit loaded")
	}
}

func TestReadOnIsIgnoredWhileARangeIsOpen(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	down(t, h, rowContext)
	press(t, h, "V")
	if m.tray.from == nil {
		t.Fatal("V started no range")
	}
	at := m.files.diff.Cursor()
	press(t, h, "space")
	if _, saves := store.state(); saves != 0 || m.files.viewed != 0 || m.files.diff.Cursor() != at {
		t.Errorf("space with a range open read on: %d saves, %d viewed", saves, m.files.viewed)
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

// toastsOf runs cmd, and the commands of a batch, and returns the toasts they ask
// for.
func toastsOf(cmd tea.Cmd) []ui.NotifyMsg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []ui.NotifyMsg
		for _, c := range msg {
			out = append(out, toastsOf(c)...)
		}
		return out
	case ui.NotifyMsg:
		return []ui.NotifyMsg{msg}
	}
	return nil
}

func TestViewedBeforeTheVisitLoadsSaysSo(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	m.visit.loaded = false
	file := m.files.set.list()[0]
	got := toastsOf(m.toggleViewed(file, false))
	if len(got) != 1 || got[0].Level != toast.Info || !strings.Contains(got[0].Text, "Still reading") {
		t.Errorf("toasts = %+v, want one info that the files viewed are still being read", got)
	}
	if _, saves := store.state(); saves != 0 || m.files.viewed != 0 || m.files.diff.Collapsed(file.Path) {
		t.Errorf("the mark changed before the visit loaded: %d saves, %d viewed", saves, m.files.viewed)
	}
}

func TestViewedAfterAFailedReadWarnsOnce(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{err: errors.New("unreadable")}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	if !m.visit.readFailed {
		t.Fatal("the failed read did not set readFailed")
	}
	files := m.files.set.list()
	first := toastsOf(m.toggleViewed(files[0], false))
	if len(first) != 1 || first[0].Level != toast.Warning || !strings.Contains(first[0].Text, "won't be kept") {
		t.Errorf("toasts = %+v, want one warning that marks won't be kept", first)
	}
	if !m.files.diff.Collapsed(files[0].Path) || m.files.viewed != 1 {
		t.Error("the mark did not change for the session")
	}
	if again := toastsOf(m.toggleViewed(files[1], false)); len(again) != 0 {
		t.Errorf("a second mark warned again: %+v", again)
	}
	if m.files.viewed != 2 {
		t.Errorf("%d files viewed, want 2", m.files.viewed)
	}
	if _, saves := store.state(); saves != 0 {
		t.Errorf("%d saves over a state that could not be read, want none", saves)
	}
}

func TestViewedSaveThatFailsLetsALaterOneRun(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{saveFails: 1}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	m.visit.prev.Viewed = map[string]string{"a.go": "1"}
	first := m.saveViewed()
	m.visit.prev.Viewed = map[string]string{"a.go": "1", "b.go": "2"}
	second := m.saveViewed()
	got := toastsOf(first)
	if len(got) != 1 || got[0].Level != toast.Warning {
		t.Errorf("toasts = %+v, want one warning for the failed save", got)
	}
	second()
	st, saves := store.state()
	if saves != 1 || len(st.Viewed) != 2 {
		t.Errorf("kept %v after %d saves, want the later marks kept after the failure", st.Viewed, saves)
	}
}

func TestViewedSaveThatFailsLetsAnOlderOneRun(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{saveFails: 1}
	h, m := filedWith(t, blobbed(), 100, 30, WithVisits(store))
	press(t, h, "[")
	m.visit.prev.Viewed = map[string]string{"a.go": "1"}
	older := m.saveViewed()
	m.visit.prev.Viewed = map[string]string{"a.go": "1", "b.go": "2"}
	newer := m.saveViewed()
	newer()
	older()
	st, saves := store.state()
	if saves != 1 || len(st.Viewed) != 1 {
		t.Errorf("kept %v after %d saves, want the older marks kept after the newer save failed", st.Viewed, saves)
	}
}

func TestVisitWriteThatFailsLetsAPendingSaveRun(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	_, m, advance := visiting(t, store, nil)
	m.visit.prev.Viewed = map[string]string{"a.go": "1"}
	late := m.saveViewed()
	advance(4 * time.Second)
	store.mu.Lock()
	store.saveFails = 1
	store.mu.Unlock()
	if err := m.writeVisit(); err == nil {
		t.Fatal("writeVisit returned no error from a failing store")
	}
	late()
	st, saves := store.state()
	if saves != 1 || st.Viewed["a.go"] != "1" {
		t.Errorf("kept %+v after %d saves, want the pending save to keep the marks", st, saves)
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
