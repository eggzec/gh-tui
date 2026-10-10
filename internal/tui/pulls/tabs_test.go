package pulls

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// opened returns a section with checks, whose icons are plain unicode, and
// the modal of its first pull request, on the Overview it opens on.
func opened(tb testing.TB, svc *fakeService, c *fakeChecks, opts ...checks.Option) (*host, *detailModal) {
	tb.Helper()
	h := started(tb, svc, 100, 30, WithChecks(c, opts...), WithIcons(ui.NewIcons(config.IconsUnicode)))
	press(tb, h, "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter opened no pull request")
	}
	return h, m
}

// tabbed is opened with the modal moved to its conversation.
func tabbed(tb testing.TB, svc *fakeService, c *fakeChecks, opts ...checks.Option) (*host, *detailModal) {
	tb.Helper()
	h, m := opened(tb, svc, c, opts...)
	m.tab = conversationTab
	return h, m
}

func TestTabsCycle(t *testing.T) {
	h, m := opened(t, newFakeService(), &fakeChecks{})
	if m.tab != overviewTab || m.checks != nil || m.files != nil {
		t.Fatalf("the modal opened on tab %d with checks %v and files %v, want the overview and neither built", m.tab, m.checks, m.files)
	}
	names, active := m.Tabs()
	if want := []string{"Overview", "Files 10", "Conversation", "Checks ✗1"}; !slices.Equal(names, want) || active != 0 {
		t.Errorf("tabs = %v at %d, want %v at 0", names, active, want)
	}
	press(t, h, "]")
	if m.tab != filesTab {
		t.Fatalf("] showed tab %d, want the Files tab", m.tab)
	}
	press(t, h, "]")
	if m.tab != conversationTab {
		t.Fatalf("] showed tab %d, want the conversation", m.tab)
	}
	press(t, h, "]")
	if m.tab != checksTab || m.checks == nil {
		t.Fatalf("] showed tab %d with checks %v, want the Checks tab built", m.tab, m.checks)
	}
	if names, active := m.Tabs(); active != 3 || names[3] != "Checks ✗1" {
		t.Errorf("tabs = %v at %d, want Checks active with the failing check counted", names, active)
	}
	press(t, h, "]")
	if m.tab != overviewTab {
		t.Error("] on the last tab didn't wrap to the first")
	}
	press(t, h, "[")
	if m.tab != checksTab {
		t.Error("[ on the first tab didn't wrap to the last")
	}
	press(t, h, "[")
	press(t, h, "[")
	if m.tab != filesTab {
		t.Error("[ from the conversation didn't go to the files")
	}
	press(t, h, "[")
	if m.tab != overviewTab {
		t.Error("[ from the files didn't go to the overview")
	}
	press(t, h, "C")
	if m.tab != checksTab {
		t.Error("C didn't jump to the Checks tab")
	}
}

// The modal opens on the Overview, but one asked for the checks opens on
// them.
func TestOpensOnTheOverviewUnlessAskedForChecks(t *testing.T) {
	h := started(t, newFakeService(), 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 142}))
	if m := h.modal(); m == nil || m.tab != overviewTab {
		t.Fatalf("OpenPullMsg opened %v, want the Overview", m)
	}
	press(t, h, "esc")
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 142, Checks: true}))
	if m := h.modal(); m == nil || m.tab != checksTab || m.checks == nil {
		t.Fatalf("OpenPullMsg with Checks opened %v, want the Checks tab", m)
	}
}

func TestOpenOnChecksThenEscCloses(t *testing.T) {
	h := started(t, newFakeService(), 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	press(t, h, "C")
	if m := h.modal(); m == nil || m.tab != checksTab {
		t.Fatalf("C on the list opened %v, want the modal on its Checks tab", m)
	}
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("one esc didn't close the modal from the Checks tab")
	}
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 135, Checks: true}))
	if m := h.modal(); m == nil || m.tab != checksTab {
		t.Fatalf("OpenPullMsg opened %v, want the Checks tab", m)
	}
}

func TestTabKeepsItsState(t *testing.T) {
	svc := newFakeService()
	svc.thread = uitest.Comments(clock)
	h, m := tabbed(t, svc, &fakeChecks{})
	for range 6 {
		press(t, h, "j")
	}
	off := m.thread.YOffset()
	if off == 0 {
		t.Fatal("j didn't scroll the thread")
	}
	press(t, h, "]")
	press(t, h, "down")
	press(t, h, "[")
	if got := m.thread.YOffset(); got != off {
		t.Errorf("thread offset = %d after the other tab, want %d", got, off)
	}
	press(t, h, "]")
	if r, ok := m.checks.Checks(); !ok || r.Total == 0 {
		t.Error("the Checks tab lost its checks")
	}
}

// watching counts how often the checks are polled and polling stops.
type watching struct {
	mu            sync.Mutex
	starts, stops int
}

func (w *watching) watch(actionssvc.ChecksQuery) func() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.starts++
	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.stops++
	}
}

