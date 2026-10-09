package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The References step of the modal: the issues and pull requests linked to
// the pull request, shown over the tab on view.

var (
	_ ui.Referencing = (*detailModal)(nil)
	_ ui.Hosted      = (*detailModal)(nil)
)

// KeyContext implements ui.Hosted: the links have keys of their own, but the
// modal is still the one of the pull request.
func (m *detailModal) KeyContext() string { return ctxModal }

// WithReferences reads the issues and pull requests linked to a pull
// request from svc, so that its modal can show them in a step. opts
// configure the step. Without it, the modal has no such step.
func WithReferences(svc refs.Service, opts ...refs.Option) Option {
	return func(s *Section) { s.refs, s.refsOpts = svc, opts }
}

// ShowReferences implements ui.Referencing. It reports whether the modal
// has the step to show, which it doesn't without a service for the links.
func (m *detailModal) ShowReferences() (tea.Cmd, bool) {
	if m.newRefs == nil {
		return nil, false
	}
	if m.refs != nil || m.ask != nil {
		return nil, true
	}
	return m.openRefs(), true
}

// openRefs shows the links over the tab on view, which pauses until they
// close.
func (m *detailModal) openRefs() tea.Cmd {
	if m.newRefs == nil || m.refs != nil {
		return nil
	}
	// A merge that waits for the detail would ask over the links.
	m.merging = nil
	if m.onChecks() {
		m.checks.Hide()
	}
	m.refs = m.newRefs()
	m.refs.SetTheme(m.theme)
	m.refs.SetSize(m.width, m.height)
	return m.refs.Init()
}

// closeRefs steps back from the links to the tab they were opened over.
func (m *detailModal) closeRefs() tea.Cmd {
	if m.refs == nil {
		return nil
	}
	m.refs.Close()
	m.refs = nil
	if m.onChecks() {
		return m.checks.Show()
	}
	return nil
}
