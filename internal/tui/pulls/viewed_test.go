package pulls

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

const (
	firstFile  = "cmd/gh-tui/main.go"
	secondFile = "internal/cache/disk.go"
)

// withFiles returns a service that changes the sample files.
func withFiles() *fakeService {
	svc := newFakeService()
	svc.changed = sampleFiles()
	return svc
}

// restViewed makes GitHub keep every file of the sample but those in except as
// viewed.
func restViewed(svc *fakeService, except ...string) {
	for _, f := range svc.changed {
		if !slices.Contains(except, f.Path) {
			svc.holding(core.Viewed, f.Path)
		}
	}
}

// msgsOf runs cmd and the commands of the batches it answers with, and returns
// the messages they end in.
func msgsOf(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, msgsOf(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestViewedInTheDiffMarksOnGitHubFoldsAndMovesOn(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	store := &memVisitStore{}
	h, m := filedWith(t, svc, 100, 30, WithVisits(store))
	press(t, h, "[")
	press(t, h, "v")
	if got := mustPos(t, m); !strings.Contains(got, "disk.go") {
		t.Errorf("v left the diff at %q, want the next file", got)
	}
	if got := svc.sets(); len(got) != 1 || got[0] != firstFile+"=true" {
		t.Errorf("marks sent = %v, want the first file marked", got)
	}
	if !m.files.diff.Collapsed(firstFile) {
		t.Error("the file viewed is not folded")
	}
	if title := plain(m)[0]; !strings.Contains(title, "1/7 viewed") {
		t.Errorf("the tree title is %q, want the count", title)
	}
	if !strings.Contains(strings.Join(plain(m), "\n"), "1 viewed") {
		t.Error("the status line lacks the count of the files viewed")
	}
	if _, saves := store.state(); saves != 0 {
		t.Errorf("v wrote the visit store %d times, want none", saves)
	}
}

func TestViewedShowsWhatGitHubKeeps(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	svc.holding(core.Viewed, firstFile)
	svc.holding(core.ViewedChanged, secondFile)
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	if !m.files.diff.Collapsed(firstFile) {
		t.Error("a file viewed as it is now is not folded")
	}
	if m.files.diff.Collapsed(secondFile) {
		t.Error("a file that changed since it was viewed is folded")
	}
	if m.files.viewed != 1 {
		t.Errorf("%d files count as viewed, want 1", m.files.viewed)
	}
	if got := m.viewingOf(m.files.set.list()[1]); got != changedFile {
		t.Errorf("the changed file is %v, want changedFile", got)
	}
	if got := m.files.viewMark(m.files.set.list()[1]); got == "" {
		t.Error("the changed file shows no mark that it changed since")
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
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
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
	if title := plain(m)[0]; !strings.Contains(title, "1/7 viewed") {
		t.Errorf("the tree title is %q, want the count", title)
	}

	press(t, h, "v")
	if m.files.diff.Collapsed(path) {
		t.Error("the file is still folded after v took the mark off")
	}
	if got := svc.sets(); len(got) != 2 || got[0] != path+"=true" || got[1] != path+"=false" {
		t.Errorf("marks sent = %v, want a mark and an unmark of %q", got, path)
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
	svc := withFiles()
	svc.holding(core.Viewed, firstFile)
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	if !m.files.diff.Collapsed(firstFile) || mustPos(t, m) != firstFile {
		t.Fatalf("the diff is on %q with the first file folded %v, want it folded and under the cursor", mustPos(t, m), m.files.diff.Collapsed(firstFile))
	}
	press(t, h, "v")
	if m.files.diff.Collapsed(firstFile) {
		t.Error("the file is still folded after v took the mark off")
	}
	if got := mustPos(t, m); got != firstFile {
		t.Errorf("v moved the diff to %q, want it to stay on %q", got, firstFile)
	}
	if got := svc.sets(); len(got) != 1 || got[0] != firstFile+"=false" {
		t.Errorf("marks sent = %v, want an unmark of the first file", got)
	}
}

func TestViewedThatGitHubRefusesComesBack(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	svc.viewedSendErr = errors.New("refused")
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	file := m.files.set.list()[0]
	cmd := m.toggleViewed(file, false)
	if !m.files.diff.Collapsed(file.Path) || m.files.viewed != 1 {
		t.Fatal("the mark did not show at once")
	}
	var failed bool
	for _, msg := range msgsOf(cmd) {
		switch msg := msg.(type) {
		case ui.FailMsg:
			failed = strings.Contains(msg.What, "mark a file of #142 viewed") && msg.Err != nil
		case viewedSetMsg:
			_ = m.Update(msg)
		}
	}
	if !failed {
		t.Error("the refusal told the user nothing")
	}
	if m.files.diff.Collapsed(file.Path) || m.files.viewed != 0 {
		t.Errorf("the file is still marked after the refusal: folded %v, %d viewed", m.files.diff.Collapsed(file.Path), m.files.viewed)
	}
	if got := m.viewingOf(file); got != notViewed {
		t.Errorf("the file is %v, want notViewed", got)
	}
}

func TestViewedUnmarkThatGitHubRefusesComesBack(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	svc.holding(core.Viewed, firstFile)
	svc.viewedSendErr = errors.New("refused")
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	file := m.files.set.list()[0]
	for _, msg := range msgsOf(m.toggleViewed(file, false)) {
		if msg, ok := msg.(viewedSetMsg); ok {
			_ = m.Update(msg)
		}
	}
	if !m.files.diff.Collapsed(file.Path) || m.viewingOf(file) != viewedFile {
		t.Error("the mark did not come back after GitHub refused to take it off")
	}
}

func TestViewedReadThatLandsWhileAMarkIsInFlightKeepsTheMark(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	file := m.files.set.list()[0]
	cmd := m.toggleViewed(file, false)
	// A read answered before the mark was sent says that nothing is marked.
	_ = m.Update(viewedMsg{files: m.files, pv: core.PullViewed{Supported: true}})
	if m.viewingOf(file) != viewedFile || !m.files.diff.Collapsed(file.Path) || m.files.viewed != 1 {
		t.Error("a read that landed in the meantime took the mark off the screen")
	}
	for _, msg := range msgsOf(cmd) {
		if msg, ok := msg.(viewedSetMsg); ok {
			_ = m.Update(msg)
		}
	}
	if m.viewingOf(file) != viewedFile || !m.files.diff.Collapsed(file.Path) {
		t.Error("the mark is gone once GitHub answered")
	}
}

func TestViewedReadThatLandsWhileAnUnmarkIsInFlightKeepsItOff(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	svc.holding(core.Viewed, firstFile)
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	file := m.files.set.list()[0]
	cmd := m.toggleViewed(file, false)
	_ = m.Update(viewedMsg{files: m.files, pv: core.PullViewed{Supported: true, States: map[string]core.ViewedState{firstFile: core.Viewed}}})
	if m.viewingOf(file) != notViewed {
		t.Error("a read that landed in the meantime brought the mark back")
	}
	for _, msg := range msgsOf(cmd) {
		if msg, ok := msg.(viewedSetMsg); ok {
			_ = m.Update(msg)
		}
	}
	if m.viewingOf(file) != notViewed {
		t.Error("the mark is back once GitHub answered")
	}
}

func TestViewedRefusalAfterTheFilesRestartedTakesTheMarkOff(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	svc.viewedSendErr = errors.New("refused")
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	file := m.files.set.list()[0]
	cmd := m.toggleViewed(file, false)
	// The scope changes before GitHub answers, for the same head.
	_ = m.startFiles(m.files, m.files.scope)
	if m.viewingOf(file) != viewedFile {
		t.Fatal("the files that restarted don't show the mark that is on its way")
	}
	for _, msg := range msgsOf(cmd) {
		if msg, ok := msg.(viewedSetMsg); ok {
			_ = m.Update(msg)
		}
	}
	if m.viewingOf(file) != notViewed {
		t.Error("the files that restarted keep a mark that GitHub refused")
	}
}

func TestViewedSecondPressWhileOneIsInFlightSendsNothing(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	file := m.files.set.list()[0]
	// The command isn't run, so the answer to the first isn't in.
	if m.toggleViewed(file, false) == nil {
		t.Fatal("the first press did nothing")
	}
	if cmd := m.toggleViewed(file, false); cmd != nil {
		t.Error("a second press of the same file while one is in flight did something")
	}
	if got := svc.sets(); len(got) != 1 {
		t.Errorf("marks sent = %v, want one", got)
	}
}

func TestViewedOnAServerWithoutMarksSaysSo(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	svc.viewedOff = true
	store := &memVisitStore{}
	h, m := filedWith(t, svc, 100, 30, WithVisits(store))
	press(t, h, "[")
	got := toastsOf(m.toggleViewed(m.files.set.list()[0], false))
	if len(got) != 1 || got[0].Level != toast.Info || !strings.Contains(got[0].Text, "keeps no viewed marks") {
		t.Errorf("toasts = %+v, want one info that the server keeps no marks", got)
	}
	if sets := svc.sets(); len(sets) != 0 || m.files.viewed != 0 || m.files.diff.Collapsed(firstFile) {
		t.Errorf("a file was marked: sent %v, %d viewed", sets, m.files.viewed)
	}
	if _, saves := store.state(); saves != 0 {
		t.Errorf("the visit store was written %d times, want none", saves)
	}
}

func TestViewedBeforeTheMarksAreReadSaysSo(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	m.files.pvIn = false
	file := m.files.set.list()[0]
	got := toastsOf(m.toggleViewed(file, false))
	if len(got) != 1 || got[0].Level != toast.Info || !strings.Contains(got[0].Text, "Still reading") {
		t.Errorf("toasts = %+v, want one info that the files viewed are still being read", got)
	}
	if sets := svc.sets(); len(sets) != 0 || m.files.diff.Collapsed(file.Path) {
		t.Errorf("the mark changed before the marks were read: sent %v", sets)
	}
}

func TestViewedAfterAFailedReadShowsNoMarksAndSaysSo(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	svc.viewedReadErr = errors.New("unreadable")
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	if !m.files.pvFailed || m.files.viewed != 0 {
		t.Fatalf("pvFailed = %v with %d viewed, want a failed read and no marks", m.files.pvFailed, m.files.viewed)
	}
	if len(m.files.set.list()) == 0 {
		t.Fatal("the files are not shown after the failed read")
	}
	got := toastsOf(m.toggleViewed(m.files.set.list()[0], false))
	if len(got) != 1 || !strings.Contains(got[0].Text, "couldn't be read") {
		t.Errorf("toasts = %+v, want one that the marks couldn't be read", got)
	}
	if sets := svc.sets(); len(sets) != 0 {
		t.Errorf("marks sent = %v after a failed read, want none", sets)
	}
}

func TestViewedIsIgnoredWhileARangeIsOpen(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	down(t, h, rowContext)
	press(t, h, "V")
	if m.tray.from == nil {
		t.Fatal("V started no range")
	}
	press(t, h, "v")
	if sets := svc.sets(); len(sets) != 0 || m.files.viewed != 0 {
		t.Errorf("v with a range open marked a file: sent %v, %d viewed", sets, m.files.viewed)
	}
}

func TestReadOnPagesInTheMiddleOfAFile(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	extra := make([]string, 40)
	for i := range extra {
		extra[i] = "+\tx()"
	}
	svc.changed[0].Patch = mainPatch(extra...)
	h, m := filedWith(t, svc, 100, 12)
	press(t, h, "[")
	before := m.files.diff.Cursor()
	press(t, h, "space")
	if got := mustPos(t, m); !strings.Contains(got, "main.go") {
		t.Errorf("space left the diff at %q, want the same file", got)
	}
	if after := m.files.diff.Cursor(); after <= before {
		t.Errorf("space left the cursor at row %d, want it below row %d", after, before)
	}
	if sets := svc.sets(); len(sets) != 0 || m.files.viewed != 0 {
		t.Errorf("space in the middle of a file marked it: sent %v, %d viewed", sets, m.files.viewed)
	}
}

func TestReadOnMarksAFileOfSeveralWindows(t *testing.T) {
	t.Parallel()
	// Lengths that put the end of the file at every phase of a page, among them
	// tails shorter than a window.
	for n := 40; n < 56; n++ {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			svc := withFiles()
			extra := make([]string, n)
			for i := range extra {
				extra[i] = "+\tx()"
			}
			svc.changed[0].Patch = mainPatch(extra...)
			h, m := filedWith(t, svc, 100, 12)
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
			if sets := svc.sets(); len(sets) != 1 || sets[0] != firstFile+"=true" {
				t.Errorf("marks sent = %v, want the first file", sets)
			}
		})
	}
}

func TestReadOnMarksAtTheEndAndMovesToTheNextUnviewed(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	press(t, h, "space")
	if got := mustPos(t, m); !strings.Contains(got, "disk.go") {
		t.Errorf("space left the diff at %q, want the next file", got)
	}
	if sets := svc.sets(); len(sets) != 1 || sets[0] != firstFile+"=true" {
		t.Errorf("marks sent = %v, want the first file", sets)
	}
	if !m.files.diff.Collapsed(firstFile) {
		t.Error("the file read is not folded")
	}
}

func TestReadOnWrapsToAnUnviewedFile(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	restViewed(svc, firstFile)
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	press(t, h, "G")
	if mustPos(t, m) == firstFile {
		t.Fatal("G stayed in the first file")
	}
	press(t, h, "space")
	if got := mustPos(t, m); got != firstFile {
		t.Errorf("space left the diff at %q, want the first file, the only one not viewed", got)
	}
	if want := len(m.files.set.list()) - 1; m.files.viewed != want {
		t.Errorf("%d files viewed, want %d: all but the first", m.files.viewed, want)
	}
}

func TestReadOnWithEveryFileViewedGoesToTheFirst(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	restViewed(svc)
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	press(t, h, "G")
	if got := mustPos(t, m); got == firstFile {
		t.Fatal("G stayed in the first file")
	}
	press(t, h, "space")
	if got := mustPos(t, m); got != firstFile {
		t.Errorf("space left the diff at %q, want the first file", got)
	}
	if sets := svc.sets(); len(sets) != 0 {
		t.Errorf("reading on over viewed files sent %v, want none", sets)
	}
}

func TestReadOnWithEveryFileViewedPagesOnAndWraps(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	restViewed(svc)
	// A modal 5 rows high leaves a window that pages through the folded
	// headers a few at a time.
	h, m := filedWith(t, svc, 100, 5)
	press(t, h, "[")
	if got := mustPos(t, m); got != firstFile {
		t.Fatalf("the diff is at %q, want the first file", got)
	}
	last := m.files.set.list()[len(m.files.set.list())-1].Path
	moved, wrapped := false, false
	for range 20 {
		before := m.files.diff.Cursor()
		press(t, h, "space")
		got := mustPos(t, m)
		if got != firstFile {
			moved = true
		}
		if moved && got == firstFile {
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
	if sets := svc.sets(); len(sets) != 0 {
		t.Errorf("reading on over viewed files sent %v, want none", sets)
	}
}

func TestReadOnOnlyPagesBeforeTheMarksAreRead(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	m.files.pvIn = false
	m.files.pvFailed = false
	press(t, h, "space")
	if sets := svc.sets(); len(sets) != 0 || m.files.viewed != 0 {
		t.Errorf("space before the marks were read marked a file: sent %v, %d viewed", sets, m.files.viewed)
	}
	if m.files.diff.Collapsed(firstFile) {
		t.Error("the first file is folded before the marks were read")
	}
}

func TestReadOnIsIgnoredWhileARangeIsOpen(t *testing.T) {
	t.Parallel()
	svc := withFiles()
	h, m := filedWith(t, svc, 100, 30)
	press(t, h, "[")
	down(t, h, rowContext)
	press(t, h, "V")
	if m.tray.from == nil {
		t.Fatal("V started no range")
	}
	at := m.files.diff.Cursor()
	press(t, h, "space")
	if sets := svc.sets(); len(sets) != 0 || m.files.viewed != 0 || m.files.diff.Cursor() != at {
		t.Errorf("space with a range open read on: sent %v, %d viewed", sets, m.files.viewed)
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
