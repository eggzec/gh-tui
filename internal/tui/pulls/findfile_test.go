package pulls

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// typeQuery types the letters of q into the finder.
func typeQuery(t *testing.T, h *host, q string) {
	t.Helper()
	for _, r := range q {
		press(t, h, string(r))
	}
}

func TestFindFileJumpsToTheFile(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "J")
	press(t, h, "ctrl+p")
	if m.find == nil {
		t.Fatal("ctrl+p opened no finder")
	}
	if got := m.find.Total(); got != len(sampleFiles()) {
		t.Errorf("the finder lists %d files, want the %d the pull request changes", got, len(sampleFiles()))
	}
	if h.modal() != m || len(h.modals) != 1 {
		t.Error("the finder opened as a modal of its own")
	}
	typeQuery(t, h, "yaml")
	if it, ok := m.find.Selected(); !ok || it.Path != "internal/config/default.yaml" {
		t.Fatalf("typing yaml selected %v, want the yaml file", it)
	}
	press(t, h, "enter")
	if m.find != nil {
		t.Error("choosing a file left the finder open")
	}
	if cur, _ := m.files.diff.CurrentFile(); cur.Path != "internal/config/default.yaml" {
		t.Errorf("the diff is in %q, want the file chosen", cur.Path)
	}
	if got := pathOf(m); got != "internal/config/default.yaml" {
		t.Errorf("the tree is on %q, want it to follow the diff", got)
	}
	if m.files.focus != diffPane {
		t.Error("the diff doesn't have the focus after the jump")
	}
}

func TestFindFileFromAnotherTabReadsTheFiles(t *testing.T) {
	svc := newFakeService()
	svc.filePage = 2
	h, m := filed(t, svc, 100, 30)
	if m.tab != conversationTab || m.files != nil {
		t.Fatalf("the modal starts on tab %d with files %v, want the conversation and none", m.tab, m.files)
	}
	read := h.Update(keyMsg("ctrl+p"))
	if v := strings.Join(plain(m), "\n"); !strings.Contains(v, "Loading files") {
		t.Errorf("the finder doesn't say it loads while the files are read:\n%s", v)
	}
	drain(t, h, read)
	if m.find.Loading() || m.find.Total() != len(sampleFiles()) {
		t.Errorf("the finder holds %d files, loading %v, want all of them", m.find.Total(), m.find.Loading())
	}
	// The files were read behind the finder, a page at a time, without
	// the Files tab.
	if m.files != nil {
		t.Fatal("opening the finder built the Files tab")
	}
	if n := svc.fileReadCount(); n != 4 {
		t.Errorf("the finder read %d pages of 7 files at 2 a page, want 4", n)
	}
}

func TestFindFileInALaterPage(t *testing.T) {
	svc := newFakeService()
	svc.filePage = 2
	h, m := filed(t, svc, 100, 30)
	press(t, h, "ctrl+p")
	if m.find == nil || m.find.Loading() {
		t.Fatalf("the finder is %v, want it open and loaded", m.find)
	}
	if got := m.find.Total(); got != len(sampleFiles()) {
		t.Fatalf("the finder lists %d files, want all %d, not the first page", got, len(sampleFiles()))
	}
	typeQuery(t, h, "logo")
	press(t, h, "enter")
	if m.tab != filesTab || m.files == nil || m.find != nil {
		t.Fatalf("choosing a file left tab %d with files %v and finder %v, want the Files tab", m.tab, m.files, m.find)
	}
	// The diff reads pages until the file arrives, then puts the cursor there.
	if cur, ok := m.files.diff.CurrentFile(); !ok || cur.Path != "img/logo.png" {
		t.Errorf("the diff is in %q, want img/logo.png from a later page", cur.Path)
	}
	if got := pathOf(m); got != "img/logo.png" {
		t.Errorf("the tree is on %q, want img/logo.png", got)
	}
}

func TestFindFileUsesTheFilesTheDiffRead(t *testing.T) {
	svc := newFakeService()
	h, m := filed(t, svc, 100, 30)
	press(t, h, "[")
	reads := svc.fileReadCount()
	press(t, h, "ctrl+p")
	if got := svc.fileReadCount(); got != reads {
		t.Errorf("the finder read %d more pages, want it to list the files the diff holds", got-reads)
	}
	if got := m.find.Total(); got != len(sampleFiles()) {
		t.Errorf("the finder lists %d files, want %d", got, len(sampleFiles()))
	}
}

