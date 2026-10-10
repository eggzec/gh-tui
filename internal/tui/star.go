package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Starrer stars and unstars repositories for the viewer. The operations it
// returns already show the change, and send it when done.
type Starrer interface {
	// CachedGet returns the repository if it is in memory, without I/O,
	// which tells whether the viewer has starred it.
	CachedGet(ref core.RepoRef) (core.Repo, bool)
	Star(ref core.RepoRef) *optimistic.Op
	Unstar(ref core.RepoRef) *optimistic.Op
}

// WithStarrer sets what stars the repository of the repository screen, for
// the star command, which toggles it. Without it, the screen has nothing
// to star.
func WithStarrer(s Starrer) Option {
	return func(m *Model) { m.starrer = s }
}

// starFrom names the changes that the star command sends, in the log.
const starFrom = "Repository"

// starDoneMsg reports that a star or unstar finished.
type starDoneMsg struct {
	repo    core.RepoRef
	starred bool
	done    ui.DoneMsg
}

// canStar reports whether the star command does something: only the
// repository screen has a repository to star.
func (m *Model) canStar() bool {
	return m.starrer != nil && m.screen == repoScreen && m.repo != (core.RepoRef{})
}

// star asks whether to star the repository of the screen, or to unstar it
// if the viewer has starred it, and does so on a yes, as the key of
// repo.star or the star command does.
func (m *Model) star() tea.Cmd {
	repo := m.repo
	now := func() (c ui.Confirm, ok bool, refusal tea.Cmd) {
		r, known := m.starrer.CachedGet(repo)
		if !known {
			// Which of the two it is isn't known until the repository
			// is read, and a question for the wrong one would undo a star.
			return c, false, ui.Notify(toast.Info, "Still reading "+repo.String()+", so star it again in a moment.")
		}
		verb, send := "Star", m.starrer.Star
		if r.Starred {
			verb, send = "Unstar", m.starrer.Unstar
		}
		starred := !r.Starred
		return ui.Confirm{
			Question: verb + " " + repo.String() + "?",
			Run: func() tea.Cmd {
				done := ui.Do(m.ctx, starFrom, send(repo), strings.ToLower(verb)+" "+repo.String())
				return func() tea.Msg {
					d, _ := done().(ui.DoneMsg)
					return starDoneMsg{repo: repo, starred: starred, done: d}
				}
			},
		}, true, nil
	}
	c, ok, refusal := now()
	if !ok {
		return refusal
	}
	c = ui.Recheck(c.Question, repo.String(), now)
	return ui.OpenModal(ui.NewConfirmModal(c, ui.NewConfirmKeys(m.cfg.Keys), m.icons))
}

// starDone shows how a star or unstar went: a toast for the error that
// rolled it back, or one that says it is done.
func (m *Model) starDone(msg starDoneMsg) tea.Cmd {
	if msg.done.Err != nil {
		return m.fail(msg.done.What, msg.done.Err)
	}
	text := "Starred " + msg.repo.String() + "."
	if !msg.starred {
		text = "Unstarred " + msg.repo.String() + "."
	}
	return m.toast.Push(toast.Success, text)
}
