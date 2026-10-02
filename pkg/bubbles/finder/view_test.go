package finder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// sized returns the sample with the size of each file as its detail.
func sized() Load {
	sizes := []string{"12K", "31K", "2.1K", "860B", "0.6K", "3.4K", "1.8K", "24K"}
	return func(context.Context) (Listing, error) {
		its := items(sample...)
		for i := range its {
			its[i].Detail = sizes[i]
		}
		return Listing{Items: its}, nil
	}
}

func TestView(t *testing.T) {
	deep := "staging/src/k8s.io/apimachinery/pkg/util/strategicpatch/testdata/swagger-merge-item.json"
	tests := []struct {
		name          string
		width, height int
		load          Load
		opts          []Option
		// skipInit leaves the paths loading.
		skipInit bool
		typed    string
		keys     []string
	}{
		{name: "all", width: 60, height: 12, load: sized()},
		{name: "matches", width: 60, height: 12, load: sized(), typed: "rend"},
		{name: "second selected", width: 60, height: 12, load: sized(), typed: "rend", keys: []string{"down"}},
		{name: "terms", width: 60, height: 8, load: sized(), typed: "rend test"},
		{name: "no match", width: 60, height: 6, load: sized(), typed: "zzz"},
		{name: "loading", width: 60, height: 6, load: sized(), skipInit: true},
		{name: "error", width: 60, height: 6, load: func(context.Context) (Listing, error) {
			return Listing{}, errors.New("403 rate limited")
		}},
		{name: "empty", width: 60, height: 6, load: loader()},
		{name: "note", width: 60, height: 6, load: func(context.Context) (Listing, error) {
			return Listing{Items: items(sample...), Note: "listing cut short"}, nil
		}},
		{name: "cut from the left", width: 40, height: 6, load: loader(deep, "a/"+strings.Repeat("x", 60)+".go"), typed: "swag"},
		{name: "narrow drops detail", width: 26, height: 8, load: sized(), typed: "rend"},
		{name: "scrolled", width: 40, height: 5, load: sized(), keys: []string{"down", "down", "down", "down"}},
		{name: "light", width: 60, height: 8, load: sized(), typed: "rend", opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "one row", width: 40, height: 1, load: sized(), typed: "rend"},
		{name: "icons", width: 60, height: 8, load: sized(), typed: "rend", opts: []Option{WithIcons(extIcons)}},
		{name: "icons narrow", width: 26, height: 8, load: sized(), typed: "rend", opts: []Option{WithIcons(extIcons)}},
		{name: "icons cut from the left", width: 40, height: 6, load: loader(deep), typed: "swag", opts: []Option{WithIcons(extIcons)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.load, append([]Option{WithSize(tt.width, tt.height)}, tt.opts...)...)
			m.Focus()
			if !tt.skipInit {
				m = run(t, m, m.Init())
			}
			m = typed(t, m, tt.typed)
			m = keys(t, m, tt.keys...)
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// TestViewMarksMatches checks the characters that the rows mark, without
// the colors of the golden files.
func TestViewMarksMatches(t *testing.T) {
	m := open(t, 60, 6, sample, WithStyles(Styles{Match: DefaultStyles(true).Match.Reverse(true), CursorGlyph: "▌"}))
	m = typed(t, m, "crt")
	row := strings.Split(m.View(), "\n")[1]
	on := newPair(m.styles.Match).on
	var marked strings.Builder
	for _, part := range strings.Split(row, on)[1:] {
		marked.WriteString(ansi.Strip(part)[:1])
	}
	if got := marked.String(); got != "crt" {
		t.Errorf("marked %q in %q, want crt", got, ansi.Strip(row))
	}
}

// extIcons marks Go files with a g, and other files with a dot.
func extIcons(it Item) string {
	if strings.HasSuffix(it.Path, ".go") {
		return "g"
	}
	return "·"
}

// TestViewMarksMatchesAfterIcons checks that the icon moves the marks with
// the path, so they stay on the characters that match.
func TestViewMarksMatchesAfterIcons(t *testing.T) {
	for _, width := range []int{60, 27} {
		m := open(t, width, 6, sample, WithIcons(extIcons),
			WithStyles(Styles{Match: DefaultStyles(true).Match.Reverse(true), CursorGlyph: "▌"}))
		m = typed(t, m, "crt")
		row := strings.Split(m.View(), "\n")[1]
		plain := ansi.Strip(row)
		if !strings.HasPrefix(plain, "▌ g ") {
			t.Fatalf("at %d columns the row %q should start with the gutter and the icon", width, plain)
		}
		on := newPair(m.styles.Match).on
		var marked strings.Builder
		for _, part := range strings.Split(row, on)[1:] {
			marked.WriteString(ansi.Strip(part)[:1])
		}
		if got := marked.String(); got != "crt" {
			t.Errorf("at %d columns marked %q in %q, want crt", width, got, plain)
		}
	}
}

func TestViewIconsFitAnySize(t *testing.T) {
	m := open(t, 40, 6, sample, WithIcons(extIcons))
	m = typed(t, m, "rend")
	for _, size := range [][2]int{{2, 3}, {3, 3}, {4, 3}, {5, 4}, {12, 4}, {80, 10}} {
		m.SetSize(size[0], size[1])
		assertFits(t, m.View(), size[0], size[1])
	}
}

func TestSetIcons(t *testing.T) {
	m := open(t, 40, 6, sample)
	m.SetIcons(extIcons)
	if row := ansi.Strip(strings.Split(m.View(), "\n")[1]); !strings.HasPrefix(row, "▌ g cursed_renderer.go") {
		t.Fatalf("SetIcons should draw the icons at once, got %q", row)
	}
	m.SetIcons(nil)
	if row := ansi.Strip(strings.Split(m.View(), "\n")[1]); !strings.HasPrefix(row, "▌ cursed_renderer.go") {
		t.Fatalf("SetIcons(nil) should drop the icons, got %q", row)
	}
}

func TestViewZeroSize(t *testing.T) {
	m := open(t, 0, 0, sample)
	if m.View() != "" {
		t.Error("a finder of no size renders something")
	}
}

// The prompt, the cursor, the separators of the status line and the
// ellipsis of cut paths are the glyphs of the styles, so a view with ASCII
// glyphs draws ASCII alone.
func TestViewGlyphs(t *testing.T) {
	st := DefaultStyles(true)
	st.PromptGlyph, st.CursorGlyph, st.Separator, st.Ellipsis = ">", ">", " - ", "..."
	note := func(context.Context) (Listing, error) {
		return Listing{Items: items(sample...), Note: "truncated"}, nil
	}
	m := New(note, WithSize(24, 6), WithStyles(st))
	m.Focus()
	m = typed(t, run(t, m, m.Init()), "rend")
	v := ansi.Strip(m.View())
	for _, want := range []string{"> rend", "> renderer.go", "  ...es/render/render.go", "8 files - 5 matches -..."} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
		t.Errorf("view has glyphs beyond ASCII:\n%s", v)
	}
}
