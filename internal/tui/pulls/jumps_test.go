package pulls

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// jumpThreads are threads in three files of the sample pull request, one of
// them resolved.
func jumpThreads() []core.Thread {
	done := review("t3", "cmd/gh-tui/main.go", 12, "hubot", "Unused.")
	done.Resolved = true
	return []core.Thread{
		review("t1", "internal/cache/disk.go", 45, "hubot", "Swallowing err hides a full disk."),
		review("t2", "docs/design.md", 0, "octocat", "Needs a section on the disk layout."),
		done,
	}
}

// cursorThread returns the id of the thread the diff's cursor is on.
func cursorThread(t *testing.T, m *detailModal) string {
	t.Helper()
	b, on := m.files.diff.CursorBlock()
	if !on {
		t.Fatalf("the diff's cursor is not on a thread")
	}
	return b.ID
}

func TestJumpThreadsInTheDiffWrap(t *testing.T) {
	t.Parallel()
	h, m := onFilesWith(t, jumpThreads()...)
	next := make([]string, 0, 4)
	for range 4 {
		press(t, h, ")")
		next = append(next, cursorThread(t, m))
	}
	if next[3] != next[0] {
		t.Errorf(") four times visited %v, want the fourth to wrap to the first", next)
	}
	got := slices.Clone(next[:3])
	slices.Sort(got)
	if !slices.Equal(got, []string{"t1", "t2", "t3"}) {
		t.Errorf(") visited %v, want every thread once, resolved ones too", next[:3])
	}
	// Backwards undoes the steps, and wraps to the last.
	press(t, h, "(")
	if id := cursorThread(t, m); id != next[2] {
		t.Errorf("( from the first thread went to %q, want the last, %q", id, next[2])
	}
	press(t, h, "(")
	if id := cursorThread(t, m); id != next[1] {
		t.Errorf("( went to %q, want %q", id, next[1])
	}
	if m.files.focus != diffPane {
		t.Errorf("a jump left the focus on pane %d, want the diff", m.files.focus)
	}
}

func TestJumpThreadsInTheConversationWrap(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: jumpThreads(), Total: 3}}
	h, m := threaded(t, svc, 116, 37, config.IconsUnicode)
	// Two threads are unresolved.
	rows := make([]int, 0, 3)
	for range 3 {
		press(t, h, ")")
		rows = append(rows, m.th.cursorRow())
	}
	if !slices.Equal(rows, []int{1, 0, 1}) {
		t.Errorf(") visited the rows %v, want 1, then wrapping to 0 and 1", rows)
	}
	press(t, h, "(")
	if r := m.th.cursorRow(); r != 0 {
		t.Errorf("( went to row %d, want 0", r)
	}
	press(t, h, "(")
	if r := m.th.cursorRow(); r != 1 {
		t.Errorf("( from the first row went to %d, want the last, 1", r)
	}
	if m.tab != conversationTab {
		t.Errorf("a jump moved to tab %d, want to stay on the Conversation tab", m.tab)
	}
}

func TestJumpFromOverviewShowsTheFirstAndLastThread(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: jumpThreads(), Total: 3}}
	h, m := threaded(t, svc, 116, 60, config.IconsUnicode)
	// Walk the diff to learn its order.
	drain(t, h, m.switchTo(filesTab))
	order := make([]string, 0, 3)
	for range 3 {
		press(t, h, ")")
		order = append(order, cursorThread(t, m))
	}
	for key, want := range map[string]string{")": order[0], "(": order[2]} {
		drain(t, h, m.switchTo(overviewTab))
		press(t, h, key)
		if m.tab != filesTab {
			t.Fatalf("%s on the Overview showed tab %d, want Files", key, m.tab)
		}
		if id := cursorThread(t, m); id != want {
			t.Errorf("%s on the Overview showed thread %q, want %q", key, id, want)
		}
	}
}

func TestJumpFromOverviewReadsTheThreadsFirst(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: jumpThreads(), Total: 3}}
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	h := started(t, svc, 116, 60)
	press(t, h, "enter")
	m := h.modal()
	if m == nil || m.tab != overviewTab {
		t.Fatalf("the modal is %v, want it on the Overview tab", m)
	}
	press(t, h, ")")
	if m.tab != filesTab {
		t.Fatalf(") on the Overview, before any thread was read, showed tab %d, want Files", m.tab)
	}
	if id := cursorThread(t, m); id == "" {
		t.Error("the diff's cursor is on no thread")
	}
}

func TestJumpWithoutThreadsSaysSo(t *testing.T) {
	t.Parallel()
	h, m := onFilesWith(t)
	drain(t, h, m.switchTo(overviewTab))
	press(t, h, ")")
	if m.tab != overviewTab {
		t.Errorf(") with no threads showed tab %d, want to stay on the Overview", m.tab)
	}
	if len(m.back) != 0 {
		t.Errorf("a jump that went nowhere left %d return points", len(m.back))
	}
}

func TestBackspaceReturnsToTheConversationRow(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	h, m := threaded(t, svc, 100, 30, config.IconsUnicode)
	press(t, h, "j")
	row := m.th.cursorRow()
	press(t, h, "enter")
	if m.tab != filesTab {
		t.Fatalf("enter showed tab %d, want Files", m.tab)
	}
	back(t, h, m)
	if m.tab != conversationTab {
		t.Fatalf("backspace showed tab %d, want the Conversation tab", m.tab)
	}
	if got := m.th.cursorRow(); got != row {
		t.Errorf("backspace put the cursor on row %d, want %d", got, row)
	}
	if len(m.back) != 0 {
		t.Errorf("the return point was kept: %v", m.back)
	}
}

func TestBackspaceAfterAJumpFromTheOverview(t *testing.T) {
	t.Parallel()
	h, m := onFilesWith(t, jumpThreads()...)
	drain(t, h, m.switchTo(overviewTab))
	press(t, h, ")")
	if m.tab != filesTab {
		t.Fatalf(") showed tab %d, want Files", m.tab)
	}
	press(t, h, ")")
	if len(m.back) != 1 {
		t.Errorf("a jump on the Files tab left %d return points, want 1", len(m.back))
	}
	back(t, h, m)
	if m.tab != overviewTab {
		t.Errorf("backspace showed tab %d, want the Overview", m.tab)
	}
}

// back presses the back key as the app does, which asks the modal first.
func back(t *testing.T, h *host, m *detailModal) {
	t.Helper()
	cmd, ok := m.Act(ui.ActBack)
	if !ok {
		t.Fatal("the back key was not taken by the modal")
	}
	drain(t, h, cmd)
}

func TestBackspaceWithoutReturnPoint(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": firstPage()}
	_, m := threaded(t, svc, 100, 30, config.IconsUnicode)
	// Cycling the tabs leaves nothing to return to.
	m.cycle(1)
	if cmd, ok := m.Act(ui.ActBack); ok || cmd != nil {
		t.Errorf("the back key with no return point was taken (%v)", ok)
	}
	m.back = []returnPoint{{tab: overviewTab}}
	m.Discard()
	if m.back != nil {
		t.Errorf("closing the modal kept the return points %v", m.back)
	}
}
