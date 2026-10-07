package cmdline

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys(t))
	keytest.Tagged(t, testKeys(t))
	keytest.HelpTags(t, testKeys(t))
	keytest.NoConflicts(t, testKeys(t))
	keytest.NoConflicts(t, New(testHistoryLimit, WithComplete(repoComplete), WithHistory([]string{"quit"})))
}

// A command line without keys takes none, and one whose action is unbound
// still lists it, without a key.
func TestWithoutKeys(t *testing.T) {
	m := New(testHistoryLimit)
	m.Open("goto")
	m, sent := press(t, m, enter)
	if sent != nil || !m.Focused() {
		t.Errorf("enter sent %v, focused %v; want a command line without keys to ignore it", sent, m.Focused())
	}
	km := NewKeyMap(func(action string) []string {
		if action == "cancel" {
			return []string{"esc"}
		}
		return nil
	})
	if km.Submit.Enabled() || km.Submit.Help().Key != "" || km.Submit.Help().Desc != "run" {
		t.Errorf("unbound run = enabled %v, help %+v; want a disabled binding that says run", km.Submit.Enabled(), km.Submit.Help())
	}
	if !km.Cancel.Enabled() {
		t.Error("cancel lost its key")
	}
}
