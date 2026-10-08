package files

import (
	"strings"
	"testing"
	"unicode"

	"charm.land/bubbles/v2/key"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		section       func(t *testing.T, width, height int) *Section
	}{
		{"no repo at 30 columns", 30, 8, func(t *testing.T, w, h int) *Section {
			t.Helper()
			return newSection(t, sampleFake(), w, h)
		}},
		{"no repo at 80 columns", 80, 6, func(t *testing.T, w, h int) *Section {
			t.Helper()
			return newSection(t, sampleFake(), w, h)
		}},
		{"loaded", 30, 10, func(t *testing.T, w, h int) *Section {
			t.Helper()
			return loaded(t, sampleFake(), w, h)
		}},
		{"nested", 30, 10, func(t *testing.T, w, h int) *Section {
			t.Helper()
			s := loaded(t, sampleFake(), w, h)
			keys(s, "l", "down", "l", "down")
			return s
		}},
		{"blurred", 30, 4, func(t *testing.T, w, h int) *Section {
			t.Helper()
			s := loaded(t, sampleFake(), w, h)
			s.Blur()
			return s
		}},
		{"empty repo", 30, 3, func(t *testing.T, w, h int) *Section {
			t.Helper()
			f := newFake()
			f.addTree(ghTUI, "")
			return loaded(t, f, w, h)
		}},
		{"error", 40, 3, func(t *testing.T, w, h int) *Section {
			t.Helper()
			f := newFake()
			return loaded(t, f, w, h)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.section(t, tt.width, tt.height).View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewPreview(t *testing.T) {
	tests := []struct {
		name string
		row  int
	}{
		{"text", rowAgents},
		{"too large", rowReadme},
		{"symlink", rowLink},
		{"error", rowGitignore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := openRow(t, sampleFake(), tt.row)
			v := h.top().View()
			assertFits(t, v, h.width, h.height)
			golden.RequireEqual(t, v)
		})
	}
}

// TestViewWithoutFileIcons draws the rows as before in the icon sets
// without file icons.
func TestViewWithoutFileIcons(t *testing.T) {
	for set, first := range map[string]string{config.IconsUnicode: "▌ ▸ cmd", config.IconsASCII: "> + cmd"} {
		s := loaded(t, sampleFake(), 30, 4, WithIcons(ui.NewIcons(set)))
		lines := strings.Split(ansi.Strip(s.View()), "\n")
		if strings.TrimRight(lines[0], " ") != first || lines[2] != "    vendor-lib                " {
			t.Errorf("%s: rows %q, want no icons", set, lines)
		}
	}
}

func TestViewIcons(t *testing.T) {
	s := loaded(t, sampleFake(), 30, 10)
	ic := ui.NewIcons(config.IconsNerd)
	th := testTheme()
	v := s.View()
	for _, e := range sampleFake().trees[treeKey(ghTUI, "")].Entries {
		want := th.FileIcon(ic.Entry(e, false)).Render(ic.Entry(e, false).Glyph) + " "
		if !strings.Contains(v, want) {
			t.Errorf("no icon %q before %s in\n%s", want, e.Name, v)
		}
	}
	// A theme renders the icons again, in its colors.
	p, err := config.Default().Palette(false)
	if err != nil {
		t.Fatal(err)
	}
	light := ui.NewTheme(p, false)
	s.SetTheme(light)
	link := ic.Entry(core.TreeEntry{Type: core.EntryBlob, Mode: core.ModeSymlink}, false)
	if want := light.Muted.Render(link.Glyph); !strings.Contains(s.View(), want) {
		t.Errorf("after a light theme, no link icon %q in\n%s", want, s.View())
	}
}

func TestViewIconsFitNarrowWidths(t *testing.T) {
	s := loaded(t, sampleFake(), 30, 10)
	keys(s, "l", "down", "l", "down")
	for w := range 32 {
		s.SetSize(w, 10)
		if w == 0 {
			continue
		}
		assertFits(t, s.View(), w, 10)
	}
}

// With the ASCII icons the tree draws ASCII alone, and so do the notes of
// a file the preview doesn't show.
func TestViewASCII(t *testing.T) {
	ic := ui.NewIcons(config.IconsASCII)
	s := loaded(t, sampleFake(), 40, 10, WithIcons(ic))
	open := key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open"))
	for _, v := range []string{ansi.Strip(s.View()), browserHint(open, ic)} {
		if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("%q isn't ASCII", v)
		}
	}
}
