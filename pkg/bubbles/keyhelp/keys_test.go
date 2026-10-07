package keyhelp_test

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMap(t *testing.T) {
	keytest.Complete(t, keyhelp.DefaultKeyMap())
	keytest.Tagged(t, keyhelp.DefaultKeyMap())
	keytest.HelpTags(t, keyhelp.DefaultKeyMap())
	keytest.NoConflicts(t, keyhelp.DefaultKeyMap())
	keytest.Complete(t, keyhelp.New().KeyMap())
}
