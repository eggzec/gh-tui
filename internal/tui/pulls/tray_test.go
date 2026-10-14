package pulls

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// memTray is a TrayStore in memory that counts its saves.
type memTray struct {
	mu     sync.Mutex
	kept   map[string][]tray.Draft
	saves  int
	saveEr error
	loadEr error
}

func (s *memTray) Load(key string) ([]tray.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadEr != nil {
		return nil, s.loadEr
	}
	return append([]tray.Draft(nil), s.kept[key]...), nil
}

func (s *memTray) Save(key string, drafts []tray.Draft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveEr != nil {
		return s.saveEr
	}
	if s.kept == nil {
		s.kept = map[string][]tray.Draft{}
	}
	s.kept[key] = append([]tray.Draft(nil), drafts...)
	s.saves++
	return nil
}

// drafts returns what is kept for #142 of the test repository.
func (s *memTray) drafts() []tray.Draft {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kept[tray.Key(repo.String(), 142)]
}

// drafting returns the modal of #142 on the diff of its Files tab, with the
// threads ths and the store, 116 by 60.
func drafting(t *testing.T, store TrayStore, ths ...core.Thread) (*host, *detailModal) {
	t.Helper()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: ths, Total: len(ths)}}
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	h := started(t, svc, 116, 60, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)), WithTray(store))
	h.SetTheme(theme(true))
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter opened no pull request")
	}
	drain(t, h, m.switchTo(filesTab))
	if m.files == nil {
		t.Fatal("the Files tab was not built")
	}
	return h, m
}

// typeText sends text to the open composer, a key at a time.
func typeText(t *testing.T, h *host, text string) {
	t.Helper()
	for _, r := range text {
		press(t, h, string(r))
	}
}

// down moves the cursor of the diff n rows down.
func down(t *testing.T, h *host, n int) {
	t.Helper()
	for range n {
		press(t, h, "down")
	}
}

// The rows of the first file of sampleFiles, from the cursor on its header:
// the hunk header, then line 10 of both sides, and the deleted lines 11 and
// 12.
const (
	rowContext = 2
	rowDeleted = 3
)

func TestTrayCommentOnALine(t *testing.T) {
	t.Parallel()
	store := &memTray{}
	h, m := drafting(t, store)
	// A line that was deleted is drafted on the left side.
	down(t, h, rowDeleted)
	press(t, h, "c")
	typeText(t, h, "gone")
	press(t, h, "esc")
	got := store.drafts()
	if len(got) != 1 || got[0].Side != tray.Left || got[0].Line != 11 {
		t.Fatalf("drafts = %+v, want one on the left side of line 11", got)
	}

	press(t, h, "up")
	press(t, h, "c")
	if m.tray.comp == nil {
		t.Fatal("c on a line opened no composer")
	}
	typeText(t, h, "why cfg")
	press(t, h, "esc")
	if m.tray.comp != nil {
		t.Fatal("esc left the composer open")
	}
	got = store.drafts()
	if len(got) != 2 {
		t.Fatalf("kept %d drafts, want 2: %+v", len(got), got)
	}
	d := got[1]
	if d.Path != "cmd/gh-tui/main.go" || d.Side != tray.Right || d.Line != 10 || d.StartLine != 0 || d.ThreadID != "" || d.Commit != "a1b2c3d" || d.Body != "why cfg" || d.Created.IsZero() {
		t.Errorf("draft = %+v, want one on the new side of line 10 of main.go at a1b2c3d", d)
	}
	lines := diffLines(m)
	at := lineWith(lines, "why cfg")
	if at < 2 || !strings.Contains(lines[at-1], "draft") || !strings.Contains(lines[at-2], "cfg := load()") {
		t.Fatalf("the draft is not right under line 10:\n%s", strings.Join(lines, "\n"))
	}
}

