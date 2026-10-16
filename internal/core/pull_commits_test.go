package core

import (
	"reflect"
	"testing"
	"time"
)

func TestSinceLastReview(t *testing.T) {
	t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	commit := func(oid string, m int) PullCommit { return PullCommit{OID: oid, CommittedAt: at(m)} }
	a, b, c, d := commit("aaa", 0), commit("bbb", 10), commit("ccc", 20), commit("ddd", 30)
	list := []PullCommit{a, b, c, d}

	tests := []struct {
		name string
		pc   PullCommits
		want SinceResult
	}{
		{"no review", PullCommits{Commits: list}, SinceResult{Reason: SinceNoReview}},
		{"on the head", PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "ddd", SubmittedAt: at(40)}}, SinceResult{Reason: SinceOnHead}},
		{"in the middle", PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "bbb", SubmittedAt: at(15)}}, SinceResult{Commits: []PullCommit{c, d}, Reason: SinceExact}},
		{"on the first", PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "aaa", SubmittedAt: at(5)}}, SinceResult{Commits: []PullCommit{b, c, d}, Reason: SinceExact}},
		{"the SHA's case is ignored", PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "CCC", SubmittedAt: at(25)}}, SinceResult{Commits: []PullCommit{d}, Reason: SinceExact}},
		{
			"a commit the list lacks falls back on the dates",
			PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "gone", SubmittedAt: at(15)}},
			SinceResult{Commits: []PullCommit{c, d}, Reason: SinceApproximate, Approximate: true},
		},
		{
			"a commit that GitHub forgot",
			PullCommits{Commits: list, LastReview: &ViewerReview{SubmittedAt: at(15)}},
			SinceResult{Commits: []PullCommit{c, d}, Reason: SinceApproximate, Approximate: true},
		},
		{
			"a commit at the very time of the review is not after it",
			PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "gone", SubmittedAt: at(20)}},
			SinceResult{Commits: []PullCommit{d}, Reason: SinceApproximate, Approximate: true},
		},
		{
			"every commit before the review",
			PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "gone", SubmittedAt: at(60)}},
			SinceResult{Reason: SinceApproximate, Approximate: true},
		},
		{
			"commits of equal time are all after an earlier review",
			PullCommits{Commits: []PullCommit{commit("x", 10), commit("y", 10)}, LastReview: &ViewerReview{Commit: "gone", SubmittedAt: at(9)}},
			SinceResult{Commits: []PullCommit{commit("x", 10), commit("y", 10)}, Reason: SinceApproximate, Approximate: true},
		},
		{
			"commits of equal time are none after a review at that time",
			PullCommits{Commits: []PullCommit{commit("x", 10), commit("y", 10)}, LastReview: &ViewerReview{Commit: "gone", SubmittedAt: at(10)}},
			SinceResult{Reason: SinceApproximate, Approximate: true},
		},
		{
			"a review on a capped list's last commit does not know the head",
			PullCommits{Commits: list, Capped: true, LastReview: &ViewerReview{Commit: "ddd", SubmittedAt: at(25)}},
			SinceResult{Commits: []PullCommit{d}, Reason: SinceApproximate, Approximate: true},
		},
		{
			"a review inside a capped list is exact",
			PullCommits{Commits: list, Capped: true, LastReview: &ViewerReview{Commit: "bbb", SubmittedAt: at(15)}},
			SinceResult{Commits: []PullCommit{c, d}, Reason: SinceExact},
		},
		{"no commits", PullCommits{LastReview: &ViewerReview{Commit: "aaa", SubmittedAt: at(1)}}, SinceResult{Reason: SinceApproximate, Approximate: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SinceLastReview(tt.pc)
			if got.Reason != tt.want.Reason || got.Approximate != tt.want.Approximate || !reflect.DeepEqual(got.Commits, tt.want.Commits) {
				t.Errorf("SinceLastReview = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSinceLastReviewDoesNotShareTheList(t *testing.T) {
	list := []PullCommit{{OID: "a"}, {OID: "b"}, {OID: "c"}}
	got := SinceLastReview(PullCommits{Commits: list, LastReview: &ViewerReview{Commit: "a"}})
	got.Commits[0].OID = "changed"
	if list[1].OID != "b" {
		t.Errorf("the result shares the list: %+v", list)
	}
}
