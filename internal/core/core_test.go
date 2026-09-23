package core

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestParseRepoRef(t *testing.T) {
	tests := []struct {
		in      string
		want    RepoRef
		wantErr bool
	}{
		{in: "eggzec/gh-tui", want: RepoRef{Owner: "eggzec", Name: "gh-tui"}},
		{in: "eggzec", wantErr: true},
		{in: "/gh-tui", wantErr: true},
		{in: "eggzec/", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseRepoRef(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRepoRef(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
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
