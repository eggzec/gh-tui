package feed

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, DefaultKeyMap())
	keytest.NoConflicts(t, DefaultKeyMap())
}
