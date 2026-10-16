package github

import (
	"errors"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestBehindCommits(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != "/repos/golang/go/compare/feat%2Fx...main" {
			t.Errorf("path = %s, want the head first, then the base, as one segment each", got)
		}
		if q := r.URL.Query(); q.Get("per_page") != "100" || q.Get("page") != "" {
			t.Errorf("query = %v, want the first page of a hundred", q)
		}
		_, _ = w.Write([]byte(`{"status":"behind","ahead_by":250,"behind_by":0,"commits":[
			{"sha":"a1","commit":{"message":"first\n\nbody"},"parents":[{"sha":"a0"}]},
			{"sha":"a2","commit":{"message":"second"},"parents":[{"sha":"a1"}]}]}`))
	}))

	got, err := c.BehindCommits(t.Context(), golang, "feat/x", "main")
	if err != nil {
		t.Fatalf("BehindCommits: %v", err)
	}
	if got.AheadBy != 250 || len(got.Commits) != 2 {
		t.Fatalf("BehindCommits = %+v, want 2 of 250 commits", got)
	}
	if c := got.Commits[0]; c.SHA != "a1" || c.Subject != "first" || len(c.Parents) != 1 || c.Parents[0] != "a0" {
		t.Errorf("first commit = %+v", c)
	}
}

func TestBehindCommitsErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{name: "unknown ref", status: http.StatusNotFound, wantErr: core.ErrNotFound},
		{name: "no common ancestor", status: http.StatusUnprocessableEntity, wantErr: core.ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			got, err := c.BehindCommits(t.Context(), golang, "feat", "main")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.AheadBy != 0 || got.Commits != nil {
				t.Errorf("BehindCommits = %+v, want none", got)
			}
		})
	}
}
