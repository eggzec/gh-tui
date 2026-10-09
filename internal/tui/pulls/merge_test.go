package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// stand sets how pull request #142 stands, as its detail says: what GitHub
// says of merging it, and what the list says of its reviews and checks.
func stand(svc *fakeService, merge core.MergeInfo, review core.ReviewDecision, checks core.ChecksState) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.merges = map[int]core.MergeInfo{142: merge}
	i := slices.IndexFunc(svc.pulls, func(pr core.PullRequest) bool { return pr.Number == 142 })
	svc.pulls[i].ReviewDecision, svc.pulls[i].Checks, svc.pulls[i].HeadSHA = review, checks, "a1b2c3d"
}

// asking starts the app over svc with caps, and presses the keys, the
// last of which asks a change. from names where it asks: the list or the
// modal of #142, as the keys lead.
func asking(t *testing.T, svc *fakeService, caps core.RepoCaps, keys ...string) (h *host, msgs []tea.Msg) {
	t.Helper()
	h = started(t, svc, 120, 20)
	drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: caps}))
	for _, k := range keys {
		msgs = press(t, h, k)
	}
	return h, msgs
}

// confirmKeys returns the help of the open question.
func confirmKeys(h *host) []string {
	top, ok := h.modals[len(h.modals)-1].(ui.Keyed)
	if !ok {
		return nil
	}
	return uitest.Enabled(top.KeyLayers())
}

// entries are where a merge is asked: the list, and the modal.
var entries = []struct {
	name string
	keys []string
}{
	{"list", []string{"M"}},
	{"modal", []string{"enter", "M"}},
}

func TestMergeMethodCycles(t *testing.T) {
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			svc := newFakeService()
			h, _ := asking(t, svc, writeCaps, e.keys...)
			// The methods in the order GitHub offers them, from the
			// repository's.
			want := []string{
				"Squash-merge #142 into main?",
				"Rebase-merge #142 into main?",
				"Merge #142 into main with a merge commit?",
				"Squash-merge #142 into main?",
			}
			for i, w := range want {
				if i > 0 {
					press(t, h, "tab")
				}
				if got := question(h); got != w {
					t.Fatalf("after %d tabs asks %q, want %q", i, got, w)
				}
			}
			press(t, h, "tab")
			press(t, h, "y")
			if got, want := svc.changes(), []string{"merge rebase 142"}; !slices.Equal(got, want) {
				t.Errorf("changes = %v, want the method the question named: %v", got, want)
			}
		})
	}
}

func TestMergeMethodIsRemembered(t *testing.T) {
	svc := newFakeService()
	h, _ := asking(t, svc, writeCaps, "M", "tab", "y")
	press(t, h, "M")
	if got, want := question(h), "Rebase-merge #135 into main?"; got != want {
		t.Errorf("the next merge asks %q, want it to start from the method used: %q", got, want)
	}
}

func TestMergeMethodsOfTheRepository(t *testing.T) {
	squashOrRebase := core.RepoCaps{Known: true, Permission: core.PermissionWrite, Squash: true, Rebase: true}
	rebaseOnly := core.RepoCaps{Known: true, Permission: core.PermissionWrite, Rebase: true, DefaultMerge: core.MergeRebase}
	for _, e := range entries {
		t.Run(e.name+"/never offers one the repository refuses", func(t *testing.T) {
			h, _ := asking(t, newFakeService(), squashOrRebase, e.keys...)
			asked := make([]string, 0, 4)
			for range 4 {
				asked = append(asked, question(h))
				press(t, h, "tab")
			}
			want := []string{
				"Squash-merge #142 into main?", "Rebase-merge #142 into main?",
				"Squash-merge #142 into main?", "Rebase-merge #142 into main?",
			}
			if !slices.Equal(asked, want) {
				t.Errorf("asked %q, want %q", asked, want)
			}
		})
		t.Run(e.name+"/one method has nothing to step through", func(t *testing.T) {
			svc := newFakeService()
			h, _ := asking(t, svc, rebaseOnly, e.keys...)
			if slices.Contains(confirmKeys(h), "method") {
				t.Errorf("help = %v, want no key for the method", confirmKeys(h))
			}
			press(t, h, "tab")
			if got, want := question(h), "Rebase-merge #142 into main?"; got != want {
				t.Errorf("after tab asks %q, want %q still", got, want)
			}
			press(t, h, "y")
			if got, want := svc.changes(), []string{"merge rebase 142"}; !slices.Equal(got, want) {
				t.Errorf("changes = %v, want %v", got, want)
			}
		})
	}
}

