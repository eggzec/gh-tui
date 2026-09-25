package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// findingSection is a files section that finds its files in a fake modal.
type findingSection struct {
	*fakeSection
	modal *fakeModal
	finds int
	loads int
	// none makes it find nothing, as before a repository is selected.
	none bool
}

func (s *findingSection) FindFile() (ui.Modal, tea.Cmd) {
	s.finds++
	if s.none {
		return nil, nil
	}
	s.modal = &fakeModal{title: "Find file"}
	return s.modal, func() tea.Msg { s.loads++; return nil }
}

// newFindingApp returns an app on repo whose files find their files, on an
// 80x24 terminal.
func newFindingApp(t *testing.T, repo core.RepoRef) (*Model, *findingSection, []*fakeSection) {
	t.Helper()
	files := &findingSection{fakeSection: &fakeSection{title: "Files"}}
	fakes := []*fakeSection{files.fakeSection, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	layout := Layout{Files: files, Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	var opts []Option
	if repo != (core.RepoRef{}) {
		opts = append(opts, WithRepo(repo))
	}
	m := New(t.Context(), config.Default(), layout, opts...)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, files, fakes
}

func TestFindFileKeyOpensTheFinder(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{press("t"), {Code: 'p', Mod: tea.ModCtrl}} {
		t.Run(k.String(), func(t *testing.T) {
			m, files, _ := newFindingApp(t, testRepo)
			run(m, m.Init())
			if s := onScreen(m); !strings.Contains(s, "t find file") {
				t.Errorf("help lacks the find-file key:\n%s", s)
			}
			// The finder opens from any pane of the screen.
			run(m, m.key(press("2")))
			run(m, m.key(k))
			if m.topModal() != files.modal || files.loads != 1 {
				t.Fatalf("%s opened %v and loaded %d times", k, m.topModal(), files.loads)
			}
			if files.modal.width == 0 || !files.modal.themed {
				t.Error("the finder wasn't sized and themed before it was drawn")
			}
		})
	}
}

func TestFindFileKeyNeedsTheRepoScreen(t *testing.T) {
	m, files, fakes := newFindingApp(t, testRepo)
	run(m, m.key(press("n")))
	run(m, m.key(press("t")))
	if files.finds != 0 || m.topModal() != nil {
		t.Errorf("t found files %d times on the notifications", files.finds)
	}
	if !fakes[3].got(isKey("t")) {
		t.Error("t didn't reach the notifications")
	}

	m, files, _ = newFindingApp(t, core.RepoRef{})
	run(m, m.key(press("t")))
	if files.finds != 0 {
		t.Error("t found files without a repository")
	}

	m, files, _ = newFindingApp(t, testRepo)
	files.none = true
	run(m, m.key(press("t")))
	if files.finds != 1 || m.topModal() != nil {
		t.Errorf("a finder that finds nothing opened %v", m.topModal())
	}
}

func TestFindFileKeyAbsentWithoutAFinder(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.Init())
	if s := onScreen(m); strings.Contains(s, "find file") {
		t.Errorf("help offers the finder without one:\n%s", s)
	}
}
