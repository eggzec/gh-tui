package owner

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestTabCycleWraps(t *testing.T) {
	t.Parallel()
	for _, kind := range []core.OwnerKind{core.OwnerUser, core.OwnerOrg} {
		tabs := tabsOf(kind)
		c := ui.Cycle[tab](tabs)
		for i, tb := range tabs {
			if got, want := c.Next(tb), tabs[(i+1)%len(tabs)]; got != want {
				t.Errorf("%v: next of %v = %v, want %v", kind, tb, got, want)
			}
			if got, want := c.Prev(tb), tabs[(i+len(tabs)-1)%len(tabs)]; got != want {
				t.Errorf("%v: prev of %v = %v, want %v", kind, tb, got, want)
			}
		}
	}
}