// The key for the method is in the help of the merge question alone.
func TestMethodKeyIsHelpedInTheMergeQuestionOnly(t *testing.T) {
	for _, e := range []struct {
		name string
		keys []string
		want bool
	}{
		{"merge", []string{"M"}, true},
		{"merge in the modal", []string{"enter", "M"}, true},
		{"close", []string{"X"}, false},
		{"close in the modal", []string{"enter", "X"}, false},
		{"draft", []string{"W"}, false},
	} {
		t.Run(e.name, func(t *testing.T) {
			h, _ := asking(t, newFakeService(), writeCaps, e.keys...)
			if got := slices.Contains(confirmKeys(h), "method"); got != e.want {
				t.Errorf("help = %v, want the method key: %v", confirmKeys(h), e.want)
			}
			q := question(h)
			press(t, h, "tab")
			if got := question(h); (got != q) != e.want {
				t.Errorf("tab changed %q to %q, want it to step through the choices: %v", q, got, e.want)
			}
		})
	}
}

func TestAutoMergeWhileChecksArePending(t *testing.T) {
	autoCaps := writeCaps
	autoCaps.AutoMerge = true
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			svc := newFakeService()
			stand(svc, core.MergeInfo{Status: core.MergeBlocked, CanAutoMerge: true, RepoAutoMerge: true}, core.ReviewApproved, core.ChecksPending)
			h, _ := asking(t, svc, autoCaps, e.keys...)
			if got, want := question(h), "Merge #142 into main when checks pass (squash)?"; got != want {
				t.Fatalf("asks %q, want %q", got, want)
			}
			if !slices.Contains(confirmKeys(h), "method") {
				t.Errorf("help = %v, want the method key", confirmKeys(h))
			}
			press(t, h, "tab")
			if got, want := question(h), "Merge #142 into main when checks pass (rebase)?"; got != want {
				t.Fatalf("after tab asks %q, want %q", got, want)
			}
			msgs := press(t, h, "y")
			if got, want := svc.changes(), []string{"automerge rebase at a1b2c3d 142"}; !slices.Equal(got, want) {
				t.Errorf("changes = %v, want auto-merge with the chosen method: %v", got, want)
			}
			if !slices.Contains(msgs, tea.Msg(ui.DoneMsg{From: ui.PullsTitle, What: "merge #142 when its checks pass"})) {
				t.Errorf("messages %v, want a DoneMsg for the auto-merge", msgs)
			}
		})
	}
	t.Run("not where the repository forbids it", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeBlocked, CanAutoMerge: false}, core.ReviewApproved, core.ChecksPending)
		_, msgs := asking(t, svc, writeCaps, "M")
		want := ui.NotifyMsg{Level: toast.Warning, Text: "Can't merge: checks are still running."}
		if !slices.Contains(msgs, tea.Msg(want)) {
			t.Errorf("messages %v, want %v", msgs, want)
		}
	})
}

