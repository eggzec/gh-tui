package ui

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// planOf returns a plan to close n pull requests, numbered from 1, whose
// ops run do.
func planOf(n int, do func(i int) error) BulkPlan {
	p := NewBulkPlan("Close", "Closed", "pull request", "", 0)
	for i := 1; i <= n; i++ {
		p.Items = append(p.Items, BulkItem{
			Key: strconv.Itoa(i), Name: "#" + strconv.Itoa(i), What: "close #" + strconv.Itoa(i),
			Start: func() Op { return opFunc(func(context.Context) error { return do(i) }) },
		})
	}
	return p
}

func TestBulkQuestion(t *testing.T) {
	tests := []struct {
		name string
		plan func() BulkPlan
		want string
	}{
		{"all of them", func() BulkPlan { return planOf(3, nil) }, "Close 3 pull requests?"},
		{"one", func() BulkPlan { return planOf(1, nil) }, "Close 1 pull request?"},
		{"tail", func() BulkPlan {
			p := planOf(2, nil)
			p.Verb, p.Tail = "Mark", " as read"
			return p
		}, "Mark 2 pull requests as read?"},
		{"not loaded", func() BulkPlan {
			p := planOf(3, nil)
			p.unloaded = 1
			return p
		}, "Close 3 of 4 marked (1 not loaded)?"},
		{"skipped", func() BulkPlan {
			p := planOf(2, nil)
			p.Skip("already closed")
			return p
		}, "Close 2 of 3 marked (1 already closed)?"},
		{"skipped and not loaded, with a tail", func() BulkPlan {
			p := planOf(2, nil)
			p.Verb, p.Tail, p.unloaded = "Mark", " as read", 2
			p.Skip("already read")
			p.Skip("already read")
			p.Skip("not allowed")
			return p
		}, "Mark 2 of 7 marked as read (2 already read, 1 not allowed, 2 not loaded)?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan().Question(); got != tt.want {
				t.Errorf("Question() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBulkNothing(t *testing.T) {
	p := NewBulkPlan("Close", "Closed", "pull request", "", 1)
	p.Skip("already closed")
	if got, want := p.Nothing(), "No marked pull request can be closed: 1 already closed, 1 not loaded."; got != want {
		t.Errorf("Nothing() = %q, want %q", got, want)
	}
	p.SkipRefused("You can't close in eggzec/x (read access).")
	if got, want := p.Nothing(), "You can't close in eggzec/x (read access)."; got != want {
		t.Errorf("Nothing() = %q, want the reason the viewer was refused: %q", got, want)
	}
}

// The changes are sent at most BulkLimit at a time, and that many at once
// when there are more.
func TestBulkConcurrencyIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var running, most atomic.Int32
		p := planOf(3*BulkLimit, func(int) error {
			n := running.Add(1)
			for old := most.Load(); n > old; old = most.Load() {
				if most.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
			return nil
		})
		msg := p.Do(t.Context(), "Pull requests")().(BulkDoneMsg)
		if got := most.Load(); got != BulkLimit {
			t.Errorf("%d changes were in flight at once, want %d", got, BulkLimit)
		}
		if len(msg.Failed) != 0 {
			t.Errorf("failed %v", msg.Failed)
		}
	})
}

// A change that fails is reported with the others that were sent, and
// each is sent once, and only after the user said yes.
func TestBulkReportsFailures(t *testing.T) {
	var mu sync.Mutex
	var sent []int
	offline := &core.Problem{Kind: core.Offline, Action: "close", Err: errors.New("no route")}
	p := planOf(5, func(i int) error {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, i)
		if i%2 == 0 {
			return offline
		}
		return nil
	})
	for i, it := range p.Items {
		if it.Start == nil || i < 0 {
			t.Fatal("no start")
		}
	}
	if len(sent) != 0 {
		t.Fatalf("sent %v before Do", sent)
	}
	msg := p.Do(t.Context(), "Pull requests")().(BulkDoneMsg)
	slices.Sort(sent)
	if !slices.Equal(sent, []int{1, 2, 3, 4, 5}) {
		t.Errorf("sent %v, want each once", sent)
	}
	if got := msg.FailedKeys(); !slices.Equal(got, []string{"2", "4"}) {
		t.Errorf("failed keys %v, want 2 and 4 in order", got)
	}
}

