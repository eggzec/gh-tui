package checks

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// The names, workflows and descriptions of checks are text from GitHub,
// which the panes draw.
func TestViewCleansHostileChecks(t *testing.T) {
	h := termtexttest.Hostile
	hostile := func() *fake {
		f := newFake()
		lint := &f.checks.Runs[0]
		lint.Name, lint.Workflow = h, h
		st := &f.checks.Statuses[0]
		st.Context, st.Description = h, h
		return f
	}
	for _, size := range [][2]int{{wideW, wideH}, {narrowW, narrowH}} {
		for _, keys := range [][]string{nil, {"enter"}, {"down", "down", "enter"}} {
			s, host := newStep(t, hostile(), size[0], size[1])
			host.keys(keys...)
			termtexttest.AssertClean(t, s.View(), size[0])
		}
	}
}