func (w *watching) counts() (starts, stops int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.starts, w.stops
}

func TestChecksPollOnlyWhileVisible(t *testing.T) {
	w := &watching{}
	h, m := tabbed(t, newFakeService(), &fakeChecks{pending: true}, checks.WithWatch(w.watch), checks.WithTick(0))
	if starts, _ := w.counts(); starts != 0 {
		t.Fatalf("polled %d times on the conversation, want none", starts)
	}
	press(t, h, "]")
	if starts, stops := w.counts(); starts != 1 || stops != 0 {
		t.Fatalf("on the Checks tab: %d starts, %d stops, want 1 and 0", starts, stops)
	}
	press(t, h, "[")
	if starts, stops := w.counts(); starts != 1 || stops != 1 {
		t.Fatalf("after leaving it: %d starts, %d stops, want 1 and 1", starts, stops)
	}
	press(t, h, "]")
	if starts, stops := w.counts(); starts != 2 || stops != 1 {
		t.Fatalf("back on it: %d starts, %d stops, want 2 and 1", starts, stops)
	}
	// Another modal replaces this one and gives it back: the polls go on
	// only where the Checks tab shows.
	m.Hide()
	_ = m.Update(ui.ReopenedMsg{Modal: m})
	if starts, stops := w.counts(); starts != 3 || stops != 2 {
		t.Fatalf("hidden and reopened on the Checks tab: %d starts, %d stops, want 3 and 2", starts, stops)
	}
	press(t, h, "[")
	m.Hide()
	_ = m.Update(ui.ReopenedMsg{Modal: m})
	if starts, stops := w.counts(); starts != 3 || stops != 3 {
		t.Fatalf("reopened on the conversation: %d starts, %d stops, want 3 and 3", starts, stops)
	}
	press(t, h, "]")
	press(t, h, "esc")
	if starts, stops := w.counts(); starts != stops {
		t.Errorf("closing left polls running: %d starts, %d stops", starts, stops)
	}
}

func TestBackspaceStepsOutOfTheLog(t *testing.T) {
	h, m := openedOnAJob(t)
	if v := modalText(h); !strings.Contains(v, "Step 1") {
		t.Fatalf("the log isn't shown:\n%s", v)
	}
	press(t, h, "backspace")
	if v := modalText(h); strings.Contains(v, "Step 1") || !strings.Contains(v, "dco") {
		t.Errorf("backspace didn't go back to the checks:\n%s", v)
	}
	press(t, h, "backspace")
	if h.modal() != m || m.tab != checksTab || !strings.Contains(modalText(h), "dco") {
		t.Error("backspace on the list of checks did something")
	}
}

func TestEscClearsTheLogSearchThenCloses(t *testing.T) {
	h, m := openedOnAJob(t)
	for _, k := range []string{"/", "F", "A", "I", "L", "enter"} {
		press(t, h, k)
	}
	press(t, h, "esc")
	if h.modal() != m || !strings.Contains(modalText(h), "Step 1") {
		t.Fatal("the first esc left the log")
	}
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("the second esc didn't close the modal")
	}
}

func TestNoChecksTabWithoutChecks(t *testing.T) {
	svc := newFakeService()
	// #114 has no checks at all.
	svc.bare = map[int]bool{114: true}
	h := started(t, svc, 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 114}))
	m := h.modal()
	if m == nil || m.number != 114 {
		t.Fatalf("opened %v, want #114", m)
	}
	if names, _ := m.Tabs(); !slices.Equal(names, []string{"Overview", "Files 1", "Conversation"}) {
		t.Errorf("tabs = %v, want no Checks tab for a pull request without checks", names)
	}
	press(t, h, "C")
	if m.tab != overviewTab || m.checks != nil {
		t.Error("C showed the checks of a pull request without them")
	}
	press(t, h, "]")
	if m.tab != filesTab || m.checks != nil {
		t.Errorf("] showed tab %d with checks %v, want the files and no checks", m.tab, m.checks)
	}
	if got := uitest.Enabled(m.KeyLayers()); slices.Contains(got, "checks") {
		t.Errorf("help offers %v, want no checks", got)
	}
}

func TestTabsBeforeTheDetailAndInShort(t *testing.T) {
	_, m := tabbed(t, newFakeService(), &fakeChecks{})
	m.loaded, m.detail = false, core.PullRequestDetail{}
	if names, _ := m.Tabs(); !slices.Equal(names, []string{"Overview", "Files", "Conversation", "Checks"}) {
		t.Errorf("tabs = %v before the detail, want no counts", names)
	}
	m.loaded, m.detail.Comments = true, 6
	m.detail.CheckCounts = core.CheckCounts{Failed: 2, Passed: 3}
	m.checksSvc = nil
	m.SetSize(36, 20)
	if names, _ := m.Tabs(); !slices.Equal(names, []string{"Ov", "Fi 0", "Co 6", "Ch ✗2"}) {
		t.Errorf("tabs = %v in a narrow modal, want the short ones", names)
	}
}
