package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// WithOldEnterprise sets where the app hears of a GitHub Enterprise Server
// older than supported, by its version, which the app tells the user of
// once.
func WithOldEnterprise(old <-chan string) Option {
	return func(m *Model) { m.oldEnterprise = old }
}

// oldEnterpriseMsg reports the version of a GitHub Enterprise Server older
// than supported.
type oldEnterpriseMsg struct {
	version string
}

// listenOldEnterprise waits to hear of a server older than supported. It
// hears once: a session talks to one server.
func (m *Model) listenOldEnterprise() tea.Cmd {
	if m.oldEnterprise == nil {
		return nil
	}
	ch, ctx := m.oldEnterprise, m.ctx
	return func() tea.Msg {
		select {
		case v := <-ch:
			return oldEnterpriseMsg{version: v}
		case <-ctx.Done():
			return nil
		}
	}
}

// oldEnterpriseText tells that version, such as 3.12.4, isn't supported.
func oldEnterpriseText(version string) string {
	if parts := strings.SplitN(version, ".", 3); len(parts) == 3 {
		version = parts[0] + "." + parts[1]
	}
	// A warning has room for three short lines at 80 columns.
	return "GitHub Enterprise " + version + " isn't supported (" + core.MinEnterprise +
		"+ is); some things may not work."
}

// toldOldEnterprise tells the user that the server is older than
// supported.
func (m *Model) toldOldEnterprise(msg oldEnterpriseMsg) tea.Cmd {
	return m.toast.Push(toast.Warning, oldEnterpriseText(msg.version))
}
