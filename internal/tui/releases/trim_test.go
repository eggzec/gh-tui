package releases

import (
	"context"
	"strconv"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The tag stays in the top edge of the frame and the name is cut first, at the
// widths of a wide and of a narrow frame.
func TestTrimTitle(t *testing.T) {
	m := New(context.Background(), &fakeService{}, repo, v3.ID, "", config.Default().Keys,
		WithNow(func() time.Time { return clock }), WithIcons(ui.NewIcons(config.IconsASCII)))
	if got := m.TrimTitle(); got.Lead != "" || got.Detail != "" || got.Name == "" {
		t.Errorf("before it loads: %+v, want a name only", got)
	}
	m.loaded = true
	m.rel = v3
	m.rel.Name = "Glow v3, the quiet release that updates Bubble Tea and Lip Gloss"
	if got := m.TrimTitle(); got.Name != "v3.0.0" || got.Detail != m.rel.Name || ansi.Strip(got.Lead) == "" {
		t.Errorf("loaded: %+v", got)
	}
	leads := map[string]bool{}
	for _, c := range []struct{ draft, pre bool }{{false, false}, {true, false}, {false, true}} {
		m.rel.Draft, m.rel.Prerelease = c.draft, c.pre
		leads[m.TrimTitle().Lead] = true
	}
	if len(leads) != 3 {
		t.Errorf("the states share glyphs: %d distinct", len(leads))
	}
	m.rel = core.Release{Tag: "v1", Name: "v1"}
	if got := m.TrimTitle(); got.Detail != "" {
		t.Errorf("a name equal to the tag is a detail: %q", got.Detail)
	}
	m.rel = v3
	m.rel.Name = "Glow v3, the quiet release that updates Bubble Tea and Lip Gloss"
	for _, w := range []int{80, 60} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			got := ansi.Strip(m.TrimTitle().Fit(w, lipgloss.NewStyle(), "…"))
			if ansi.StringWidth(got) > w {
				t.Errorf("width %d: title is %d cells", w, ansi.StringWidth(got))
			}
			golden.RequireEqual(t, got)
		})
	}
}
