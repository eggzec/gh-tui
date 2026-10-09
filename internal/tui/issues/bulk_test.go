package issues

import (
	"maps"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
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

// One question covers every marked issue, the changes are sent once the
// user says yes, each once, and the marks are gone when they are done.
func TestBulkCloseAsksOnce(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	markRows(t, h, 0, 1, 2)
	press(t, h, "X")
	if got, want := question(h), "Close 3 issues?"; got != want {
		t.Fatalf("asks %q, want %q", got, want)
	}
	if len(h.modals) != 1 || len(svc.changeCalls()) != 0 {
		t.Fatalf("%d modals and changes %v before the answer", len(h.modals), svc.changeCalls())
	}
	msgs := press(t, h, "y")
	got := svc.changeCalls()
	slices.Sort(got)
	if want := []string{"close 1000", "close 998", "close 999"}; !slices.Equal(got, want) {
		t.Errorf("changes = %v, want %v", got, want)
	}
	if done, ok := has[ui.BulkDoneMsg](msgs); !ok || len(done.Failed) != 0 {
		t.Errorf("report %+v (found %v), want one without failures", done, ok)
	}
	if _, ok := has[ui.DoneMsg](msgs); ok {
		t.Error("a change reported on its own, want one report for all")
	}
	if h.list.Marks() != 0 {
		t.Errorf("%d rows are still marked", h.list.Marks())
	}
}

// What the change doesn't apply to is left out and counted.
func TestBulkCountsWhatItSkips(t *testing.T) {
	tests := []struct {
		name string
		rows []int
		key  string
		want string
		sent []string
	}{
		{"close skips closed", []int{0, 4}, "X", "Close 1 of 2 marked (1 already closed)?", []string{"close 1000"}},
		{"reopen skips open", []int{0, 4, 9}, "O", "Reopen 2 of 3 marked (1 already open)?", []string{"reopen 991", "reopen 996"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			h := started(t, svc, 80, 20)
			// All issues.
			press(t, h, "]", "]")
			markRows(t, h, tt.rows...)
			press(t, h, tt.key)
			if got := question(h); got != tt.want {
				t.Fatalf("asks %q, want %q", got, tt.want)
			}
			press(t, h, "y")
			got := svc.changeCalls()
			slices.Sort(got)
			if !slices.Equal(got, tt.sent) {
				t.Errorf("changes = %v, want %v", got, tt.sent)
			}
		})
	}
}

// With nothing to change, a warning says why, and no question opens. The
// labels key stays with one issue: with marks it does nothing on the list.
func TestBulkNothingToChangeAndLabels(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	press(t, h, "space")
	press(t, h, "L")
	if len(h.modals) != 0 || len(svc.changeCalls()) != 0 {
		t.Errorf("labels with marks opened %d modals and changed %v", len(h.modals), svc.changeCalls())
	}
	msgs := press(t, h, "O")
	want := ui.NotifyMsg{Level: toast.Warning, Text: "No marked issue can be reopened: 1 already open."}
	if len(h.modals) != 0 || !slices.Contains(msgs, tea.Msg(want)) {
		t.Errorf("reopen of an open issue asked %q and showed %v, want only %v", question(h), msgs, want)
	}
}

// The change keys type their letters into the list's prompt.
func TestBulkKeysTypeInThePrompt(t *testing.T) {
	h := started(t, newFakeService(sampleIssues(12)), 80, 20)
	markRows(t, h, 0, 1)
	for _, k := range []string{"/", "X", "O"} {
		press(t, h, k)
	}
	if len(h.modals) != 0 {
		t.Errorf("a key typed in the prompt asked %q", question(h))
	}
}

// Close and reopen on one key go the way of the marked issue under the
// cursor, or else of the first marked one.
func TestBulkSharedCloseReopenKey(t *testing.T) {
	keys := config.Default().Keys
	own := maps.Clone(keys)
	own["issues"] = maps.Clone(keys["issues"])
	own["issues"]["close"] = []string{"X"}
	own["issues"]["reopen"] = []string{"X"}
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
			svc := newFakeService(sampleIssues(12))
			s := New(t.Context(), svc, own, WithNow(func() time.Time { return testNow }))
			s.SetTheme(testTheme())
			s.SetSize(80, 20)
			s.Focus()
			h := &host{Section: s}
			run(t, h, h.Update(ui.RepoMsg{Repo: testRepo}))
			run(t, h, h.Init())
			// All issues; #1000 is open and #996 closed.
			press(t, h, "]", "]")
			markRows(t, h, 0, 4)
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
