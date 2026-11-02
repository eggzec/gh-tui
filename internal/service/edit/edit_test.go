package edit

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPatch(t *testing.T) {
	items := []int{1, 2, 3}
	got, ok := Patch(items, func(n int) bool { return n == 2 }, func(n *int) { *n = 20 })
	if !ok || !slices.Equal(got, []int{1, 20, 3}) {
		t.Errorf("Patch = %v, %v", got, ok)
	}
	if !slices.Equal(items, []int{1, 2, 3}) {
		t.Errorf("Patch changed its input: %v", items)
	}
	if _, ok := Patch(items, func(n int) bool { return n == 9 }, func(*int) {}); ok {
		t.Error("Patch of no match reports a change")
	}
}

func TestAt(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := At(core.Edited{}, now); !got.Equal(now) {
		t.Errorf("At of zero = %v, want now", got)
	}
	then := now.Add(-time.Hour)
	if got := At(core.Edited{At: then}, now); !got.Equal(then) {
		t.Errorf("At = %v, want %v", got, then)
	}
}

func TestMerged(t *testing.T) {
	name := func(s string) string { return s }
	in := []string{"bug", "Docs"}
	got := Merged(in, []string{"new", "bug"}, []string{"docs"}, name)
	if !slices.Equal(got, []string{"bug", "new"}) {
		t.Errorf("Merged = %v", got)
	}
	if !slices.Equal(in, []string{"bug", "Docs"}) {
		t.Errorf("Merged changed its input: %v", in)
	}
}
