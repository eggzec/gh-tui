package checktree

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// Each level has a glyph of its own, and a level that GitHub adds later has the
// notice's.
func TestStylesLevel(t *testing.T) {
	t.Parallel()
	s := styles("unicode")
	failure, warning, notice := s.Level(core.AnnotationFailure), s.Level(core.AnnotationWarning), s.Level(core.AnnotationNotice)
	if failure == warning || warning == notice || failure == notice {
		t.Errorf("levels share a glyph: %q %q %q", failure, warning, notice)
	}
	if got := s.Level("later"); got != notice {
		t.Errorf("an unknown level is %q, want the notice %q", got, notice)
	}
	for _, g := range []string{failure, warning, notice} {
		if g == "" {
			t.Error("a level has no glyph")
		}
	}
}
