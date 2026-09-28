package filterform

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestLabelOfCleansHostileItems(t *testing.T) {
	h := termtexttest.Hostile
	termtexttest.AssertClean(t, labelOf([]Item{{Value: "bug", Label: h}}, "bug"), 200)
	termtexttest.AssertClean(t, labelOf(nil, h), 200)
}