func TestFindFileEscClosesOnlyTheFinder(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "ctrl+p")
	typeQuery(t, h, "disk")
	press(t, h, "esc")
	if m.find != nil {
		t.Fatal("esc left the finder open")
	}
	if h.modal() != m || m.closed || m.tab != filesTab {
		t.Fatal("esc in the finder closed more than the finder")
	}
	// The next one finds again from an empty query.
	press(t, h, "ctrl+p")
	if q := m.find.Query(); q != "" {
		t.Errorf("the finder opened with %q typed, want it empty", q)
	}
	press(t, h, "esc")
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("esc after the finder didn't close the modal")
	}
}

func TestFindFileBackspaceEditsTheQuery(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "ctrl+p")
	typeQuery(t, h, "dis")
	press(t, h, "backspace")
	if m.find == nil || m.find.Query() != "di" {
		t.Errorf("backspace left the query %q, want it to delete a letter and keep the finder", m.find.Query())
	}
	if _, ok := m.Act("owner"); ok {
		t.Error("the author key acted over the finder")
	}
}

func TestFindFileNeedsTheHead(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	m.detail.HeadSHA = ""
	press(t, h, "ctrl+p")
	if m.find != nil {
		t.Error("ctrl+p opened the finder without a head to read the files at")
	}
	for _, l := range m.KeyLayers() {
		for _, b := range l.Bindings {
			if b.Help().Desc == "find file" && b.Enabled() {
				t.Error("help lists find file while it does nothing")
			}
		}
	}
}

func TestFindFileFailureIsInline(t *testing.T) {
	svc := newFakeService()
	h, m := filed(t, svc, 100, 30)
	svc.filesErr = errors.New("boom")
	press(t, h, "ctrl+p")
	if m.find == nil || m.find.Err() == nil {
		t.Fatal("a failed read left no error in the finder")
	}
	if v := strings.Join(plain(m), "\n"); !strings.Contains(v, "Something went wrong") {
		t.Errorf("the finder says nothing of the failure:\n%s", v)
	}
	press(t, h, "esc")
	svc.filesErr = nil
	press(t, h, "ctrl+p")
	if m.find == nil || m.find.Err() != nil || m.find.Total() != len(sampleFiles()) {
		t.Error("opening the finder again didn't read the files again")
	}
}

func TestFindFileHelp(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	has := func(layers []keyhelp.Layer, desc string) bool {
		for _, l := range layers {
			for _, b := range l.Bindings {
				if b.Help().Desc == desc && b.Enabled() {
					return true
				}
			}
		}
		return false
	}
	for _, tab := range []string{"[", "]", "]"} {
		if !has(m.KeyLayers(), "find file") {
			t.Errorf("help on tab %d doesn't list find file", m.tab)
		}
		press(t, h, tab)
	}
	press(t, h, "ctrl+p")
	layers := m.KeyLayers()
	if len(layers) != 1 || layers[0].Context != "finder" || !layers[0].Typing {
		t.Errorf("the finder's help is %+v, want the typing layer of the finder alone", layers)
	}
}

