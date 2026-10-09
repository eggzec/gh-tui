package issues

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The References step of the modal: the issues and pull requests linked to
// the issue, shown in place of its thread.

var (
	_ ui.Referencing = (*detailModal)(nil)
	_ ui.Hosted      = (*detailModal)(nil)
)

// KeyContext implements ui.Hosted: the links have keys of their own, but the
// modal is still the one of the issue.
func (m *detailModal) KeyContext() string { return ctxModal }

// WithReferences reads the issues and pull requests linked to an issue
// from svc, so that its modal can show them in a step. opts configure the
// step. Without it, the modal has no such step.
func WithReferences(svc refs.Service, opts ...refs.Option) Option {
	return func(s *Section) { s.refs, s.refsOpts = svc, opts }
}

// ShowReferences implements ui.Referencing. It reports whether the modal
// has the step to show, which it doesn't without a service for the links.
func (m *detailModal) ShowReferences() (tea.Cmd, bool) {
	if m.newRefs == nil {
		return nil, false
	}
	if m.refs != nil || m.ask != nil || m.composing != composeNone {
		return nil, true
	}
	return m.openRefs(), true
}

// openRefs shows the links in place of the thread.
func (m *detailModal) openRefs() tea.Cmd {
	if m.newRefs == nil || m.refs != nil {
		return nil
	}
	m.refs = m.newRefs()
	m.refs.SetTheme(m.theme)
	m.refs.SetSize(m.width, m.height)
	return m.refs.Init()
}

// closeRefs steps back from the links to the thread.
func (m *detailModal) closeRefs() {
	if m.refs == nil {
		return
	}
	m.refs.Close()
	m.refs = nil
}