func TestCantMerge(t *testing.T) {
	tests := []struct {
		name   string
		merge  core.MergeInfo
		review core.ReviewDecision
		checks core.ChecksState
		want   string
	}{
		{
			name: "changes requested and a failing check", merge: core.MergeInfo{Status: core.MergeBlocked},
			review: core.ReviewChangesRequested, checks: core.ChecksFailure,
			want: "Can't merge: changes were requested, checks are failing.",
		},
		{
			name: "conflicts", merge: core.MergeInfo{Status: core.MergeDirty, Mergeable: core.MergeableConflicting},
			review: core.ReviewApproved, checks: core.ChecksSuccess,
			want: "Can't merge: it has conflicts with main.",
		},
		{
			name: "behind its base", merge: core.MergeInfo{Status: core.MergeBehind},
			review: core.ReviewApproved, checks: core.ChecksSuccess,
			want: "Can't merge: its branch is behind main.",
		},
		{
			name: "a review is missing", merge: core.MergeInfo{Status: core.MergeBlocked},
			review: core.ReviewRequired, checks: core.ChecksSuccess,
			want: "Can't merge: a review is required.",
		},
		{
			name: "branch protection", merge: core.MergeInfo{Status: core.MergeBlocked},
			review: core.ReviewApproved, checks: core.ChecksSuccess,
			want: "Can't merge: branch protection rules aren't met.",
		},
		{
			name: "pending checks where auto-merge is off", merge: core.MergeInfo{Status: core.MergeBlocked},
			review: core.ReviewApproved, checks: core.ChecksPending,
			want: "Can't merge: checks are still running.",
		},
		{
			name: "already in the queue", merge: core.MergeInfo{Status: core.MergeClean, Queue: core.MergeQueue{Enabled: true, Queued: true}},
			review: core.ReviewApproved, checks: core.ChecksSuccess,
			want: "#142 is already in the merge queue.",
		},
	}
	for _, tt := range tests {
		for _, e := range entries {
			t.Run(tt.name+"/"+e.name, func(t *testing.T) {
				svc := newFakeService()
				stand(svc, tt.merge, tt.review, tt.checks)
				h, msgs := asking(t, svc, writeCaps, e.keys...)
				want := ui.NotifyMsg{Level: toast.Warning, Text: tt.want}
				if !slices.Contains(msgs, tea.Msg(want)) {
					t.Errorf("messages %v, want %v", msgs, want)
				}
				if got := question(h); got != "" {
					t.Errorf("asks %q, want nothing asked", got)
				}
				if got := svc.changes(); len(got) != 0 {
					t.Errorf("changes = %v, want none", got)
				}
			})
		}
	}
}

func TestMergeUnstable(t *testing.T) {
	t.Run("a check that isn't required doesn't block", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeUnstable}, core.ReviewApproved, core.ChecksFailure)
		h, _ := asking(t, svc, writeCaps, "M")
		if got, want := question(h), "Squash-merge #142 into main?"; got != want {
			t.Errorf("asks %q, want %q", got, want)
		}
	})
}

// A pull request that is blocked is never asked about, even of an
// administrator: no yes goes past the rules.
func TestAdministratorsGetTheSameWarning(t *testing.T) {
	admin := core.RepoCaps{Known: true, Permission: core.PermissionAdmin, MergeCommit: true, Squash: true, Rebase: true}
	tests := []struct {
		name   string
		merge  core.MergeInfo
		review core.ReviewDecision
		checks core.ChecksState
		want   string
	}{
		{"protection", core.MergeInfo{Status: core.MergeBlocked}, core.ReviewRequired, core.ChecksSuccess, "Can't merge: a review is required."},
		{"conflicts", core.MergeInfo{Status: core.MergeDirty, Mergeable: core.MergeableConflicting}, core.ReviewApproved, core.ChecksSuccess, "Can't merge: it has conflicts with main."},
		{"pending checks without auto-merge", core.MergeInfo{Status: core.MergeBlocked}, core.ReviewApproved, core.ChecksPending, "Can't merge: checks are still running."},
	}
	for _, tt := range tests {
		for _, e := range entries {
			t.Run(tt.name+"/"+e.name, func(t *testing.T) {
				svc := newFakeService()
				stand(svc, tt.merge, tt.review, tt.checks)
				h, msgs := asking(t, svc, admin, e.keys...)
				if want := (ui.NotifyMsg{Level: toast.Warning, Text: tt.want}); !slices.Contains(msgs, tea.Msg(want)) {
					t.Errorf("messages %v, want %v", msgs, want)
				}
				if got := question(h); got != "" {
					t.Errorf("asks %q, want nothing asked", got)
				}
				if got := svc.changes(); len(got) != 0 {
					t.Errorf("changes = %v, want none", got)
				}
			})
		}
	}
}

