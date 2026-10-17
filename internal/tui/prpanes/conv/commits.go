package conv

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// commitsMsg carries the state of the checks of the commits of the pull
// request to the model that asked.
type commitsMsg struct {
	model, seq int
	// cached marks the list that was kept, and have that there was one.
	cached, have bool
	value        core.PullCommits
	err          error
}

// cachedCommitsCmd reads the kept commits, which costs no I/O.
func (m Model) cachedCommitsCmd() tea.Cmd {
	id, seq, s := m.id, m.seq, m.src
	return func() tea.Msg {
		msg := commitsMsg{model: id, seq: seq, cached: true}
		if s.CachedCommits != nil {
			msg.value, msg.have = s.CachedCommits()
		}
		return msg
	}
}

// readCommitsCmd reads the commits from the network, or does nothing when the
// source has no way to.
func (m Model) readCommitsCmd() tea.Cmd {
	id, seq, ctx, s := m.id, m.seq, m.ctx, m.src
	if s.Commits == nil {
		return nil
	}
	return func() tea.Msg {
		v, err := s.Commits(ctx)
		return commitsMsg{model: id, seq: seq, value: v, err: err}
	}
}

// commits takes in the commits a read brought. A list that was kept goes on to
// the read; a failed read leaves the icons as they were, and no error line.
func (m Model) commits(msg commitsMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case msg.cached:
		if msg.have {
			m.checks = checksOf(msg.value)
		}
		cmd = m.readCommitsCmd()
	case msg.err != nil:
		m.commitsErr = msg.err
	default:
		m.checks, m.commitsErr = checksOf(msg.value), nil
	}
	m.version++
	m.sync()
	return m, cmd
}

// checksOf maps the lower-case OID of each commit to the state of its checks.
func checksOf(p core.PullCommits) map[string]core.CheckState {
	checks := make(map[string]core.CheckState, len(p.Commits))
	for _, c := range p.Commits {
		checks[strings.ToLower(c.OID)] = c.Checks
	}
	return checks
}
