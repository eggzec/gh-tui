package pulls

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// markRows marks the rows at the offsets from the top of the list, which
// ascend, and leaves the cursor on the last.
func markRows(t *testing.T, h *host, rows ...int) {
	t.Helper()
	at := 0
	for _, r := range rows {
		for ; at < r; at++ {
			press(t, h, "down")
		}
		press(t, h, "space")
	}
}

// markedNumbers returns the numbers of the pull requests marked, in list
// order.
func markedNumbers(h *host) []int {
	items, _ := h.feed.MarkedItems()
	out := make([]int, 0, len(items))
	for i := range items {
		out = append(out, items[i].Number)
	}
	return out
}

// bulkDone returns the report of a bulk change among msgs, if there is one.
func bulkDone(msgs []tea.Msg) (ui.BulkDoneMsg, bool) {
	for _, m := range msgs {
		if b, ok := m.(ui.BulkDoneMsg); ok {
			return b, true
		}
	}
	return ui.BulkDoneMsg{}, false
}

// One question covers every marked row, and the changes are sent once the
// user says yes, each pull request once, with one report and no other
// change message. The marks are gone once it is done.
func TestBulkCloseAsksOnce(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	markRows(t, h, 0, 1, 3)
	if got := markedNumbers(h); !slices.Equal(got, []int{142, 135, 121}) {
		t.Fatalf("marked %v", got)
	}
	press(t, h, "X")
	if got, want := question(h), "Close 3 pull requests?"; got != want {
		t.Fatalf("asks %q, want %q", got, want)
	}
	if len(h.modals) != 1 || len(svc.changes()) != 0 {
		t.Fatalf("%d modals and changes %v before the answer, want one question and none", len(h.modals), svc.changes())
	}
	msgs := press(t, h, "y")
	changes := svc.changes()
	slices.Sort(changes)
	if want := []string{"close 121", "close 135", "close 142"}; !slices.Equal(changes, want) {
		t.Errorf("changes = %v, want %v", changes, want)
	}
	done, ok := bulkDone(msgs)
	if !ok || len(done.Failed) != 0 || len(done.Plan.Items) != 3 {
		t.Errorf("report %+v (found %v), want 3 changes, none failed", done, ok)
	}
	for _, m := range msgs {
		if _, ok := m.(ui.DoneMsg); ok {
			t.Errorf("a change reported %v of its own, want one report for all", m)
		}
	}
	if h.feed.Marks() != 0 {
		t.Errorf("%d rows are still marked after a successful change", h.feed.Marks())
	}
	if len(h.modals) != 0 {
		t.Errorf("the question is still open")
	}
}

// Rows the change doesn't apply to are left out and counted in the question.
func TestBulkCountsWhatItSkips(t *testing.T) {
	tests := []struct {
		name string
		rows []int
		key  string
		want string
		sent []string
	}{
		{
			name: "close skips closed and merged", rows: []int{0, 7, 9}, key: "X",
			want: "Close 1 of 3 marked (1 already closed, 1 already merged)?",
			sent: []string{"close 142"},
		},
		{
			name: "reopen skips open", rows: []int{0, 7, 8}, key: "O",
			want: "Reopen 2 of 3 marked (1 already open)?",
			sent: []string{"reopen 86", "reopen 93"},
		},
		{
			// The cursor is on a closed pull request, so the first marked
			// open one, which is ready, says which way the key goes.
			name: "draft goes the way of the first open one", rows: []int{0, 2, 7}, key: "W",
			want: "Convert 1 of 3 marked to draft (1 already draft, 1 not open)?",
			sent: []string{"draft 142"},
		},
		{
			// The cursor is on a draft that is marked.
			name: "draft goes the way of the one under the cursor", rows: []int{0, 2}, key: "W",
			want: "Mark 1 of 2 marked ready for review (1 already ready)?",
			sent: []string{"ready 128"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			// All pull requests, in every state.
			h := started(t, svc, 80, 20)
			for range 3 {
				press(t, h, "]")
			}
			markRows(t, h, tt.rows...)
			press(t, h, tt.key)
			if got := question(h); got != tt.want {
				t.Fatalf("asks %q, want %q", got, tt.want)
			}
			press(t, h, "y")
			got := svc.changes()
			slices.Sort(got)
			if !slices.Equal(got, tt.sent) {
				t.Errorf("changes = %v, want %v", got, tt.sent)
			}
		})
	}
}

