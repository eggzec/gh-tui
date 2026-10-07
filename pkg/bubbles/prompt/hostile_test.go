package prompt

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileTitle(t *testing.T) {
	m := newKeyed(t, WithTitle(termtexttest.Hostile), WithSize(60, 5))
	m.Focus()
	termtexttest.AssertClean(t, m.View(), 60)
}
