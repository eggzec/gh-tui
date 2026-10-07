package pager

import (
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileNames(t *testing.T) {
	h := termtexttest.Hostile
	m := open(t, h, "text", WithSize(60, 5))
	termtexttest.AssertClean(t, m.View(), 60)
	m.SetMessage(h, h)
	termtexttest.AssertClean(t, m.View(), 60)
}

// TestViewCleansHostileErrors fails a load with an error that holds text
// from outside, which the pager shows when the app doesn't word it.
func TestViewCleansHostileErrors(t *testing.T) {
	m := fresh(t, WithSize(60, 5))
	m.SetError("main.go", errors.New(termtexttest.Hostile))
	termtexttest.AssertClean(t, m.View(), 60)
}
