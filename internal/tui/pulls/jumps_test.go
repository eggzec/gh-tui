package pulls

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"

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

// With neither the threads nor the files read, the jump waits for both and
// lands on the diff's first thread, or its last, not on the first or last in
// the order GitHub lists them, which is that of their creation: here the
// threads are t1, t2 and t3 in that order, and the diff, which follows the
// order of the changed files, has t3 (cmd/), t1 (internal/) and t2 (docs/).
func TestJumpFromOverviewReadsTheThreadsFirst(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{")": "t3", "(": "t2"} {
		t.Run(key, func(t *testing.T) {
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
			press(t, h, key)
			if m.tab != filesTab {
				t.Fatalf("%s on the Overview, before any thread was read, showed tab %d, want Files", key, m.tab)
			}
			if id := cursorThread(t, m); id != want {
				t.Errorf("%s on the Overview showed thread %q, want %q", key, id, want)
			}
		})
	}
}

// heldThreads returns the section with the modal of the sample pull request
// open on its Overview tab, and a function that takes the pages of the threads
// off the messages it is given, so that they arrive when the test says. In the
// diff, the threads come in the order t3 (cmd/), t1 (internal/) and t2
// (docs/). Reading the threads fails if failing is set.
func heldThreads(t *testing.T, failing bool) (h *host, m *detailModal, lose func(tea.Msg) bool, release func() []tea.Msg) {
	t.Helper()
	svc := newFakeService()
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: jumpThreads(), Total: 3}}
	if failing {
		svc.threadsErr = errors.New("boom")
	}
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	h = started(t, svc, 116, 60)
	press(t, h, "enter")
	m = h.modal()
	if m == nil || m.tab != overviewTab {
		t.Fatalf("the modal is %v, want it on the Overview tab", m)
	}
	var held []tea.Msg
	lose = func(msg tea.Msg) bool {
		if _, ok := msg.(threadsMsg); ok {
			held = append(held, msg)
			return true
		}
		return false
	}
	release = func() []tea.Msg {
		t.Helper()
		out := make([]tea.Msg, 0, len(held))
		for _, msg := range held {
			out = append(out, drain(t, h, h.Update(msg))...)
		}
		held = nil
		return out
	}
	return h, m, lose, release
}

// pressHolding presses k, keeping the pages of the threads back, and returns
// the text of the toasts it showed.
func pressHolding(t *testing.T, h *host, lose func(tea.Msg) bool, k string) []string {
	t.Helper()
	var toasts []string
	for _, msg := range drainLosing(t, h, h.Update(keyMsg(k)), lose) {
		if n, ok := msg.(ui.NotifyMsg); ok {
			toasts = append(toasts, n.Text)
		}
	}
	return toasts
}

// A jump pressed on the Files tab before the threads are in is dropped, with a
// toast, and doesn't land when they arrive.
func TestJumpOnFilesBeforeTheThreadsLoadIsDropped(t *testing.T) {
	t.Parallel()
	h, m, lose, release := heldThreads(t, false)
	drainLosing(t, h, m.switchTo(filesTab), lose)
	if m.tab != filesTab || m.files == nil || m.files.set.len() == 0 {
		t.Fatalf("the Files tab is not shown with its files: tab %d", m.tab)
	}
	for _, key := range []string{")", "("} {
		toasts := pressHolding(t, h, lose, key)
		if !slices.Equal(toasts, []string{"Review threads are still loading."}) {
			t.Errorf("%s before the threads loaded showed the toasts %q, want that they are loading", key, toasts)
		}
	}
	release()
	if _, on := m.files.diff.CursorBlock(); on {
		t.Errorf("a jump dropped before the threads loaded landed on thread %q once they had", cursorThread(t, m))
	}
	if len(m.back) != 0 {
		t.Errorf("a dropped jump left %d return points", len(m.back))
	}
	// The next press works.
	press(t, h, ")")
	if _, on := m.files.diff.CursorBlock(); !on {
		t.Error(") after the threads loaded did not land on a thread")
	}
}