func TestTrayCommentNeedsALine(t *testing.T) {
	t.Parallel()
	store := &memTray{}
	h, m := drafting(t, store)
	press(t, h, "c")
	if m.tray.comp != nil {
		t.Error("c on a file header opened a composer")
	}
	press(t, h, "V")
	if m.tray.from != nil {
		t.Error("V on a file header started a range")
	}
	down(t, h, rowContext)
	press(t, h, "c")
	press(t, h, "esc")
	if m.tray.comp != nil || len(store.drafts()) != 0 || store.saves != 0 {
		t.Errorf("an empty composer left %+v, %d saves; want no draft", store.drafts(), store.saves)
	}
}

func TestTrayRange(t *testing.T) {
	t.Parallel()
	store := &memTray{}
	h, m := drafting(t, store)
	down(t, h, rowDeleted)
	press(t, h, "V")
	if m.tray.from == nil || m.tray.from.Line != 11 {
		t.Fatalf("V started %+v, want a range at line 11", m.tray.from)
	}
	press(t, h, "V")
	if m.tray.from != nil {
		t.Error("V again didn't clear the range")
	}
	press(t, h, "V")
	// A range that crosses the sides is refused, and stays started.
	down(t, h, 2)
	press(t, h, "c")
	if m.tray.comp != nil || m.tray.from == nil {
		t.Fatalf("c on the other side than the range's opened %+v, want nothing and the range kept", m.tray.comp)
	}
	press(t, h, "up")
	press(t, h, "c")
	if m.tray.comp == nil {
		t.Fatal("c on the end of the range opened no composer")
	}
	typeText(t, h, "both")
	press(t, h, "esc")
	if m.tray.from != nil {
		t.Error("the range stayed started after the comment")
	}
	got := store.drafts()
	if len(got) != 1 || got[0].StartLine != 11 || got[0].Line != 12 || got[0].Side != tray.Left {
		t.Fatalf("drafts = %+v, want one on lines 11 to 12 of the left side", got)
	}
	if v := strings.Join(diffLines(m), "\n"); !strings.Contains(v, "lines 11-12") {
		t.Errorf("the draft row doesn't say its range:\n%s", v)
	}
}

