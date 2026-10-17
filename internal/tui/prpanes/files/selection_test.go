package files

import (
	"strconv"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// commitsAt makes n commits c0..c(n-1), one a day apart, oldest first.
func commitsAt(n int) []core.PullCommit {
	cs := make([]core.PullCommit, n)
	for i := range cs {
		cs[i] = core.PullCommit{
			OID:         "c" + strconv.Itoa(i),
			CommittedAt: epoch.AddDate(0, 0, i),
		}
	}
	return cs
}

func TestHintText(t *testing.T) {
	t.Parallel()
	bound := Keys{Mark: "space", SelectAll: "A"}
	tests := []struct {
		name string
		h    Hint
		k    Keys
		want string
	}{
		{"one, bound", HintOne, bound, "at least one commit stays selected · A selects all"},
		{"one, select all unbound", HintOne, Keys{Mark: "space"}, "at least one commit stays selected"},
		{"one, rebound", HintOne, Keys{SelectAll: "ctrl+a"}, "at least one commit stays selected · ctrl+a selects all"},
		{"not a commit, bound", HintNotACommit, bound, "space selects a commit or marks a file viewed"},
		{"not a commit, unbound", HintNotACommit, Keys{SelectAll: "A"}, ""},
		{"contiguous", HintContiguous, Keys{}, "a range has to be contiguous: only this commit is selected now"},
		{"none since", HintNoneSince, Keys{}, "no commits since your last review"},
		{"gone", HintGone, Keys{}, "your reviewed commit is gone: selected the commits made after your review"},
		{"no hint", "", bound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.h.Text(tt.k); got != tt.want {
				t.Errorf("Text = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRangeMark(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		r    Range
		i, n int
		want Range
		hint Hint
	}{
		{"grow below", Range{3, 5}, 1, 8, Range{1, 5}, ""},
		{"grow above", Range{3, 5}, 7, 8, Range{3, 7}, ""},
		{"grow next to the range", Range{3, 5}, 6, 8, Range{3, 6}, ""},
		{"first leaves", Range{3, 5}, 3, 8, Range{4, 5}, ""},
		{"last leaves", Range{3, 5}, 5, 8, Range{3, 4}, ""},
		{"two commits, first leaves", Range{3, 4}, 3, 8, Range{4, 4}, ""},
		{"all, first leaves", All(4), 0, 4, Range{1, 3}, ""},
		{"only commit stays", Range{2, 2}, 2, 8, Range{2, 2}, HintOne},
		{"inside selects only it", Range{1, 5}, 3, 8, Range{3, 3}, HintContiguous},
		{"inside of all", All(3), 1, 3, Range{1, 1}, HintContiguous},
		{"out of bounds changes nothing", Range{1, 2}, 9, 4, Range{1, 2}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, hint := tt.r.Mark(tt.i, tt.n)
			if got != tt.want || hint != tt.hint {
				t.Fatalf("%v.Mark(%d, %d) = %v, %q, want %v, %q", tt.r, tt.i, tt.n, got, hint, tt.want, tt.hint)
			}
		})
	}
}

func TestRangeBasics(t *testing.T) {
	t.Parallel()
	r := All(4)
	if r.Len() != 4 || !r.Contains(0) || !r.Contains(3) || r.Contains(4) {
		t.Fatalf("All(4) = %v, Len %d", r, r.Len())
	}
	if (Range{First: 2, Last: 1}).Len() != 0 || All(0).Len() != 0 {
		t.Fatal("an empty range must have length 0")
	}
}

func TestSince(t *testing.T) {
	t.Parallel()
	cs := commitsAt(5)
	tests := []struct {
		name string
		pc   core.PullCommits
		want Range
		ok   bool
		gone bool
	}{
		{"no review", core.PullCommits{Commits: cs}, Range{}, false, false},
		{"review on head", core.PullCommits{
			Commits:    cs,
			LastReview: &core.ViewerReview{Commit: "c4", SubmittedAt: epoch.AddDate(0, 0, 4)},
		}, Range{}, false, false},
		{"review on a middle commit", core.PullCommits{
			Commits:    cs,
			LastReview: &core.ViewerReview{Commit: "c1", SubmittedAt: epoch.AddDate(0, 0, 1)},
		}, Range{2, 4}, true, false},
		{"review on the first commit", core.PullCommits{
			Commits:    cs,
			LastReview: &core.ViewerReview{Commit: "C0"},
		}, Range{1, 4}, true, false},
		{"commit gone, later commits exist", core.PullCommits{
			Commits:    cs,
			LastReview: &core.ViewerReview{Commit: "", SubmittedAt: epoch.AddDate(0, 0, 2).Add(time.Hour)},
		}, Range{3, 4}, true, true},
		{"commit gone, none later", core.PullCommits{
			Commits:    cs,
			LastReview: &core.ViewerReview{Commit: "x", SubmittedAt: epoch.AddDate(0, 0, 9)},
		}, Range{}, false, true},
		{"review on the last of a capped list", core.PullCommits{
			Commits:    cs,
			Capped:     true,
			LastReview: &core.ViewerReview{Commit: "c4", SubmittedAt: epoch.AddDate(0, 0, 3).Add(time.Hour)},
		}, Range{4, 4}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Since(tt.pc)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Since = %v, %v, want %v, %v", got, ok, tt.want, tt.ok)
			}
			if Gone(tt.pc) != tt.gone {
				t.Fatalf("Gone = %v, want %v", Gone(tt.pc), tt.gone)
			}
		})
	}
}

func TestDivider(t *testing.T) {
	t.Parallel()
	cs := commitsAt(5)
	tests := []struct {
		name   string
		review *core.ViewerReview
		want   int
		ok     bool
	}{
		{"no review", nil, 0, false},
		{"review on head", &core.ViewerReview{Commit: "c4"}, 0, false},
		{"review on a middle commit", &core.ViewerReview{Commit: "c2"}, 3, true},
		{"review on the first commit", &core.ViewerReview{Commit: "c0"}, 1, true},
		{"commit missing, later commits", &core.ViewerReview{Commit: "x", SubmittedAt: epoch.AddDate(0, 0, 1).Add(time.Hour)}, 2, true},
		{"commit missing, none later", &core.ViewerReview{Commit: "x", SubmittedAt: epoch.AddDate(0, 0, 8)}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Divider(core.PullCommits{Commits: cs, LastReview: tt.review})
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Divider = %d, %v, want %d, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