func TestFindFileView(t *testing.T) {
	svc := newFakeService()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	h := started(t, svc, 76, 21, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	press(t, h, "enter")
	press(t, h, "ctrl+p")
	typeQuery(t, h, "disk")
	golden.RequireEqual(t, h.modal().View())
}

// notices returns the toasts among msgs.
func notices(msgs []tea.Msg) []ui.NotifyMsg {
	var out []ui.NotifyMsg
	for _, m := range msgs {
		if n, ok := m.(ui.NotifyMsg); ok {
			out = append(out, n)
		}
	}
	return out
}

func TestFindFileClosedWithAReadInFlight(t *testing.T) {
	svc := newFakeService()
	h, m := filed(t, svc, 100, 30)
	read := h.Update(keyMsg("ctrl+p"))
	press(t, h, "esc")
	if m.find != nil {
		t.Fatal("esc left the finder open")
	}
	// The read ends after the finder closed, and changes nothing.
	msgs := drain(t, h, read)
	if m.find != nil || m.tab != conversationTab || m.files != nil || h.modal() != m {
		t.Errorf("the late read left finder %v, tab %d, files %v", m.find, m.tab, m.files)
	}
	if n := notices(msgs); len(n) > 0 {
		t.Errorf("the late read said %v, want nothing", n)
	}
	press(t, h, "ctrl+p")
	if m.find == nil || m.find.Total() != len(sampleFiles()) {
		t.Error("the finder opened again doesn't list the files")
	}
}

func TestFindFileHeadMovedWhileOpen(t *testing.T) {
	svc := newFakeService()
	svc.changedAt = map[string][]core.CommitFile{"b2c3d4e": sampleFiles()[:2]}
	h, m := filed(t, svc, 100, 30)
	press(t, h, "ctrl+p")
	typeQuery(t, h, "logo")
	svc.pulls[0].HeadSHA = "b2c3d4e"
	msgs := drain(t, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if m.detail.HeadSHA != "b2c3d4e" {
		t.Fatalf("the sync left the head at %q", m.detail.HeadSHA)
	}
	want := ui.NotifyMsg{Level: toast.Warning, Text: "The pull request changed; find again."}
	if n := notices(msgs); len(n) != 1 || n[0] != want {
		t.Errorf("the toasts are %v, want %v", n, want)
	}
	if m.find != nil {
		t.Fatal("the finder stayed open with the files of the old head")
	}
	if m.tab != conversationTab || m.files != nil {
		t.Errorf("the finder's closing changed tab %d or built files %v", m.tab, m.files)
	}
	press(t, h, "ctrl+p")
	if m.find == nil || m.find.Total() != 2 {
		t.Errorf("the finder opened again lists %d files, want the 2 of the new head", m.find.Total())
	}
}

func TestFindFileChoosesAFileNoLongerChanged(t *testing.T) {
	svc := newFakeService()
	h, m := filed(t, svc, 100, 30)
	press(t, h, "ctrl+p")
	typeQuery(t, h, "logo")
	// The pull request changes fewer files by the time the diff reads them.
	svc.changed = sampleFiles()[:2]
	want := ui.NotifyMsg{Level: toast.Warning, Text: "img/logo.png isn't changed in this pull request."}
	msgs := press(t, h, "enter")
	if n := notices(msgs); len(n) != 1 || n[0] != want {
		t.Errorf("the toasts are %v, want %v", n, want)
	}
	if m.tab != filesTab || m.files == nil || m.files.settling != "" {
		t.Errorf("tab %d, files %v: want the Files tab, no longer settling", m.tab, m.files)
	}
	if m.files.diff.Files() != 2 {
		t.Errorf("the diff holds %d files, want all 2 read", m.files.diff.Files())
	}

	// With every page read, a file that isn't there says so at once.
	msgs = drain(t, h, m.chooseFile("nope.go"))
	if n := notices(msgs); len(n) != 1 || n[0].Text != "nope.go isn't changed in this pull request." {
		t.Errorf("choosing a file the diff lacks gave %v", n)
	}
	if m.files.settling != "" {
		t.Error("the diff still waits for a file it read every page without")
	}
}

func TestFindFileSettlingYieldsToTheTree(t *testing.T) {
	h, m := filed(t, newFakeService(), 100, 30)
	press(t, h, "[")
	press(t, h, "ctrl+p")
	typeQuery(t, h, "disk_test")
	press(t, h, "enter")
	const chosen = "internal/cache/disk_test.go"
	if m.files.settling != "" || pathOf(m) != chosen {
		t.Fatalf("after the jump the tree is on %q, settling %q", pathOf(m), m.files.settling)
	}

	// While the tree still waits for the chosen file's branch to load,
	// moving in it ends the wait, so later loads don't pull it back.
	m.files.settling = chosen
	press(t, h, "1")
	if m.files.settling != "" {
		t.Error("focusing the tree left the finder's file settling")
	}
	m.files.settling = chosen
	press(t, h, "k")
	moved := pathOf(m)
	if m.files.settling != "" || moved == chosen {
		t.Fatalf("k left the tree on %q, settling %q, want it moved and free", moved, m.files.settling)
	}
	drain(t, h, m.follow())
	drain(t, h, h.Update(ui.SyncMsg{Key: pulls.SyncKey(repo)}))
	if got := pathOf(m); got != moved {
		t.Errorf("a later load pulled the tree back to %q from %q", got, moved)
	}

	// A key in the diff ends it too.
	m.files.settling = chosen
	press(t, h, "2")
	if m.files.settling != "" {
		t.Error("focusing the diff left the finder's file settling")
	}
}
