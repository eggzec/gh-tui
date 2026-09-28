package toast

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileText(t *testing.T) {
	m := New(testDuration, testErrorDuration, WithSize(80, 0))
	m.Push(Error, termtexttest.Hostile)
	termtexttest.AssertClean(t, m.View(), 80)
}
