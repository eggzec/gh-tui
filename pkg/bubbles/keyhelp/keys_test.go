package keyhelp_test

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMap(t *testing.T) {
	km := keyhelp.TestKeys()
	keytest.Complete(t, km)
	keytest.Tagged(t, km)
	keytest.HelpTags(t, km)
	keytest.NoConflicts(t, km)
	keytest.Complete(t, keyhelp.New(keyhelp.WithKeyMap(km)).KeyMap())
}
