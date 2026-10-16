package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
)

// Every glyph is drawn, and one cell wide, in every set.
func TestGlyphsAreOneCellWide(t *testing.T) {
	for _, set := range iconSets {
		ic := NewIcons(set)
		for g := range NumGlyphs {
			s := ic.Glyph(g)
			if w := ansi.StringWidth(s); w != 1 || len([]rune(s)) != 1 {
				t.Errorf("%s: glyph %d %q is %d cells wide, want 1", set, g, s, w)
			}
		}
	}
}

func TestGlyphsASCII(t *testing.T) {
	ic := NewIcons(config.IconsASCII)
	for g := range NumGlyphs {
		for _, r := range ic.Glyph(g) {
			if r < ' ' || r > '~' {
				t.Errorf("glyph %d %q is not printable ASCII", g, ic.Glyph(g))
			}
		}
	}
}

// The glyphs that share a row of a view differ, so that the set tells them
// apart without color.
func TestGlyphsTellApart(t *testing.T) {
	rows := [][]Glyph{
		{GlyphApproved, GlyphChanges, GlyphCommented, GlyphRequested, GlyphDismissed},
		{GlyphUnresolved, GlyphResolved, GlyphOutdated},
		{GlyphCheckPass, GlyphCheckFail},
		{GlyphCheckPass, GlyphCheckPending},
	}
	for _, set := range iconSets {
		ic := NewIcons(set)
		for _, row := range rows {
			seen := map[string]Glyph{}
			for _, g := range row {
				s := ic.Glyph(g)
				if prev, ok := seen[s]; ok {
					t.Errorf("%s: glyphs %d and %d share %q", set, prev, g, s)
				}
				seen[s] = g
			}
		}
	}
}

// Checks that are waiting and checks that failed may share a glyph in a set,
// but then their colors differ.
func TestGlyphChecksTellApart(t *testing.T) {
	for _, set := range iconSets {
		ic := NewIcons(set)
		gs := NewGlyphStyles(NewTheme(config.Palette{}, true), ic)
		for _, pair := range [][2]Glyph{{GlyphCheckPass, GlyphCheckFail}, {GlyphCheckPass, GlyphCheckPending}, {GlyphCheckFail, GlyphCheckPending}} {
			if ic.Glyph(pair[0]) == ic.Glyph(pair[1]) && gs.Render(pair[0]) == gs.Render(pair[1]) {
				t.Errorf("%s: glyphs %d and %d look the same", set, pair[0], pair[1])
			}
		}
	}
}

func TestGlyphOutOfRange(t *testing.T) {
	ic := NewIcons(config.IconsNerd)
	gs := NewGlyphStyles(NewTheme(config.Palette{}, true), ic)
	for _, g := range []Glyph{-1, NumGlyphs, NumGlyphs + 3} {
		if got := ic.Glyph(g); got != "" {
			t.Errorf("Glyph(%d) = %q, want none", g, got)
		}
		if got := gs.Render(g); got != "" {
			t.Errorf("Render(%d) = %q, want none", g, got)
		}
	}
}

func TestGlyphStylesFollowBackground(t *testing.T) {
	ic := NewIcons(config.IconsUnicode)
	dark := NewGlyphStyles(NewTheme(config.Palette{}, true), ic)
	light := NewGlyphStyles(NewTheme(config.Palette{}, false), ic)
	for _, g := range []Glyph{GlyphApproved, GlyphChanges, GlyphCheckPending, GlyphResolved} {
		d, l := dark.Render(g), light.Render(g)
		if d == l {
			t.Errorf("glyph %d renders %q on both backgrounds", g, d)
		}
		if ansi.Strip(d) != ic.Glyph(g) || ansi.Strip(l) != ic.Glyph(g) {
			t.Errorf("glyph %d renders %q and %q, want %q inside", g, d, l, ic.Glyph(g))
		}
	}
}

// A glyph without a color of its own is dim, like the rest of the muted text.
func TestGlyphStylesDimOthers(t *testing.T) {
	th := NewTheme(config.Palette{Muted: "#888888"}, true)
	gs := NewGlyphStyles(th, NewIcons(config.IconsUnicode))
	if got, want := gs.Render(GlyphCommit), th.Muted.Render("•"); got != want {
		t.Errorf("Render(GlyphCommit) = %q, want %q", got, want)
	}
}
