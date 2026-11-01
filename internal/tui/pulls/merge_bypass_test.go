package pulls

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPlanMergeOffersTheBypassOnlyForBlockedAndAdmin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		merge  core.MergeInfo
		review core.ReviewDecision
		bypass bool
	}{
		{"blocked admin", core.MergeInfo{Status: core.MergeBlocked, CanMergeAsAdmin: true}, core.ReviewRequired, true},
		{"blocked not admin", core.MergeInfo{Status: core.MergeBlocked}, core.ReviewRequired, false},
		{"conflicting", core.MergeInfo{Status: core.MergeBlocked, CanMergeAsAdmin: true, Mergeable: core.MergeableConflicting}, core.ReviewRequired, false},
		{"dirty", core.MergeInfo{Status: core.MergeDirty, CanMergeAsAdmin: true, Mergeable: core.MergeableConflicting}, "", false},
		{"behind", core.MergeInfo{Status: core.MergeBehind, CanMergeAsAdmin: true}, "", false},
		{"draft", core.MergeInfo{Status: core.MergeDraft, CanMergeAsAdmin: true}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := core.PullRequestDetail{Merge: tt.merge,
				Number:         142,
				ReviewDecision: tt.review}
			p := planMerge(d)
			if p.Kind != 0 {
				t.Fatalf("kind = %v, want none", p.Kind)
			}
			if p.Bypass != tt.bypass {
				t.Errorf("Bypass = %v, want %v (%q)", p.Bypass, tt.bypass, p.Cant)
			}
			if tt.bypass && p.Unmet == "" {
				t.Error("Unmet is empty for a bypassable pull request")
			}
		})
	}
}

// A merge asked of a pull request that was clean, and that a rule holds by
// the time the user says yes, is not sent: it would go past the rule.
func TestConfirmedRefusesABlockedPullRequestNotOptedIn(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, dlg := merging(t, svc, &memDrafts{})
	stand(svc, core.MergeInfo{Status: core.MergeBlocked, CanMergeAsAdmin: true}, core.ReviewRequired, core.ChecksSuccess)
	msgs := press(t, h, "y")
	if got := svc.changes(); len(got) != 0 {
		t.Fatalf("changes = %q, want none for a pull request that became blocked", got)
	}
	if !dlg.done {
		t.Errorf("the dialog stayed open: %v", msgs)
	}
	if got := question(h); got != "" {
		t.Errorf("a question is open: %q", got)
	}
}

// A merge asked with the bypass on is not sent when, by the time the user
// says yes, no rule holds the pull request or the viewer may no longer merge
// past one.
func TestConfirmedBypassIsNotSentOnceTheStandingChanges(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		merge  core.MergeInfo
		review core.ReviewDecision
	}{
		{"unblocked", core.MergeInfo{Status: core.MergeClean}, ""},
		{"lost admin", core.MergeInfo{Status: core.MergeBlocked}, core.ReviewRequired},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, h, dlg := blockedByReview(t, true)
			press(t, h, "right")
			if !dlg.bypass {
				t.Fatal("right didn't turn the bypass on")
			}
			stand(svc, tt.merge, tt.review, core.ChecksSuccess)
			press(t, h, "y")
			if got := svc.changes(); len(got) != 0 {
				t.Errorf("changes = %q, want none once the pull request stands otherwise", got)
			}
		})
	}
}

// The modal of the pull request asks the same question, and a merge goes
// out only once the bypass is on.
func TestModalMergeDialogBypassesRulesOnlyWhenTurnedOn(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	stand(svc, core.MergeInfo{Status: core.MergeBlocked, CanMergeAsAdmin: true}, core.ReviewRequired, core.ChecksSuccess)
	h, _ := modalToMerge(t, svc)
	pressMergeOn(t, h, paneFiles)
	dlg := dialogOf(h)
	if dlg == nil || dlg.focus != focusBypass {
		t.Fatal("M in the modal opened no dialog on the bypass row")
	}
	drain(t, h, dlg.Init())
	press(t, h, "y")
	if got := svc.changes(); len(got) != 0 {
		t.Fatalf("changes = %q before the bypass was turned on, want none", got)
	}
	press(t, h, "right")
	press(t, h, "y")
	if got := svc.changes(); len(got) != 1 || !strings.HasPrefix(got[0], "merge squash ") {
		t.Errorf("changes = %q, want one squash merge", got)
	}
}
