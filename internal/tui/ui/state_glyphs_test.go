package ui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
)

func TestStateGlyphs(t *testing.T) {
	t.Parallel()
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	th := NewTheme(p, true)
	icons := NewIcons(config.IconsASCII)
	got := StateGlyphs(th, icons)
	for i := range got {
		s := State(i)
		if want := th.State(s).Render(icons.State(s)); got[i] != want {
			t.Errorf("state %d: got %q, want %q", i, got[i], want)
		}
	}
}
