package pager

import (
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
