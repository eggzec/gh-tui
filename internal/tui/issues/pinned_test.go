package issues

import (
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

func numbered(ns ...int) []core.Issue {
	items := make([]core.Issue, len(ns))
	for i, n := range ns {
		items[i] = core.Issue{Number: n}
	}
	return items
}

func numbersOf(items []core.Issue) []int {
	ns := make([]int, len(items))
	for i := range items {
		ns[i] = items[i].Number
	}
	return ns
}

func TestPinKeysStoreReportsChanges(t *testing.T) {
	t.Parallel()
	var p pinKeys
	tests := []struct {
		name  string
		items []core.Issue
		want  bool
	}{
		{"first set", numbered(1, 2), true},
		{"same set", numbered(2, 1), false},
		{"one gained", numbered(1, 2, 3), true},
		{"one lost", numbered(1, 3), true},
		{"swapped", numbered(1, 4), true},
		{"all lost", nil, true},
		{"still none", nil, false},
	}
	for _, tt := range tests {
		if got := p.store(tt.items); got != tt.want {
			t.Errorf("%s: store reports %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPinKeysDropNeverChangesItsInput(t *testing.T) {
	t.Parallel()
	var p pinKeys
	in := numbered(5, 6, 7, 8)
	if got := p.drop(in); !slices.Equal(numbersOf(got), []int{5, 6, 7, 8}) {
		t.Errorf("drop with no set = %v", numbersOf(got))
	}
	p.store(numbered(6, 8, 99))
	got := p.drop(in)
	if want := []int{5, 7}; !slices.Equal(numbersOf(got), want) {
		t.Errorf("drop = %v, want %v", numbersOf(got), want)
	}
	if want := []int{5, 6, 7, 8}; !slices.Equal(numbersOf(in), want) {
		t.Errorf("drop changed its input to %v", numbersOf(in))
	}
	clean := numbered(1, 2)
	if got := p.drop(clean); !slices.Equal(numbersOf(got), []int{1, 2}) {
		t.Errorf("drop of unpinned = %v", numbersOf(got))
	}
}

// The fetch of the list drops pinned numbers on its own goroutine while the
// section stores them, which -race checks.
func TestPinKeysStoreAndDropConcurrently(t *testing.T) {
	t.Parallel()
	var p pinKeys
	in := numbered(1, 2, 3, 4, 5, 6)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			for j := range 200 {
				if i%2 == 0 {
					p.store(numbered(j%6, (j+1)%6))
					continue
				}
				_ = p.drop(in)
			}
		})
	}
	wg.Wait()
	if want := []int{1, 2, 3, 4, 5, 6}; !slices.Equal(numbersOf(in), want) {
		t.Errorf("input became %v", numbersOf(in))
	}
}

func TestClampPinnedAtTen(t *testing.T) {
	t.Parallel()
	if feed.MaxPinned != 10 {
		t.Fatalf("feed.MaxPinned = %d, want 10", feed.MaxPinned)
	}
	many := sampleIssues(15)
	if got := clampPinned(many); len(got) != 10 {
		t.Errorf("clampPinned keeps %d of 15, want 10", len(got))
	}
	if got := clampPinned(many[:3]); len(got) != 3 {
		t.Errorf("clampPinned keeps %d of 3, want 3", len(got))
	}
	if got := clampPinned(nil); len(got) != 0 {
		t.Errorf("clampPinned of none keeps %d", len(got))
	}
}

func TestPinnedReadOnInitResetAndReload(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	svc.pin(sampleIssues(12)[0])
	h := started(t, svc, 80, 20)
	reads := svc.pinnedReads()
	if reads < 1 {
		t.Fatalf("Init read the pinned issues %d times, want one", reads)
	}
	run(t, h, h.resetList())
	if got := svc.pinnedReads(); got != reads+1 {
		t.Errorf("resetList: %d reads, want %d", got, reads+1)
	}
	reads = svc.pinnedReads()
	run(t, h, h.reload())
	if got := svc.pinnedReads(); got != reads+1 {
		t.Errorf("reload: %d reads, want %d", got, reads+1)
	}
}

func TestLandPinned(t *testing.T) {
	t.Parallel()
	all := sampleIssues(12)
	other := core.RepoRef{Owner: "eggzec", Name: "other"}
	svc := newFakeService(all)
	svc.pin(all[0])
	h := started(t, svc, 80, 20)
	lists := len(svc.listCalls())

	if cmd := h.landPinned(pinnedMsg{repo: other, items: all[1:3]}); cmd != nil {
		t.Error("the pins of another repository started a read")
	}
	if got := h.list.Pinned(); got != 1 {
		t.Errorf("the pins of another repository changed the block to %d rows", got)
	}

	if cmd := h.landPinned(pinnedMsg{repo: testRepo, items: all[:1]}); cmd != nil {
		t.Error("the same pins read the list again")
	}

	cmd := h.landPinned(pinnedMsg{repo: testRepo, items: all[:2]})
	if cmd == nil {
		t.Fatal("a new pinned issue didn't read the list again")
	}
	run(t, h, cmd)
	if len(svc.listCalls()) <= lists {
		t.Error("the list wasn't read after a pinned issue was gained")
	}

	lists = len(svc.listCalls())
	cmd = h.landPinned(pinnedMsg{repo: testRepo, items: all[:1]})
	if cmd == nil {
		t.Fatal("a lost pinned issue didn't read the list again")
	}
	run(t, h, cmd)
	if len(svc.listCalls()) <= lists {
		t.Error("the list wasn't read after a pinned issue was lost")
	}

	h.landPinned(pinnedMsg{repo: testRepo, err: errors.New("boom")})
	if got := h.list.Pinned(); got != 0 {
		t.Errorf("a failed read left %d pinned rows, want none", got)
	}
}

func TestFetchDropsPinnedNumbers(t *testing.T) {
	t.Parallel()
	all := sampleIssues(12)
	svc := newFakeService(all)
	svc.pin(all[0])
	h := started(t, svc, 80, 20)
	if first, _ := h.list.Selected(); first.Number == all[0].Number {
		t.Errorf("#%d is in the list as well as above it", first.Number)
	}
	press(t, h, "up")
	if pinned, _ := h.list.Selected(); pinned.Number != all[0].Number || !h.onPinned() {
		t.Errorf("the row above the list is #%d, want the pinned #%d", pinned.Number, all[0].Number)
	}
}

// With rows marked, closing from a pinned row closes that issue alone, since
// the marks are never on it.
func TestCloseOnPinnedRowIgnoresMarks(t *testing.T) {
	t.Parallel()
	all := sampleIssues(12)
	svc := newFakeService(all)
	svc.pin(all[0])
	h := started(t, svc, 80, 20)
	press(t, h, "space", "up", "up")
	if h.list.Marks() != 1 || !h.onPinned() {
		t.Fatalf("%d marks, on a pinned row %v, want 1 and true", h.list.Marks(), h.onPinned())
	}
	press(t, h, "X")
	if got := question(h); got == "Close 1 issue?" || got == "Close 1 issues?" {
		t.Errorf("a bulk question %q on a pinned row", got)
	}
	press(t, h, "y")
	if got, want := svc.changeCalls(), []string{"close " + strconv.Itoa(all[0].Number)}; !slices.Equal(got, want) {
		t.Errorf("changes = %v, want %v", got, want)
	}
}

// A pinned issue with more labels than any loaded row widens the labels
// column.
func TestPinnedLabelsSizeTheColumn(t *testing.T) {
	t.Parallel()
	issues := sampleIssues(4)
	for i := range issues {
		issues[i].Labels = []core.Label{{Name: "bug", Color: "d73a4a"}}
	}
	h := started(t, newFakeService(issues), 120, 12)
	before := h.cols.labels
	pin := core.Issue{Number: 99, Title: "pinned", Repo: testRepo, Labels: []core.Label{
		{Name: "documentation", Color: "0075ca"},
		{Name: "good first issue", Color: "7057ff"},
		{Name: "help wanted", Color: "008672"},
	}}
	h.landPinned(pinnedMsg{repo: testRepo, items: []core.Issue{pin}})
	if h.cols.labels <= before {
		t.Fatalf("labels column of %d cells with a wide pinned row, want more than %d", h.cols.labels, before)
	}
}

func TestSectionWithoutServiceSurvivesPins(t *testing.T) {
	t.Parallel()
	s := New(t.Context(), nil, config.Default().Keys)
	s.SetSize(80, 20)
	_ = s.Init()
	_ = s.setRepo(testRepo)
	_ = s.reload()
	_ = s.resetList()
	_ = s.newList()
	_ = s.readPinned()
	_ = s.cachedPinned()
}

func TestReloadIsOneFlatBatch(t *testing.T) {
	t.Parallel()
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	ran := false
	then := func() tea.Msg { ran = true; return nil }
	batch, ok := h.reload(then)().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("reload returned %T of %d commands, want one batch of 3", batch, len(batch))
	}
	for _, c := range batch {
		if _, nested := c().(tea.BatchMsg); nested {
			t.Error("reload nested a batch in its batch")
		}
	}
	if !ran {
		t.Error("the command given to reload didn't run")
	}
}