func TestMergeQueue(t *testing.T) {
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			svc := newFakeService()
			stand(svc, core.MergeInfo{Status: core.MergeClean, Queue: core.MergeQueue{Enabled: true}}, core.ReviewApproved, core.ChecksSuccess)
			h, _ := asking(t, svc, writeCaps, e.keys...)
			if got, want := question(h), "Add #142 to the merge queue for main?"; got != want {
				t.Fatalf("asks %q, want %q", got, want)
			}
			if slices.Contains(confirmKeys(h), "method") {
				t.Errorf("help = %v, want no method: the queue merges as it is set up", confirmKeys(h))
			}
			msgs := press(t, h, "y")
			if got, want := svc.changes(), []string{"enqueue at a1b2c3d 142"}; !slices.Equal(got, want) {
				t.Errorf("changes = %v, want %v", got, want)
			}
			if !slices.Contains(msgs, tea.Msg(ui.DoneMsg{From: ui.PullsTitle, What: "add #142 to the merge queue"})) {
				t.Errorf("messages %v, want a DoneMsg for the queue", msgs)
			}
		})
	}
	t.Run("pending checks go to auto-merge, which the queue takes in", func(t *testing.T) {
		autoCaps := writeCaps
		autoCaps.AutoMerge = true
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeBlocked, Queue: core.MergeQueue{Enabled: true}, CanAutoMerge: true, RepoAutoMerge: true}, core.ReviewApproved, core.ChecksPending)
		h, _ := asking(t, svc, autoCaps, "M")
		if got, want := question(h), "Merge #142 into main when checks pass?"; got != want {
			t.Errorf("asks %q, want %q", got, want)
		}
		if slices.Contains(confirmKeys(h), "method") {
			t.Errorf("help = %v, want no method with a queue", confirmKeys(h))
		}
	})
}

func TestAutoMergeAlreadyOn(t *testing.T) {
	on := core.MergeInfo{Status: core.MergeBlocked, AutoMerge: &core.AutoMerge{Method: core.MergeSquash}, CanDisableAutoMerge: true}
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			svc := newFakeService()
			stand(svc, on, core.ReviewApproved, core.ChecksPending)
			h, _ := asking(t, svc, writeCaps, e.keys...)
			if got, want := question(h), "Turn off auto-merge for #142?"; got != want {
				t.Fatalf("asks %q, want %q", got, want)
			}
			msgs := press(t, h, "y")
			if got, want := svc.changes(), []string{"stopautomerge 142"}; !slices.Equal(got, want) {
				t.Errorf("changes = %v, want %v", got, want)
			}
			if !slices.Contains(msgs, tea.Msg(ui.DoneMsg{From: ui.PullsTitle, What: "turn off auto-merge for #142"})) {
				t.Errorf("messages %v, want a DoneMsg for turning it off", msgs)
			}
		})
	}
	t.Run("not by someone who may not", func(t *testing.T) {
		svc := newFakeService()
		stopless := on
		stopless.CanDisableAutoMerge = false
		stand(svc, stopless, core.ReviewApproved, core.ChecksPending)
		_, msgs := asking(t, svc, writeCaps, "M")
		want := ui.NotifyMsg{Level: toast.Warning, Text: "You can't turn off auto-merge for #142."}
		if !slices.Contains(msgs, tea.Msg(want)) {
			t.Errorf("messages %v, want %v", msgs, want)
		}
	})
}

// A yes goes through only if what it was asked about still holds.
func TestMergeYesAsksAgainWhatItMerges(t *testing.T) {
	svc := newFakeService()
	stand(svc, core.MergeInfo{Status: core.MergeClean}, core.ReviewApproved, core.ChecksSuccess)
	h, _ := asking(t, svc, writeCaps, "M")
	// While the question is open, the pull request gets blocked.
	stand(svc, core.MergeInfo{Status: core.MergeBlocked}, core.ReviewChangesRequested, core.ChecksSuccess)
	msgs := press(t, h, "y")
	if got := svc.changes(); len(got) != 0 {
		t.Errorf("changes = %v, want none", got)
	}
	want := ui.NotifyMsg{Level: toast.Warning, Text: "Can't merge: changes were requested."}
	if !slices.Contains(msgs, tea.Msg(want)) {
		t.Errorf("messages %v, want %v", msgs, want)
	}
}

