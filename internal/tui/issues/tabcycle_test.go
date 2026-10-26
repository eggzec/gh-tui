package issues

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestTabCycle(t *testing.T) {
	t.Parallel()
	order := []core.StateFilter{core.FilterOpen, core.FilterClosed, core.FilterAll}
	for i, st := range order {
		if got, want := nextTab(st, 1), order[(i+1)%len(order)]; got != want {
			t.Errorf("next of %q = %q, want %q", st, got, want)
		}
		if got, want := nextTab(st, -1), order[(i+len(order)-1)%len(order)]; got != want {
			t.Errorf("prev of %q = %q, want %q", st, got, want)
		}
	}
	if got := nextTab("nope", 1); got != core.FilterClosed {
		t.Errorf("next of an unknown state = %q, want the second tab", got)
	}
}
