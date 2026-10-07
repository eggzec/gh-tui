package finder

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys(t))
	keytest.Tagged(t, testKeys(t))
	keytest.HelpTags(t, testKeys(t))
	keytest.NoConflicts(t, testKeys(t))
}
