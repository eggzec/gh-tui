package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseRepoRef(t *testing.T) {
	tests := []struct {
		in      string
		want    RepoRef
		wantErr bool
		errText string // a substring of the error, when one is wanted
	}{
		{in: "eggzec/gh-tui", want: RepoRef{Owner: "eggzec", Name: "gh-tui"}},
		{in: "eggzec", wantErr: true},
		{in: "/gh-tui", wantErr: true},
		{in: "eggzec/", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "", wantErr: true},
		{in: "Eggzec/GH-TUI", want: RepoRef{Owner: "Eggzec", Name: "GH-TUI"}},
		{in: "octocat_acme/.github", want: RepoRef{Owner: "octocat_acme", Name: ".github"}},
		{in: "a-b/c.d_e-f", want: RepoRef{Owner: "a-b", Name: "c.d_e-f"}},
		{in: "-eggzec/gh-tui", wantErr: true, errText: "may not start with '-'"},
		{in: "egg.zec/gh-tui", wantErr: true, errText: "may hold only letters, digits, '-' and '_'"},
		{in: "egg zec/gh-tui", wantErr: true},
		{in: "eggzec/gh tui", wantErr: true, errText: "may hold only letters, digits, '.', '-' and '_'"},
		{in: "eggzec/gh-tui#1", wantErr: true},
		{in: "eggzec/.", wantErr: true},
		{in: "eggzec/..", wantErr: true, errText: `name may not be ".."`},
		{in: "eggzec/gh-tüi", wantErr: true},
		{in: "eggzec/" + strings.Repeat("a", 100), want: RepoRef{Owner: "eggzec", Name: strings.Repeat("a", 100)}},
		{in: "eggzec/" + strings.Repeat("a", 101), wantErr: true, errText: "longer than 100"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseRepoRef(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRepoRef(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.errText) {
				t.Errorf("ParseRepoRef(%q) error = %q, want it to contain %q", tt.in, err, tt.errText)
			}
			if got != tt.want {
				t.Errorf("ParseRepoRef(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRepoRefRoundTrip(t *testing.T) {
	r := RepoRef{Owner: "eggzec", Name: "gh-tui"}
	got, err := ParseRepoRef(r.String())
	if err != nil || got != r {
		t.Errorf("round trip = %v, %v; want %v", got, err, r)
	}
}

func TestRateLimitErrorIs(t *testing.T) {
	err := fmt.Errorf("list pulls: %w", &RateLimitError{Reset: time.Now()})
	if !errors.Is(err, ErrRateLimited) {
		t.Error("wrapped RateLimitError does not match ErrRateLimited")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("RateLimitError matches ErrNotFound")
	}
	if _, ok := errors.AsType[*RateLimitError](err); !ok {
		t.Error("errors.AsType failed to extract RateLimitError")
	}
}

func TestPageLast(t *testing.T) {
	if !(Page[int]{}).Last() {
		t.Error("empty Next should be last")
	}
	if (Page[int]{Next: "abc"}).Last() {
		t.Error("non-empty Next should not be last")
	}
}

func TestChecksCount(t *testing.T) {
	c := Checks{
		Runs: []Check{
			{Status: RunCompleted, Conclusion: ConclusionSuccess},
			{Status: RunCompleted, Conclusion: ConclusionSkipped},
			{Status: RunCompleted, Conclusion: ConclusionFailure},
			{Status: RunCompleted, Conclusion: ConclusionCancelled},
			{Status: RunCompleted, Conclusion: ConclusionActionRequired},
			{Status: RunInProgress},
			{Status: RunQueued},
		},
		Statuses: []StatusContext{{State: "success"}, {State: "error"}, {State: "pending"}},
	}
	if f, p, ok := c.Count(); f != 4 || p != 3 || ok != 3 {
		t.Errorf("Count = %d failing, %d pending, %d passing; want 4, 3 and 3", f, p, ok)
	}
	if !c.Pending() {
		t.Error("checks in progress aren't pending")
	}
	if (Checks{Runs: c.Runs[:3]}).Pending() {
		t.Error("completed checks are pending")
	}
}
