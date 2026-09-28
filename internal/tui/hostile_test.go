package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// The frame draws whatever title a modal gives it, which may hold text
// from GitHub.
func TestFrameCleansHostileTitles(t *testing.T) {
	for _, w := range []int{40, 200} {
		m, _ := newTestApp(t)
		m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		mod := &fakeModal{title: termtexttest.Hostile}
		run(m, ui.OpenModal(mod))
		top, _, _ := strings.Cut(m.frame(mod), "\n")
		termtexttest.AssertClean(t, top, w)
	}
}

// The header names the default branch and the base, which are refs that
// may hold what git allows.
func TestHeaderCleansHostileRefs(t *testing.T) {
	h := termtexttest.Hostile
	info := func(context.Context, core.RepoRef) (core.Repo, error) { return core.Repo{DefaultBranch: h}, nil }
	for _, w := range []int{40, 200} {
		m, _ := newTestApp(t, WithRepoInfo(info))
		run(m, m.Init())
		m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		termtexttest.AssertClean(t, m.header, w)
		run(m, func() tea.Msg { return ui.BaseMsg{Repo: testRepo, Ref: h, Label: h} })
		termtexttest.AssertClean(t, m.header, w)
	}
}
