package repos

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPinned(t *testing.T) {
	outside := core.Repo{Ref: ref("golang/go"), Language: "Go", Stars: 130_000}
	errDown := errors.New("502 Bad Gateway")
	tests := []struct {
		name     string
		pinned   []string
		pageSize int
		getErr   map[core.RepoRef]error
		want     []string
		wantErr  string
	}{
		{
			name:   "pinned come first, in order, and only once",
			pinned: []string{"torvalds/linux", "golang/go", "Eggzec/CLI", "eggzec/cli"},
			want: []string{
				"torvalds/linux", "golang/go", "eggzec/cli",
				"eggzec/gh-tui", "charmbracelet/bubbletea", "eggzec/dotfiles",
				"eggzec/old-experiments", "a-very-long-organization-name/with-a-long-repository-name", "eggzec/notes",
			},
		},
		{
			name:     "deduplicated across pages",
			pinned:   []string{"eggzec/notes", "eggzec/gh-tui"},
			pageSize: 3,
			want: []string{
				"eggzec/notes", "eggzec/gh-tui",
				"charmbracelet/bubbletea", "eggzec/dotfiles", "eggzec/cli",
				"eggzec/old-experiments", "torvalds/linux", "a-very-long-organization-name/with-a-long-repository-name",
			},
		},
		{
			name:   "a pinned repo that is gone is left out",
			pinned: []string{"eggzec/renamed", "eggzec/notes"},
			want: []string{
				"eggzec/notes", "eggzec/gh-tui", "charmbracelet/bubbletea", "eggzec/dotfiles",
				"eggzec/cli", "eggzec/old-experiments", "torvalds/linux",
				"a-very-long-organization-name/with-a-long-repository-name",
			},
		},
		{
			name:    "a pinned repo that fails shows the error",
			pinned:  []string{"eggzec/notes"},
			getErr:  map[core.RepoRef]error{ref("eggzec/notes"): errDown},
			wantErr: "502 Bad Gateway",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(sampleRepos()...)
			f.extra = []core.Repo{outside}
			f.getErr = tt.getErr
			if tt.pageSize > 0 {
				f.pageSize = tt.pageSize
			}
			refs := make([]core.RepoRef, len(tt.pinned))
			for i, p := range tt.pinned {
				refs[i] = ref(p)
			}
			s := newSection(t, f, 200, 20, WithPinned(refs))
			if tt.wantErr != "" {
				if err := s.feed.Err(); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Err() = %v, want %q", err, tt.wantErr)
				}
				return
			}
			got := make([]string, 0, s.feed.Len())
			for range s.feed.Len() {
				r, _ := s.feed.Selected()
				got = append(got, strings.ToLower(r.Ref.String()))
				keys(s, "down")
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("rows = %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestStarPinned(t *testing.T) {
	f := newFake(sampleRepos()...)
	s := newSection(t, f, 120, 10, WithPinned([]core.RepoRef{ref("torvalds/linux")}))
	msgs := keys(s, "s")
	if len(msgs) != 1 || f.sentOps()[0] != "star torvalds/linux" {
		t.Fatalf("messages = %#v, sent %q; want a star of torvalds/linux", msgs, f.sentOps())
	}
	assertStarred(t, f, s, true)
}
