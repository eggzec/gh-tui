package ui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

func TestThemeTreeView(t *testing.T) {
	t.Parallel()
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	th := NewTheme(p, true)
	for _, tt := range []struct {
		mode string
		want treeview.GuideMode
	}{
		{config.GuidesConnectors, treeview.Connectors},
		{config.GuidesIndent, treeview.Indent},
	} {
		if got := th.WithTreeGuides(tt.mode).Tree(NewIcons(config.IconsUnicode)).Guides; got != tt.want {
			t.Errorf("guides %q = %v, want %v", tt.mode, got, tt.want)
		}
	}

	ic := NewIcons(config.IconsUnicode)
	s := th.Tree(ic)
	if s.CursorGlyph != ic.Cursor || s.OpenGlyph != ic.Unfolded || s.ClosedGlyph != ic.Folded {
		t.Errorf("glyphs = %q %q %q, want the icons'", s.CursorGlyph, s.OpenGlyph, s.ClosedGlyph)
	}
	if got := th.Tree(NewIcons(config.IconsASCII)).Branch; got != "|- " {
		t.Errorf("ASCII branch = %q, want an ASCII connector", got)
	}
}
