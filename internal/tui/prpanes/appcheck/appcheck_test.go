package appcheck

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func options(t *testing.T, dark bool) Options {
	t.Helper()
	p, err := config.Default().Palette(dark)
	if err != nil {
		t.Fatal(err)
	}
	th, ic := ui.NewTheme(p, dark), ui.NewIcons(config.IconsUnicode)
	md := markdown.New(th.Thread(ic).Markdown)
	md.SetGlyphs(ic.Markdown)
	return Options{Styles: NewStyles(th, ic), MD: md}
}

func data() Data {
	return Data{
		Check: core.CommitCheck{
			Check: core.Check{
				ID: 21, Name: "coverage", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
				Title: "Coverage fell by 2%", Summary: "The **patch** is covered at 61%.\n\n- `a.go` 40%\n- `b.go` 82%",
				Annotations: 3, DetailsURL: "https://example.com/coverage",
			},
			SuiteID: 7, App: &core.CheckApp{Name: "Codecov", Slug: "codecov"},
		},
		Notes: []core.Annotation{
			{Path: "a.go", StartLine: 12, EndLine: 14, Level: core.AnnotationFailure, Title: "Uncovered", Message: "These lines have no test."},
			{Path: "internal/b/b.go", StartLine: 3, EndLine: 3, Level: core.AnnotationWarning, Message: "Partially covered."},
			{Path: "c.go", StartLine: 90, EndLine: 90, Level: core.AnnotationNotice, Message: "Covered now."},
		},
		State: NotesLoaded,
	}
}

func TestGolden(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		dark bool
		w, h int
	}{{"dark_80", true, 80, 20}, {"light_80", false, 80, 20}, {"dark_40", true, 40, 16}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := New(options(t, tt.dark))
			m.SetData(data())
			m.SetSize(tt.w, tt.h)
			golden.RequireEqual(t, m.View())
		})
	}
}

func TestFits(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 40, 24} {
		m := New(options(t, true))
		m.SetData(data())
		m.SetSize(w, 12)
		termtexttest.AssertClean(t, m.View(), w)
		if n := strings.Count(m.View(), "\n") + 1; n != 12 {
			t.Errorf("width %d: view is %d lines, want 12", w, n)
		}
	}
}

func TestSelectedFollowsTheCursor(t *testing.T) {
	t.Parallel()
	m := New(options(t, true))
	m.SetData(data())
	m.SetSize(80, 20)
	if a, ok := m.Selected(); !ok || a.Path != "a.go" {
		t.Fatalf("Selected = %+v, %v; want a.go first", a, ok)
	}
	m.Move(Down)
	if a, _ := m.Selected(); a.Path != "internal/b/b.go" {
		t.Errorf("after down Selected = %q, want internal/b/b.go", a.Path)
	}
	m.Move(Bottom)
	if a, _ := m.Selected(); a.Path != "c.go" {
		t.Errorf("at the bottom Selected = %q, want c.go", a.Path)
	}
}

func TestNoAnnotationsSelectNothing(t *testing.T) {
	t.Parallel()
	d := data()
	d.Notes, d.Check.Annotations = nil, 0
	m := New(options(t, true))
	m.SetData(d)
	m.SetSize(80, 10)
	if a, ok := m.Selected(); ok {
		t.Errorf("Selected = %+v on a run with none", a)
	}
}

func BenchmarkView(b *testing.B) {
	p, _ := config.Default().Palette(true)
	th, ic := ui.NewTheme(p, true), ui.NewIcons(config.IconsUnicode)
	md := markdown.New(th.Thread(ic).Markdown)
	m := New(Options{Styles: NewStyles(th, ic), MD: md})
	m.SetData(data())
	m.SetSize(80, 20)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
