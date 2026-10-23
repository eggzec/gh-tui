package actions

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

func TestSortLess(t *testing.T) {
	t.Parallel()
	now := time.Now()
	runs := []core.Run{
		{ID: 1, Name: "lint", Branch: "main", CreatedAt: now},
		{ID: 2, Name: "build", Branch: "", CreatedAt: now.Add(-time.Hour)},
		{ID: 3, Name: "build", Branch: "dev", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: 4, Name: "build", Branch: "dev", CreatedAt: now.Add(-time.Minute)},
	}
	levels := []orderpick.Choice{{Value: "workflow"}, {Value: "branch"}}
	slices.SortStableFunc(runs, sortLess(levels))
	got := make([]int64, 0, len(runs))
	for _, r := range runs {
		got = append(got, r.ID)
	}
	// build/dev newest first, then build without a branch, then lint.
	if want := []int64{4, 3, 2, 1}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestGroupsOfIgnoresUnknownFields(t *testing.T) {
	t.Parallel()
	g := groupsOf([]orderpick.Choice{{Value: "event"}, {Value: "nope"}, {Value: "actor", Desc: true}})
	if len(g) != 2 || !g[1].Desc {
		t.Fatalf("groups = %+v", g)
	}
	if k := g[0].Key(core.Run{Event: "push"}); k != "push" {
		t.Fatalf("event key = %q", k)
	}
}

func TestSetGroupsAndSortToggle(t *testing.T) {
	t.Parallel()
	m := &Modal{levels: defaultGroups()}
	if got := len(m.groupFuncs()); got != 1 {
		t.Fatalf("default groups = %d, want 1", got)
	}
	if m.sortFunc() != nil {
		t.Fatal("a sort is in force by default")
	}
	if _, ok := m.toggleSort(); ok {
		t.Fatal("toggleSort with no levels reported a change")
	}
	m.off = true
	if m.groupFuncs() != nil {
		t.Fatal("groups remain while switched off")
	}
}
