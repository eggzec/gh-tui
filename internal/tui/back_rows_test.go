package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// shapeModal ranks as a filter does, over the content modals and the details.
type shapeModal struct{ backModal }

// anywhereModal ranks as the search and the notifications do.
type anywhereModal struct{ backModal }

// The embedded backModal would close itself, not the modal that holds it.
func (s *shapeModal) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEscape {
		return ui.CloseModal(s)
	}
	return nil
}

func (a *anywhereModal) Update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyEscape {
		return ui.CloseModal(a)
	}
	return nil
}

func init() {
	modalPriorities[reflect.TypeFor[*shapeModal]()] = modals.PriorityShape
	modalPriorities[reflect.TypeFor[*anywhereModal]()] = modals.PriorityAnywhere
}

// TestModalRowsReturnByEsc checks the wiring of the ranks: a modal of a
// higher rank, or one that opens anywhere, covers the open one and esc
// returns to it; a lower rank is refused and changes nothing, and the one on
// top is not opened twice.
func TestModalRowsReturnByEsc(t *testing.T) {
	t.Parallel()
	esc := press("esc")
	t.Run("shape over content", func(t *testing.T) {
		t.Parallel()
		m, _ := newTestApp(t)
		host := &backModal{}
		host.title = "host"
		f := &shapeModal{}
		f.title = "filter"
		run(m, ui.OpenModalOver(host, nil))
		run(m, ui.OpenModalOver(f, host))
		if m.modals.Top() != f {
			t.Fatalf("top is %v, want the filter", m.modals.Top())
		}
		run(m, m.key(esc))
		if m.modals.Top() != host || host.discarded != 0 {
			t.Errorf("after esc: top %v, host discarded %d, want the host kept", m.modals.Top(), host.discarded)
		}
	})
	t.Run("anywhere over a shape", func(t *testing.T) {
		t.Parallel()
		m, _ := newTestApp(t)
		f := &shapeModal{}
		f.title = "filter"
		s := &anywhereModal{}
		s.title = "search"
		run(m, ui.OpenModalOver(f, nil))
		run(m, ui.OpenModalOver(s, f))
		if m.modals.Top() != s {
			t.Fatalf("top is %v, want the search", m.modals.Top())
		}
		run(m, m.key(esc))
		if m.modals.Top() != f {
			t.Errorf("after esc: top %v, want the filter", m.modals.Top())
		}
	})
	t.Run("a detail over a shape is refused", func(t *testing.T) {
		t.Parallel()
		m, _ := newTestApp(t)
		f := &shapeModal{}
		f.title = "filter"
		p := &previewModal{}
		p.title = "preview"
		run(m, ui.OpenModalOver(f, nil))
		run(m, ui.OpenModalOver(p, f))
		if m.modals.Top() != f {
			t.Errorf("top is %v, want the filter kept", m.modals.Top())
		}
	})
	t.Run("the same modal twice", func(t *testing.T) {
		t.Parallel()
		m, _ := newTestApp(t)
		a := &anywhereModal{}
		a.title = "search"
		b := &anywhereModal{}
		b.title = "search"
		run(m, ui.OpenModalOver(a, nil))
		run(m, ui.OpenModalOver(b, a))
		if m.modals.Top() != a {
			t.Errorf("top is %v, want the first search kept", m.modals.Top())
		}
	})
}
