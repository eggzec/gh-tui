package logview

import (
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

func TestPositionRoundTrip(t *testing.T) {
	texts := make([]string, 100)
	for i := range texts {
		texts[i] = fmt.Sprintf("line %d", i)
	}
	m := view(t, plainLines(texts...))
	m.SetSize(80, 12)
	want := pager.Position{Cursor: 40, Top: 35}
	m.SetPosition(want)
	got := m.Position()
	if got != want {
		t.Fatalf("position %+v after SetPosition(%+v)", got, want)
	}

	other := view(t, plainLines(texts...))
	other.SetSize(80, 12)
	other.SetPosition(got)
	if other.Position() != want || cursorText(other) != "line 40" {
		t.Errorf("restored position %+v on %q, want %+v on line 40", other.Position(), cursorText(other), want)
	}
}