func TestBulkToast(t *testing.T) {
	always := func(toast.Level, string) bool { return true }
	offline := &core.Problem{Kind: core.Offline, Action: "close #2", Err: errors.New("no route")}
	unavailable := &core.Problem{Kind: core.Unavailable, Action: "close #3", Err: errors.New("503")}
	tests := []struct {
		name   string
		failed []int
		errs   []error
		fits   func(toast.Level, string) bool
		level  toast.Level
		want   string
	}{
		{name: "all done", level: toast.Success, want: "Closed 3 pull requests."},
		{
			name: "one failed", failed: []int{2}, errs: []error{offline}, level: toast.Error,
			want: "Couldn't close 1 of 3 pull requests: #2 (can't reach GitHub).",
		},
		{
			name: "two failed for one reason", failed: []int{2, 3}, errs: []error{offline, offline}, level: toast.Error,
			want: "Couldn't close 2 of 3 pull requests: #2, #3 (can't reach GitHub).",
		},
		{
			name: "two reasons", failed: []int{2, 3}, errs: []error{offline, unavailable}, level: toast.Error,
			want: "Couldn't close 2 of 3 pull requests: #2 (can't reach GitHub); #3 (GitHub isn't responding).",
		},
		{
			name: "reasons that don't fit", failed: []int{2, 3}, errs: []error{offline, unavailable}, level: toast.Error,
			fits: noReasons, want: "Couldn't close 2 of 3 pull requests: #2, #3.",
		},
		{
			name: "names that don't fit", failed: []int{2, 3}, errs: []error{offline, unavailable}, level: toast.Error,
			fits: noNames, want: "Couldn't close 2 of 3 pull requests.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planOf(3, nil)
			msg := BulkDoneMsg{From: "Pull requests", Plan: p}
			for i, n := range tt.failed {
				msg.Failed = append(msg.Failed, BulkFailure{Item: p.Items[n-1], Err: tt.errs[i]})
			}
			fits := tt.fits
			if fits == nil {
				fits = always
			}
			level, text := msg.Toast(testVoice(), fits)
			if level != tt.level || text != tt.want {
				t.Errorf("Toast() = %v %q, want %v %q", level, text, tt.level, tt.want)
			}
		})
	}
}

// The cause keeps GitHub's own words as they are, and a cause cut to fit
// keeps its call to action.
func TestBulkToastKeepsNamesAndCallsToAction(t *testing.T) {
	p := planOf(2, nil)
	rejected := &core.Problem{Kind: core.Rejected, Action: "close #1", Reason: "Pull Request is Protected by Octo rules", Err: errors.New("422")}
	auth := &core.Problem{Kind: core.Auth, Action: "close #2", Err: errors.New("401")}
	msg := BulkDoneMsg{From: "Pull requests", Plan: p, Failed: []BulkFailure{{Item: p.Items[0], Err: rejected}}}
	_, text := msg.Toast(testVoice(), func(toast.Level, string) bool { return true })
	if want := "Couldn't close 1 of 2 pull requests: #1 (Pull Request is Protected by Octo rules)."; text != want {
		t.Errorf("Toast() = %q, want %q", text, want)
	}

	msg = BulkDoneMsg{From: "Pull requests", Plan: p, Failed: []BulkFailure{{Item: p.Items[1], Err: auth}}}
	_, full := msg.Toast(testVoice(), func(toast.Level, string) bool { return true })
	// A toast with room for the command but not the whole sentence.
	_, short := msg.Toast(testVoice(), func(_ toast.Level, s string) bool { return len(s) < len(full) })
	want := "Couldn't close 1 of 2 pull requests: #2 (run gh auth login, then restart gh-tui)."
	if short == full || !strings.Contains(short, "run gh auth login") {
		t.Errorf("the cut toast %q lost the command (full: %q), want something like %q", short, full, want)
	}
}

func noReasons(_ toast.Level, s string) bool { return len(s) <= 46 }
func noNames(_ toast.Level, s string) bool   { return len(s) <= 40 }

// A yes asks the list again, and the change is sent only if it is still
// the one asked.
func TestBulkConfirmAsksAgain(t *testing.T) {
	asked := planOf(2, nil)
	var ran int
	run := func(BulkPlan) tea.Cmd { ran++; return Notify(toast.Info, "sent") }
	tests := []struct {
		name string
		now  func() (BulkPlan, bool)
		runs int
	}{
		{"the same", func() (BulkPlan, bool) { return planOf(2, nil), true }, 1},
		{"one row fewer", func() (BulkPlan, bool) { return planOf(1, nil), true }, 0},
		{"another row", func() (BulkPlan, bool) {
			p := planOf(2, nil)
			p.Items[1].Key = "9"
			return p, true
		}, 0},
		{"one skipped", func() (BulkPlan, bool) {
			p := planOf(2, nil)
			p.Skip("already closed")
			return p, true
		}, 0},
		{"no plan", func() (BulkPlan, bool) { return BulkPlan{}, false }, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ran = 0
			cmd := asked.Confirm(tt.now, run).Run()
			if ran != tt.runs {
				t.Errorf("ran %d times, want %d", ran, tt.runs)
			}
			if tt.runs == 0 {
				msg, ok := cmd().(NotifyMsg)
				if !ok || msg.Text != "The marked pull requests changed meanwhile, so nothing was sent." {
					t.Errorf("told the user %#v", msg)
				}
			}
		})
	}
}