func TestMergeWaitsForTheDetailOfARow(t *testing.T) {
	t.Run("the list reads it first", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeBlocked}, core.ReviewApproved, core.ChecksSuccess)
		h := started(t, svc, 120, 20)
		drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
		svc.mu.Lock()
		clear(svc.cached)
		svc.gets = nil
		svc.mu.Unlock()
		msgs := press(t, h, "M")
		if !slices.Contains(msgs, info("Checking #142…")) {
			t.Errorf("messages %v, want the short wait said", msgs)
		}
		if got := svc.got(); !slices.Contains(got, 142) {
			t.Errorf("read %v, want the detail of #142", got)
		}
		want := ui.NotifyMsg{Level: toast.Warning, Text: "Can't merge: branch protection rules aren't met."}
		if !slices.Contains(msgs, tea.Msg(want)) {
			t.Errorf("messages %v, want %v from what it read", msgs, want)
		}
	})
	t.Run("the list says why it couldn't read it", func(t *testing.T) {
		svc := newFakeService()
		h := started(t, svc, 120, 20)
		drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
		svc.mu.Lock()
		clear(svc.cached)
		svc.getErr = errors.New("boom")
		svc.mu.Unlock()
		msgs := press(t, h, "M")
		if !slices.ContainsFunc(msgs, func(m tea.Msg) bool { f, ok := m.(ui.FailMsg); return ok && strings.Contains(f.What, "#142") }) {
			t.Errorf("messages %v, want the failure told", msgs)
		}
		if got := question(h); got != "" {
			t.Errorf("asks %q, want nothing asked", got)
		}
	})
	t.Run("the modal asks once the detail arrives", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeClean, Queue: core.MergeQueue{Enabled: true}}, core.ReviewApproved, core.ChecksSuccess)
		h := started(t, svc, 120, 20)
		drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
		press(t, h, "enter")
		m := h.modal()
		m.seen = false
		msgs := press(t, h, "M")
		if !slices.Contains(msgs, info("Checking #142…")) || question(h) != "" {
			t.Fatalf("messages %v and question %q, want the wait said and nothing asked", msgs, question(h))
		}
		drain(t, h, h.Update(detailMsg{thread: m.thread.ID(), detail: svc.detail(142)}))
		if got, want := question(h), "Add #142 to the merge queue for main?"; got != want {
			t.Errorf("asks %q, want %q", got, want)
		}
	})
}

func TestAutoMergeAlsoWaitingForAReview(t *testing.T) {
	autoCaps := writeCaps
	autoCaps.AutoMerge = true
	svc := newFakeService()
	stand(svc, core.MergeInfo{Status: core.MergeBlocked, CanAutoMerge: true, RepoAutoMerge: true}, core.ReviewRequired, core.ChecksPending)
	h, _ := asking(t, svc, autoCaps, "M")
	if got, want := question(h), "Merge #142 into main when it can be merged (squash)?"; got != want {
		t.Errorf("asks %q, want %q", got, want)
	}
}

// What the detail said is read again when the key is pressed, though it is
// fresh, and a key pressed again meanwhile waits for the same read.
func TestMergeReadsAnAgedDetailOnce(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeClean}, core.ReviewApproved, core.ChecksSuccess)
		h := started(t, svc, 120, 20)
		drain(t, h, h.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps}))
		svc.mu.Lock()
		svc.cached[142] = true
		svc.gets, svc.revalidated = nil, nil
		svc.mu.Unlock()
		// The second key arrives before the read answers.
		first := h.Update(keyMsg("M"))
		second := h.Update(keyMsg("M"))
		if second != nil {
			t.Error("the second key started something while the first waits")
		}
		msgs := drain(t, h, first)
		if !slices.Contains(msgs, info("Checking #142…")) {
			t.Errorf("messages %v, want the short wait said", msgs)
		}
		if got := svc.got(); len(got) != 1 || got[0] != 142 || len(svc.revalidated) != 1 {
			t.Errorf("read %v and revalidated %v, want one read of #142", got, svc.revalidated)
		}
		if got, want := question(h), "Squash-merge #142 into main?"; got != want {
			t.Errorf("asks %q, want %q", got, want)
		}
	})
	t.Run("modal", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeClean}, core.ReviewApproved, core.ChecksSuccess)
		h, _ := asking(t, svc, writeCaps, "enter")
		svc.mu.Lock()
		svc.stale[142] = true
		svc.gets = nil
		svc.mu.Unlock()
		first := h.Update(keyMsg("M"))
		if second := h.Update(keyMsg("M")); second != nil {
			t.Error("the second key started something while the first waits")
		}
		msgs := drain(t, h, first)
		if !slices.Contains(msgs, info("Checking #142…")) {
			t.Errorf("messages %v, want the short wait said", msgs)
		}
		if got := svc.got(); len(got) != 1 {
			t.Errorf("read %v, want one read", got)
		}
		if got, want := question(h), "Squash-merge #142 into main?"; got != want {
			t.Errorf("asks %q, want %q", got, want)
		}
	})
}