// A jump pressed on another tab before the threads are in waits for them and
// lands once, however many times it was pressed.
func TestJumpFromOverviewBeforeTheThreadsLoadLandsOnce(t *testing.T) {
	t.Parallel()
	for presses := 1; presses <= 3; presses++ {
		t.Run(strconv.Itoa(presses), func(t *testing.T) {
			t.Parallel()
			h, m, lose, release := heldThreads(t, false)
			for range presses {
				if toasts := pressHolding(t, h, lose, ")"); len(toasts) != 0 {
					t.Errorf(") on the Overview showed the toasts %q, want none", toasts)
				}
			}
			if m.tab != overviewTab {
				t.Fatalf(") before the threads loaded showed tab %d, want to wait on the Overview", m.tab)
			}
			release()
			if m.tab != filesTab {
				t.Fatalf("the jump showed tab %d once the threads loaded, want Files", m.tab)
			}
			if id := cursorThread(t, m); id != "t3" {
				t.Errorf("%d presses landed on thread %q, want the first of the diff, t3", presses, id)
			}
			if len(m.back) != 1 {
				t.Errorf("%d presses left %d return points, want 1", presses, len(m.back))
			}
		})
	}
}

// A jump pressed on the Files tab after the threads failed to load says so, and
// doesn't land later.
func TestJumpOnFilesAfterTheThreadsFailedSaysSo(t *testing.T) {
	t.Parallel()
	h, m, lose, release := heldThreads(t, true)
	drainLosing(t, h, m.switchTo(filesTab), lose)
	release()
	if m.th.got || m.th.err == nil {
		t.Fatalf("the threads read: got %v, err %v, want it to have failed", m.th.got, m.th.err)
	}
	toasts := pressHolding(t, h, lose, ")")
	if !slices.Equal(toasts, []string{"Couldn't read the review threads."}) {
		t.Errorf(") after the threads failed showed the toasts %q, want that they couldn't be read", toasts)
	}
	release()
	if _, on := m.files.diff.CursorBlock(); on {
		t.Errorf("a jump dropped after the threads failed landed on thread %q", cursorThread(t, m))
	}
	if len(m.back) != 0 {
		t.Errorf("a dropped jump left %d return points", len(m.back))
	}
}

// A jump from another tab that waits for the threads is dropped if the user
// goes to the Files tab first, and leaves nothing to land later.
func TestJumpFromOverviewIsDroppedByLeavingForFiles(t *testing.T) {
	t.Parallel()
	h, m, lose, release := heldThreads(t, false)
	if toasts := pressHolding(t, h, lose, ")"); len(toasts) != 0 {
		t.Errorf(") on the Overview showed the toasts %q, want none", toasts)
	}
	if m.th.jumping == 0 {
		t.Fatal(") on the Overview did not wait for the threads")
	}
	drainLosing(t, h, m.switchTo(filesTab), lose)
	release()
	if m.th.jumping != 0 {
		t.Errorf("a jump is still waiting after the threads came in: %d", m.th.jumping)
	}
	if m.tab != filesTab {
		t.Errorf("the tab is %d, want Files", m.tab)
	}
	if _, on := m.files.diff.CursorBlock(); on {
		t.Errorf("a jump pressed on the Overview landed on thread %q after the user left for Files", cursorThread(t, m))
	}
	if len(m.back) != 0 {
		t.Errorf("a dropped jump left %d return points", len(m.back))
	}
}

// A jump from another tab whose read of the threads fails says so and leaves
// nothing waiting.
func TestJumpFromOverviewWhenTheThreadsFailSaysSo(t *testing.T) {
	t.Parallel()
	h, m, lose, release := heldThreads(t, true)
	pressHolding(t, h, lose, ")")
	if m.th.jumping == 0 {
		t.Fatal(") on the Overview did not wait for the threads")
	}
	var toasts []string
	for _, msg := range release() {
		if n, ok := msg.(ui.NotifyMsg); ok {
			toasts = append(toasts, n.Text)
		}
	}
	if !slices.Equal(toasts, []string{"Couldn't read the review threads."}) {
		t.Errorf("a failed read showed the toasts %q, want that the threads couldn't be read", toasts)
	}
	if m.th.jumping != 0 {
		t.Errorf("a jump is still waiting after the read failed: %d", m.th.jumping)
	}
	if m.tab != overviewTab {
		t.Errorf("the tab is %d, want to stay on the Overview", m.tab)
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
