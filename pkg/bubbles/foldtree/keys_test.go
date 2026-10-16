package foldtree

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	km := testKeyMap
	keytest.Complete(t, km)
	keytest.Tagged(t, km)
	keytest.HelpTags(t, km)
	keytest.NoConflicts(t, km)
}
