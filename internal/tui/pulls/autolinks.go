package pulls

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// WithAutolinks links the references (#N, @login, hashes) that links
// confirmed in the description of the modal.
func WithAutolinks(links *ui.Autolinks) Option {
	return func(s *Section) { s.autolinks = links }
}

// linksAsk returns the command that asks about the references of the
// description drawn since the last update, or nil.
func (m *panesModal) linksAsk() tea.Cmd {
	body := m.desc.ask
	m.desc.ask = ""
	if body == "" {
		return nil
	}
	return m.links().Verify(m.ctx, m.repo.String(), body)
}

// links returns the autolinks of the section, nil if there is none.
func (m *panesModal) links() *ui.Autolinks {
	if m.list == nil {
		return nil
	}
	return m.list.autolinks
}

// linksSynced draws the description again if links learned answers.
func (m *panesModal) linksSynced(msg ui.SyncMsg) {
	if m.links().Synced(msg) {
		m.desc.links++
		m.desc.version++
	}
}

// askLinks notes that body was drawn, so that its references are asked about.
func (d *descPane) askLinks(body string) { d.ask = strings.TrimSpace(body) }