func TestTrayReplyEditDelete(t *testing.T) {
	t.Parallel()
	store := &memTray{}
	h, m := drafting(t, store, review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err hides a full disk."))
	if !m.files.diff.SeekBlock("t1") {
		t.Fatal("the thread is not in the diff")
	}
	press(t, h, "c")
	if m.tray.comp == nil || m.tray.comp.d.ThreadID != "t1" {
		t.Fatalf("c on a thread opened %+v, want a reply to t1", m.tray.comp)
	}
	typeText(t, h, "agreed")
	press(t, h, "esc")
	got := store.drafts()
	if len(got) != 1 || got[0].ThreadID != "t1" || got[0].Path != "internal/cache/disk.go" || got[0].Body != "agreed" {
		t.Fatalf("drafts = %+v, want a reply to t1", got)
	}
	lines := diffLines(m)
	at := lineWith(lines, "draft")
	if at < 1 || !strings.Contains(lines[at], "reply") || !strings.Contains(strings.Join(lines[:at], "\n"), "Swallowing err") {
		t.Fatalf("the reply is not under its thread:\n%s", strings.Join(lines, "\n"))
	}

	// Moving onto the draft row with j from the line above reaches it too,
	// and c there edits it with its text in the composer.
	id := draftPrefix + got[0].ID
	m.files.diff.SeekBlock("t1")
	press(t, h, "j")
	if b, ok := m.files.diff.CursorBlock(); !ok || b.ID != id {
		t.Fatalf("j from the thread put the cursor on %+v, want the draft", b)
	}
	press(t, h, "c")
	if m.tray.comp == nil || m.tray.comp.c.Value() != "agreed" {
		t.Fatalf("c on the draft reached by j opened %+v, want its text to edit", m.tray.comp)
	}
	press(t, h, "esc")

	// c on the draft edits it in place.
	m.files.diff.SeekBlock(id)
	press(t, h, "c")
	if m.tray.comp == nil || m.tray.comp.c.Value() != "agreed" {
		t.Fatalf("c on a draft opened %+v, want its text to edit", m.tray.comp)
	}
	typeText(t, h, " too")
	press(t, h, "esc")
	got = store.drafts()
	if len(got) != 1 || got[0].Body != "agreed too" || got[0].ThreadID != "t1" {
		t.Errorf("drafts after the edit = %+v, want the one, edited", got)
	}

	// D deletes it, and only on a draft.
	m.files.diff.SeekBlock("t1")
	press(t, h, "D")
	if len(store.drafts()) != 1 {
		t.Error("D on a thread deleted the draft")
	}
	m.files.diff.SeekBlock(id)
	press(t, h, "D")
	if len(store.drafts()) != 0 || len(m.tray.drafts) != 0 {
		t.Errorf("D left %+v on disk and %+v in the modal", store.drafts(), m.tray.drafts)
	}
	if v := strings.Join(diffLines(m), "\n"); strings.Contains(v, "agreed") {
		t.Errorf("the deleted draft is still shown:\n%s", v)
	}
}

// Drafts come back when the pull request is opened again, and the text of an
// open composer is kept when the app quits.
func TestTrayKeptOnDiskAndOnQuit(t *testing.T) {
	t.Parallel()
	store := &memTray{}
	h, m := drafting(t, store)
	down(t, h, rowContext)
	press(t, h, "c")
	typeText(t, h, "unsent")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	got := store.drafts()
	if len(got) != 1 || got[0].Body != "unsent" || got[0].Line != 10 {
		t.Fatalf("Save kept %+v, want the open composer's text", got)
	}

	_, again := drafting(t, store)
	if len(again.tray.drafts) != 1 || again.tray.drafts[0].ID != got[0].ID {
		t.Fatalf("the reopened modal has %+v, want the kept draft", again.tray.drafts)
	}
	if v := strings.Join(diffLines(again), "\n"); !strings.Contains(v, "unsent") {
		t.Errorf("the reopened diff shows no draft:\n%s", v)
	}

	// Saving again with nothing open, or nothing typed, writes nothing.
	saves := store.saves
	again.tray.comp = nil
	if err := again.Save(); err != nil || store.saves != saves {
		t.Errorf("Save without a composer = %v with %d saves, want none", err, store.saves-saves)
	}
}

// Without a store the drafts last for the modal, and a store that fails
// tells so.
func TestTrayWithoutStoreAndFailingStore(t *testing.T) {
	t.Parallel()
	h, m := drafting(t, nil)
	down(t, h, rowContext)
	press(t, h, "c")
	typeText(t, h, "x")
	press(t, h, "esc")
	if len(m.tray.drafts) != 1 {
		t.Errorf("without a store the modal kept %+v, want the draft", m.tray.drafts)
	}

	store := &memTray{saveEr: errors.New("disk full")}
	h, _ = drafting(t, store)
	down(t, h, rowContext)
	press(t, h, "c")
	typeText(t, h, "x")
	var told bool
	for _, msg := range press(t, h, "esc") {
		if n, ok := msg.(ui.NotifyMsg); ok && strings.Contains(n.Text, "disk full") {
			told = true
		}
	}
	if !told {
		t.Error("a failed save told the user nothing")
	}
}

// A change is written when it is made, so a delete that follows a save leaves
// the later state, whatever order commands would have run in.
func TestTrayWritesInOrder(t *testing.T) {
	t.Parallel()
	store := &memTray{}
	_, m := drafting(t, store)
	m.tray.drafts = []tray.Draft{{ID: "a", Body: "x"}}
	if cmd := m.saveTray(); cmd != nil {
		t.Fatalf("a save that worked returned %v", cmd)
	}
	if got := store.drafts(); len(got) != 1 {
		t.Fatalf("kept %+v before any command ran, want the draft", got)
	}
	m.tray.drafts = nil
	m.saveTray()
	if got := store.drafts(); len(got) != 0 {
		t.Errorf("kept %+v after the delete, want none", got)
	}
}

// A file that could not be read is not written over, but one that was read
// and is damaged is.
func TestTrayKeepsAFileItCouldNotRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		err   error
		saves int
	}{
		{"denied", errors.New("read drafts: permission denied"), 0},
		{"damaged", fmt.Errorf("read drafts: %w: bad json", tray.ErrDamaged), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &memTray{loadEr: tc.err}
			h, _ := drafting(t, store)
			down(t, h, rowContext)
			press(t, h, "c")
			typeText(t, h, "x")
			press(t, h, "esc")
			if store.saves != tc.saves {
				t.Errorf("%d saves after a load that failed with %v, want %d", store.saves, tc.err, tc.saves)
			}
		})
	}
}

