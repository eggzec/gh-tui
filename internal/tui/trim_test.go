package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// trimmedModal is a modal whose title in the top edge has parts.
type trimmedModal struct {
	linkedModal
	parts ui.TrimTitle
}

func (t *trimmedModal) TrimTitle() ui.TrimTitle { return t.parts }

// The top edge shows the glyph, the number and the title; a long title is cut
// with an ellipsis and the number stays, and the glyph keeps its own style.
func TestFrameTrimTitle(t *testing.T) {
	t.Parallel()
	const glyph = "\x1b[31m*\x1b[m"
	long := strings.Repeat("long title ", 12)
	for _, tt := range []struct {
		name   string
		width  int
		parts  ui.TrimTitle
		want   string
		styled bool
	}{
		{"loading", 80, ui.TrimTitle{Name: "#7"}, "╭─ #7 ─", false},
		{"loaded", 80, ui.TrimTitle{Lead: glyph, Name: "#7", Detail: "Fix it"}, "╭─ * #7 Fix it ─", true},
		{"other repository", 80, ui.TrimTitle{Lead: glyph, Name: "o/r#7", Detail: "Fix it"}, "╭─ * o/r#7 Fix it ─", true},
		{"long", 60, ui.TrimTitle{Lead: glyph, Name: "#7", Detail: long}, "╭─ * #7 long title long title long title lon… ─╮", true},
		{"narrow", 12, ui.TrimTitle{Lead: glyph, Name: "#7", Detail: long}, "╭─ #7 ", false},
		{"hostile", 80, ui.TrimTitle{Name: "#7", Detail: uitest.HostileTitle}, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestApp(t)
			m.Update(tea.WindowSizeMsg{Width: tt.width, Height: 12})
			mod := &trimmedModal{parts: tt.parts}
			mod.link = ui.WebURL("github.com", "o/r/pull/7")
			mod.title = "#7"
			run(m, ui.OpenModal(mod))
			top, _, _ := strings.Cut(m.frame(mod), "\n")
			plain := ansi.Strip(top)
			if tt.want != "" && !strings.HasPrefix(plain, tt.want) {
				t.Errorf("top edge %q, want it to start with %q", plain, tt.want)
			}
			if !strings.Contains(plain, "#7") {
				t.Errorf("top edge %q lost the number", plain)
			}
			if tt.width < 80 && ansi.StringWidth(top) > tt.width {
				t.Errorf("top edge is %d cells wide, in %d", ansi.StringWidth(top), tt.width)
			}
			if tt.styled && !strings.Contains(top, glyph) {
				t.Errorf("top edge %q lost the glyph with its own style", top)
			}
			if links := uitest.Links(t, top); len(links) != 1 || links[0] != mod.link {
				t.Errorf("title links to %q, want %q once", links, mod.link)
			}
			if strings.Contains(top, "evil.test") {
				t.Errorf("a hostile title links to its own page: %q", top)
			}
		})
	}
}
