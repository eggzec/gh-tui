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
	if got := question(h); strings.Contains(got, "merge") && got != "" {
		t.Errorf("a question is open: %q", got)
	}
}
