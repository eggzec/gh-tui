package search

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// A hit opens at the address GitHub gave, and at the one built from its
// parts only when it gave none.
func TestHitURL(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 140, 38)
	repo := core.RepoRef{Owner: "o", Name: "r"}
	tests := []struct {
		name string
		hit  core.SearchHit
		want string
	}{
		{"commit with address", core.SearchHit{Kind: core.SearchCommits, Commit: core.SearchCommit{Repo: repo, SHA: "abc", URL: "https://x/c"}}, "https://x/c"},
		{"commit without", core.SearchHit{Kind: core.SearchCommits, Commit: core.SearchCommit{Repo: repo, SHA: "abc"}}, ui.WebURL(s.host, "o/r/commit/abc")},
		{"user with address", core.SearchHit{Kind: core.SearchUsers, User: core.SearchUser{Login: "u", URL: "https://x/u"}}, "https://x/u"},
		{"user without", core.SearchHit{Kind: core.SearchUsers, User: core.SearchUser{Login: "u"}}, ui.WebURL(s.host, "u")},
	}
	for _, tt := range tests {
		if got := s.hitURL(tt.hit); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