// Only the checks that the rules require count against a merge.
func TestCantMergeCountsRequiredChecks(t *testing.T) {
	tests := []struct {
		name   string
		merge  core.MergeInfo
		review core.ReviewDecision
		checks core.ChecksState
		want   string
	}{
		{
			name:   "required checks fail",
			merge:  core.MergeInfo{Status: core.MergeBlocked, RequiredChecks: core.CheckCounts{Passed: 1, Failed: 2}},
			review: core.ReviewApproved, checks: core.ChecksFailure,
			want: "Can't merge: 2 required checks are failing.",
		},
		{
			name:   "one that isn't required fails",
			merge:  core.MergeInfo{Status: core.MergeBlocked, RequiredChecks: core.CheckCounts{Passed: 2}},
			review: core.ReviewRequired, checks: core.ChecksFailure,
			want: "Can't merge: a review is required.",
		},
		{
			name:   "none is required",
			merge:  core.MergeInfo{Status: core.MergeBlocked, Rules: core.MergeRules{Known: true, Approvals: 1}},
			review: core.ReviewRequired, checks: core.ChecksFailure,
			want: "Can't merge: a review is required.",
		},
		{
			name:   "a required check runs",
			merge:  core.MergeInfo{Status: core.MergeBlocked, RequiredChecks: core.CheckCounts{Passed: 1, Pending: 1}},
			review: core.ReviewApproved, checks: core.ChecksFailure,
			want: "Can't merge: checks are still running.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			stand(svc, tt.merge, tt.review, tt.checks)
			_, msgs := asking(t, svc, writeCaps, "M")
			if want := (ui.NotifyMsg{Level: toast.Warning, Text: tt.want}); !slices.Contains(msgs, tea.Msg(want)) {
				t.Errorf("messages %v, want %v", msgs, want)
			}
		})
	}
}

// The methods the detail says the repository allows decide the choices.
func TestMergeMethodsOfTheDetail(t *testing.T) {
	svc := newFakeService()
	stand(svc, core.MergeInfo{Status: core.MergeClean, Methods: []core.MergeMethod{core.MergeCommit, core.MergeRebase}}, core.ReviewApproved, core.ChecksSuccess)
	h, _ := asking(t, svc, writeCaps, "M")
	got := make([]string, 0, 3)
	for range 3 {
		got = append(got, question(h))
		press(t, h, "tab")
	}
	want := []string{"Merge #142 into main with a merge commit?", "Rebase-merge #142 into main?", "Merge #142 into main with a merge commit?"}
	if !slices.Equal(got, want) {
		t.Errorf("asked %q, want %q", got, want)
	}
}

// A merge that GitHub hasn't said may merge is never asked, even of an
// administrator: a plain merge goes past the rules for them.
func TestMergeNeedsAStatusThatIsKnown(t *testing.T) {
	admin := core.RepoCaps{Known: true, Permission: core.PermissionAdmin, MergeCommit: true, Squash: true, Rebase: true}
	const wait = "GitHub is still checking whether #142 can merge; try again in a moment."
	for _, status := range []core.MergeStatus{core.MergeUnknown, "", "invented"} {
		for _, e := range entries {
			t.Run(string(status)+"/"+e.name, func(t *testing.T) {
				svc := newFakeService()
				stand(svc, core.MergeInfo{Status: status}, core.ReviewApproved, core.ChecksSuccess)
				h, msgs := asking(t, svc, admin, e.keys...)
				if want := (ui.NotifyMsg{Level: toast.Warning, Text: wait}); !slices.Contains(msgs, tea.Msg(want)) {
					t.Errorf("messages %v, want %v", msgs, want)
				}
				if got := question(h); got != "" {
					t.Errorf("asks %q, want nothing asked", got)
				}
				if got := svc.changes(); len(got) != 0 {
					t.Errorf("changes = %v, want none", got)
				}
			})
		}
	}
	t.Run("the modal reads once more", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeUnknown}, core.ReviewApproved, core.ChecksSuccess)
		h, _ := asking(t, svc, admin, "enter")
		before := len(svc.revalidated)
		msgs := press(t, h, "M")
		if got := len(svc.revalidated) - before; got != 1 {
			t.Errorf("read %d more times, want once", got)
		}
		if !slices.Contains(msgs, info("Checking #142…")) {
			t.Errorf("messages %v, want the short wait said", msgs)
		}
	})
	t.Run("GitHub has settled by the second read", func(t *testing.T) {
		svc := newFakeService()
		stand(svc, core.MergeInfo{Status: core.MergeUnknown}, core.ReviewApproved, core.ChecksSuccess)
		h, _ := asking(t, svc, writeCaps, "enter")
		stand(svc, core.MergeInfo{Status: core.MergeClean}, core.ReviewApproved, core.ChecksSuccess)
		press(t, h, "M")
		if got, want := question(h), "Squash-merge #142 into main?"; got != want {
			t.Errorf("asks %q, want %q", got, want)
		}
	})
	t.Run("the modal's read fails", func(t *testing.T) {
		svc := newFakeService()
		h, _ := asking(t, svc, admin, "enter")
		svc.mu.Lock()
		svc.getErr, svc.stale[142] = errors.New("boom"), true
		svc.mu.Unlock()
		msgs := press(t, h, "M")
		if !slices.ContainsFunc(msgs, func(m tea.Msg) bool { _, ok := m.(ui.FailMsg); return ok }) || question(h) != "" {
			t.Errorf("messages %v and question %q, want the failure told and nothing asked", msgs, question(h))
		}
	})
}

