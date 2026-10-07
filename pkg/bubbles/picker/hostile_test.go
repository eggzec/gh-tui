package picker

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileItems(t *testing.T) {
	h := termtexttest.Hostile
	m := New(nil, WithKeyMap(testKeys(t)), WithItems([]Item{{Title: h, Detail: h}}), WithSize(60, 5))
	termtexttest.AssertClean(t, m.View(), 60)
}

func TestViewCleansHostileItemsWithMarks(t *testing.T) {
	h := termtexttest.Hostile
	m := New(nil, WithKeyMap(testKeys(t)), WithItems([]Item{{Title: h, Detail: h, Value: 1}, {Title: h, Value: 2}}),
		WithMarks("[x]", h), WithSize(60, 6))
	m.SetMarked([]any{1})
	termtexttest.AssertClean(t, m.View(), 60)
}