// A marked pull request that isn't among the loaded ones is counted, and
// left alone.
func TestBulkCountsWhatIsNotLoaded(t *testing.T) {
	svc := newFakeService()
	svc.pageSize = 2
	h := started(t, svc, 80, 5)
	markRows(t, h, 0, 1)
	// #142 goes from the list, but with pages left to load, the list
	// can't tell it is gone, so its mark stays.
	svc.mu.Lock()
	svc.pulls = slices.DeleteFunc(svc.pulls, func(pr core.PullRequest) bool { return pr.Number == 142 })
	for n := 90; n < 95; n++ {
		more := svc.pulls[0]
		more.Number = n
		svc.pulls = append(svc.pulls, more)
	}
	svc.mu.Unlock()
	press(t, h, "r")
	if h.feed.Marks() != 2 {
		t.Fatalf("%d marks after the reload, want the 2 that cannot be told gone", h.feed.Marks())
	}
	press(t, h, "X")
	if got, want := question(h), "Close 1 of 2 marked (1 not loaded)?"; got != want {
		t.Fatalf("asks %q, want %q", got, want)
	}
	press(t, h, "y")
	if got, want := svc.changes(), []string{"close 135"}; !slices.Equal(got, want) {
		t.Errorf("changes = %v, want %v", got, want)
	}
}

// A change that GitHub refuses is rolled back alone, and reported by name;
// the others stay done, and only the rows that failed stay marked.
func TestBulkRollsBackWhatFailed(t *testing.T) {
	svc := newFakeService()
	svc.sendErrs = map[int]error{135: errors.New("boom")}
	h := started(t, svc, 80, 20)
	markRows(t, h, 0, 1, 3)
	press(t, h, "X")
	msgs := press(t, h, "y")
	done, ok := bulkDone(msgs)
	if !ok || len(done.Failed) != 1 || done.Failed[0].Item.Name != "#135" || done.Failed[0].Err == nil {
		t.Fatalf("report %+v (found %v), want #135 alone failed", done, ok)
	}
	for number, want := range map[int]core.State{142: core.StateClosed, 135: core.StateOpen, 121: core.StateClosed} {
		if got := svc.state(number).State; got != want {
			t.Errorf("#%d is %s, want %s", number, got, want)
		}
	}
	if got := markedNumbers(h); !slices.Equal(got, []int{135}) {
		t.Errorf("marked %v after the failure, want only #135, to try again", got)
	}
}

// Merge keeps to the pull request under the cursor, with or without marks.
func TestBulkExcludesMerge(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	markRows(t, h, 0, 1)
	press(t, h, "M")
	if got, want := question(h), "Squash-merge #135 into main?"; got != want {
		t.Fatalf("asks %q, want %q", got, want)
	}
	press(t, h, "y")
	if got, want := svc.changes(), []string{"merge squash 135"}; !slices.Equal(got, want) {
		t.Errorf("changes = %v, want %v", got, want)
	}
}

// With nothing to change, a warning says why, and no question opens.
func TestBulkNothingToChange(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	press(t, h, "]")
	markRows(t, h, 0)
	msgs := press(t, h, "X")
	want := ui.NotifyMsg{Level: toast.Warning, Text: "No marked pull request can be closed: 1 already closed."}
	if !slices.Contains(msgs, tea.Msg(want)) || len(h.modals) != 0 {
		t.Errorf("messages %v with %d modals, want %v and none", msgs, len(h.modals), want)
	}
	if h.feed.Marks() != 1 {
		t.Errorf("%d marks, want the one left", h.feed.Marks())
	}
}

// The change keys type their letters into the list's prompt.
func TestBulkKeysTypeInThePrompt(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
	markRows(t, h, 0, 1)
	for _, k := range []string{"/", "X", "O", "W"} {
		press(t, h, k)
	}
	if len(h.modals) != 0 {
		t.Errorf("a key typed in the prompt asked %q", question(h))
	}
	if got := h.feed.FindQuery(); got != "" {
		t.Errorf("find %q before enter", got)
	}
}

// Close and reopen on one key go the way of the marked row under the
// cursor, or else of the first marked row.
func TestBulkSharedCloseReopenKey(t *testing.T) {
	keys := config.Default().Keys
	own := maps.Clone(keys)
	own["pulls"] = maps.Clone(keys["pulls"])
	own["pulls"]["close"] = []string{"X"}
	own["pulls"]["reopen"] = []string{"X"}
	tests := []struct {
		name string
		more []string
		want string
	}{
		{"on a marked closed row", nil, "Reopen 1 of 2 marked (1 already open)?"},
		{"on an unmarked row", []string{"down"}, "Close 1 of 2 marked (1 already closed)?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			s := New(t.Context(), svc, own, WithClock(func() time.Time { return clock }))
			s.SetSize(80, 20)
			s.Focus()
			h := &host{Section: s}
			drain(t, h, h.Update(ui.RepoMsg{Repo: repo}))
			drain(t, h, h.Init())
			// All pull requests; #142 is open and #93 closed.
			for range 3 {
				press(t, h, "]")
			}
			markRows(t, h, 0, 7)
			for _, k := range tt.more {
				press(t, h, k)
			}
			press(t, h, "X")
			if got := question(h); got != tt.want {
				t.Errorf("asks %q, want %q", got, tt.want)
			}
		})
	}
}
