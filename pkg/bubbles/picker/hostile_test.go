package picker

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileItems(t *testing.T) {
	h := termtexttest.Hostile
	m := New(nil, WithItems([]Item{{Title: h, Detail: h}}), WithSize(60, 5))
	termtexttest.AssertClean(t, m.View(), 60)
}
