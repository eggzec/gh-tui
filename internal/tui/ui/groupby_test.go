package ui

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

func TestRepoGroupFields(t *testing.T) {
	t.Parallel()
	repo := func(f func(*core.Repo)) core.Repo {
		var r core.Repo
		f(&r)
		return r
	}
	tests := []struct {
		name  string
		field string
		repo  core.Repo
		want  string
	}{
		{"owner", "owner", repo(func(r *core.Repo) { r.Ref.Owner = "cli" }), "cli"},
		{"language", "language", repo(func(r *core.Repo) { r.Language = "Go" }), "Go"},
		{"no language", "language", core.Repo{}, "none"},
		{"private", "visibility", repo(func(r *core.Repo) { r.Private = true }), "private"},
		{"public", "visibility", core.Repo{}, "public"},
		{"archived wins", "kind", repo(func(r *core.Repo) { r.Archived, r.Mirror, r.Fork, r.Template = true, true, true, true }), "archived"},
		{"mirror over fork", "kind", repo(func(r *core.Repo) { r.Mirror, r.Fork, r.Template = true, true, true }), "mirror"},
		{"fork over template", "kind", repo(func(r *core.Repo) { r.Fork, r.Template = true, true }), "fork"},
		{"template", "kind", repo(func(r *core.Repo) { r.Template = true }), "template"},
		{"source", "kind", core.Repo{}, "source"},
		{"unknown role", "role", core.Repo{}, "unknown"},
	}
	fields := RepoGroupFields()
	hits := HitGroupFields()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			i := slices.IndexFunc(fields, func(f GroupField[core.Repo]) bool { return f.Name == tt.field })
			if i < 0 {
				t.Fatalf("no field %q", tt.field)
			}
			if got := fields[i].Key(tt.repo); got != tt.want {
				t.Errorf("repo key = %q, want %q", got, tt.want)
			}
			if got := hits[i].Key(core.SearchHit{Repo: tt.repo}); got != tt.want {
				t.Errorf("hit key = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGrouping(t *testing.T) {
	t.Parallel()
	g := Grouping[core.Repo]{Fields: RepoGroupFields()}
	if g.Toggle() {
		t.Error("Toggle switched a grouping that has no levels")
	}
	if g.Groups() != nil || g.Chip(" · ") != "" {
		t.Error("a grouping with no levels has groups or a chip")
	}
	g.Choose([]orderpick.Choice{{Value: "owner"}, {Value: "kind"}})
	if got := len(g.Groups()); got != 2 {
		t.Fatalf("Groups = %d, want 2", got)
	}
	if g.Chip(" · ") != "" {
		t.Error("the chip shows while the grouping is on")
	}
	if !g.Toggle() || !g.Off {
		t.Fatal("Toggle did not switch the grouping off")
	}
	if g.Groups() != nil {
		t.Error("Groups returned groups while off")
	}
	if g.Chip(" · ") == "" {
		t.Error("no chip while off")
	}
	g.Choose([]orderpick.Choice{{Value: "language"}})
	if g.Off || len(g.Groups()) != 1 {
		t.Errorf("Choose did not switch the grouping back on: off=%v groups=%d", g.Off, len(g.Groups()))
	}
}
