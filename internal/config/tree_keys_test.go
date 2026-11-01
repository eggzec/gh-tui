package config

import (
	"slices"
	"testing"
)

// TestTreeContextsBindHL checks that every context of a pane that shows a
// tree binds h to collapse and l to expand, and left and right too but for
// the panes that keep them for something else, so that a tree moves the same
// way in each.
func TestTreeContextsBindHL(t *testing.T) {
	t.Parallel()
	// The description keeps left and right off its tree.
	hOnly := map[string]bool{"pull_description": true}
	keys := Default().Keys
	for _, ctx := range []string{
		"files", "references", "pull_description", "pull_conversation",
		"pull_merge", "pull_files", "pull_involved", "pull_development",
	} {
		want := map[string][]string{"expand": {"l", "right"}, "collapse": {"h", "left"}}
		if hOnly[ctx] {
			want = map[string][]string{"expand": {"l"}, "collapse": {"h"}}
		}
		for action, ks := range want {
			got := keys.Of(ctx + "." + action)
			for _, k := range ks {
				if !slices.Contains(got, k) {
					t.Errorf("%s.%s = %v, want it to bind %q", ctx, action, got, k)
				}
			}
		}
	}
}

func TestUITreeGuides(t *testing.T) {
	t.Parallel()
	if got := Default().UI.TreeGuides; got != GuidesConnectors {
		t.Errorf("default tree_guides = %q, want %q", got, GuidesConnectors)
	}
	for _, tt := range []struct {
		guides string
		ok     bool
	}{
		{GuidesConnectors, true},
		{GuidesIndent, true},
		{"", false},
		{"Indent", false},
		{"lines", false},
	} {
		cfg := Default()
		cfg.UI.TreeGuides = tt.guides
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("tree_guides %q: Validate() = %v, want ok = %v", tt.guides, err, tt.ok)
		}
	}
}
