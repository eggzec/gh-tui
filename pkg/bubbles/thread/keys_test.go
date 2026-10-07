package thread

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys())
	keytest.Tagged(t, testKeys())
	keytest.HelpTags(t, testKeys())
	keytest.NoConflicts(t, testKeys())
}