// A reply is on the side of the thread it answers.
func TestTrayReplyKeepsTheThreadsSide(t *testing.T) {
	t.Parallel()
	th := review("t1", "internal/cache/disk.go", 45, "hubot", "Gone.")
	th.Side = core.SideLeft
	store := &memTray{}
	h, m := drafting(t, store, th)
	if !m.files.diff.SeekBlock("t1") {
		t.Fatal("the thread is not in the diff")
	}
	press(t, h, "c")
	typeText(t, h, "ok")
	press(t, h, "esc")
	got := store.drafts()
	if len(got) != 1 || got[0].Side != tray.Left || got[0].Line != 45 {
		t.Errorf("drafts = %+v, want a reply on the left side of line 45", got)
	}
}

// The head comes from the pull request when the files have none, and without
// one there is no comment.
func TestTrayHeadFallback(t *testing.T) {
	t.Parallel()
	store := &memTray{}
	h, m := drafting(t, store)
	m.files.head = ""
	down(t, h, rowContext)
	press(t, h, "c")
	if m.tray.comp == nil || m.tray.comp.d.Commit != "a1b2c3d" {
		t.Fatalf("composer = %+v, want a draft at the head of the pull request", m.tray.comp)
	}
	press(t, h, "esc")

	m.tray.comp = nil
	m.detail.HeadSHA = ""
	var told bool
	for _, msg := range press(t, h, "c") {
		if n, ok := msg.(ui.NotifyMsg); ok && strings.Contains(n.Text, "head") {
			told = true
		}
	}
	if m.tray.comp != nil || !told {
		t.Errorf("c without a head opened %+v and told the user: %v, want a toast only", m.tray.comp, told)
	}
}

// The tray's keys reach their actions on the diff and don't take the keys of
// the composer, which types them.
func TestTrayKeys(t *testing.T) {
	t.Parallel()
	h, m := drafting(t, &memTray{})
	for k, want := range map[string]string{"c": "draft comment", "V": "start range"} {
		if got := winner(m.KeyLayers(), k); !strings.HasSuffix(got, ": "+want) {
			t.Errorf("%s reaches %q on the diff, want %q", k, got, want)
		}
	}
	if got := winner(m.KeyLayers(), "D"); strings.HasSuffix(got, "delete draft") {
		t.Errorf("D reaches %q off a draft, want the key disabled", got)
	}
	down(t, h, rowContext)
	press(t, h, "c")
	for _, k := range []string{"c", "V", "D"} {
		if got := winner(m.KeyLayers(), k); strings.Contains(got, "draft") || strings.Contains(got, "range") {
			t.Errorf("%s reaches %q while the composer is open, want the composer", k, got)
		}
	}
	typeText(t, h, "cVD")
	if got := m.tray.comp.c.Value(); got != "cVD" {
		t.Errorf("the composer holds %q, want the keys typed", got)
	}
}