// A yes reads the pull request again and asks again of what it finds.
func TestMergeYesReadsAgain(t *testing.T) {
	tests := []struct {
		name   string
		change func(svc *fakeService)
		want   tea.Msg
	}{
		{
			name: "it became blocked",
			change: func(svc *fakeService) {
				stand(svc, core.MergeInfo{Status: core.MergeBlocked}, core.ReviewChangesRequested, core.ChecksSuccess)
			},
			want: ui.NotifyMsg{Level: toast.Warning, Text: "Can't merge: changes were requested."},
		},
		{
			name: "GitHub lost track of its state",
			change: func(svc *fakeService) {
				stand(svc, core.MergeInfo{Status: core.MergeUnknown}, core.ReviewApproved, core.ChecksSuccess)
			},
			want: ui.NotifyMsg{Level: toast.Warning, Text: "GitHub is still checking whether #142 can merge; try again in a moment."},
		},
		{
			name: "auto-merge was turned on elsewhere",
			change: func(svc *fakeService) {
				stand(svc, core.MergeInfo{Status: core.MergeClean, AutoMerge: &core.AutoMerge{}, CanDisableAutoMerge: true}, core.ReviewApproved, core.ChecksSuccess)
			},
			want: ui.NotifyMsg{Level: toast.Info, Text: "#142 changed meanwhile, so nothing was sent."},
		},
	}
	for _, tt := range tests {
		for _, e := range entries {
			t.Run(tt.name+"/"+e.name, func(t *testing.T) {
				svc := newFakeService()
				stand(svc, core.MergeInfo{Status: core.MergeClean}, core.ReviewApproved, core.ChecksSuccess)
				h, _ := asking(t, svc, writeCaps, e.keys...)
				if question(h) == "" {
					t.Fatal("asks nothing, want the merge asked")
				}
				tt.change(svc)
				before := len(svc.revalidated)
				msgs := press(t, h, "y")
				if got := len(svc.revalidated) - before; got != 1 {
					t.Errorf("read %d more times on yes, want once", got)
				}
				if !slices.Contains(msgs, tt.want) {
					t.Errorf("messages %v, want %v", msgs, tt.want)
				}
				if got := svc.changes(); len(got) != 0 {
					t.Errorf("changes = %v, want none", got)
				}
			})
		}
	}
	t.Run("the read fails", func(t *testing.T) {
		svc := newFakeService()
		h, _ := asking(t, svc, writeCaps, "M")
		svc.mu.Lock()
		svc.getErr = errors.New("boom")
		svc.mu.Unlock()
		msgs := press(t, h, "y")
		if !slices.ContainsFunc(msgs, func(m tea.Msg) bool { _, ok := m.(ui.FailMsg); return ok }) || len(svc.changes()) != 0 {
			t.Errorf("messages %v and changes %v, want the failure told and nothing sent", msgs, svc.changes())
		}
	})
}
