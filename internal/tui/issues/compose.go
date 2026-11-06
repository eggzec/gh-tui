package issues

import (
	"errors"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
)

// composerOverlay is the composer of a comment, over the panes.
type composerOverlay = prpanes.ComposerOverlay

// editing is the comment an open composer rewrites: its ID and its body
// before. A composer of a new comment has none.
type editing struct{ id, from string }

// openComposer opens the composer over the panes, for a new comment or, with
// edit set, for the rewrite of one, if the viewer may comment.
func (m *detailModal) openComposer(edit *editing) tea.Cmd {
	if !m.loaded {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.issue); refused {
		return cmd
	}
	num := "#" + strconv.Itoa(m.number)
	opts := []composer.Option{
		composer.WithKeyMap(m.keys.composer),
		composer.WithStyles(m.theme.Prompt(m.icons)),
		composer.WithSize(m.width, max(m.height/2, 1)),
		// The frame's title names the action, so the row inside names what
		// is commented on; left out, it would be a blank row.
		composer.WithTitle(m.issue.Title),
	}
	var key, head string
	if edit == nil {
		key = "comment/" + m.repo.String() + "/" + strconv.Itoa(m.number)
		head = "Comment on " + num
		opts = append(opts,
			composer.WithPlaceholder("Write a comment in markdown."),
			composer.WithDrafts(m.drafts))
	} else {
		// The text starts as the comment, so what is kept as a draft would be
		// the comment itself: an edit keeps none.
		key = "edit/" + m.repo.String() + "/" + strconv.Itoa(m.number) + "/" + edit.id
		head = "Edit your comment on " + num
		opts = append(opts, composer.WithDrafts(nil))
	}
	c := composer.New(key, opts...)
	if edit != nil {
		c.SetValue(edit.from)
		m.edit = edit
	} else {
		m.edit = nil
	}
	o := &composerOverlay{Comp: c, Head: head, Busy: func() bool { return m.ask != nil }}
	o.Submit = m.submitComposer
	m.over = o
	return tea.Batch(o.Comp.Init(), o.Comp.Focus())
}

// composerOpen returns the composer over the panes, or nil.
func (m *detailModal) composerOpen() *composerOverlay {
	if o, ok := m.over.(*composerOverlay); ok && !o.Closed() {
		return o
	}
	return nil
}

// HasDraft reports whether text is being written that the modal would lose
// if it closed: a comment that is not blank, or an edit that changed.
func (m *detailModal) HasDraft() bool {
	if e, ok := m.over.(*prpanes.DescEdit); ok && e.Changed(m.issue.Title, m.issue.Body) {
		return true
	}
	o := m.composerOpen()
	if o == nil {
		return false
	}
	if m.edit != nil {
		return !sameText(o.Comp.Value(), m.edit.from)
	}
	return strings.TrimSpace(o.Comp.Value()) != ""
}

// Save keeps the text of the open comment as its draft, so that a modal the
// app closes on its own account, or quits with, loses nothing. It implements
// ui.Saver.
func (m *detailModal) Save() error {
	o := m.composerOpen()
	if o == nil {
		return nil
	}
	return errors.Join(o.Comp.Save())
}

// closeComposer closes the composer and gives the focus back to the thread.
func (m *detailModal) closeComposer(o *composerOverlay) {
	o.Close()
	if m.over == reactOverlay(o) {
		m.over = nil
	}
	m.edit = nil
}

// submitComposer asks to send what the composer holds. A second submit, such
// as a repeated key, arrives while the first one asks.
func (m *detailModal) submitComposer(o *composerOverlay) tea.Cmd {
	if m.ask != nil {
		return nil
	}
	if m.edit != nil {
		return m.submitEdit(o, *m.edit)
	}
	return m.submitComment(o)
}

// submitComment asks to post what the composer holds as a comment on the
// issue. An empty comment asks nothing, and the composer stays open.
func (m *detailModal) submitComment(o *composerOverlay) tea.Cmd {
	body := strings.TrimSpace(o.Comp.Value())
	c, ok, refusal := m.comment(o, body)
	if !ok {
		return refusal
	}
	ask := ui.Recheck(c.Question, "#"+strconv.Itoa(m.number), func() (confirm.Ask, bool, tea.Cmd) {
		if m.over != reactOverlay(o) || strings.TrimSpace(o.Comp.Value()) != body {
			return confirm.Ask{}, false, nil
		}
		return m.comment(o, body)
	})
	return m.asks(ask)
}

// comment returns posting body as a comment on the issue as a question,
// or ok unset when there is nothing to post or the viewer may not, which
// refusal then says. Its run closes the composer and drops its draft, and the
// service shows the comment in the cache at once, as sending, so the thread
// reloads to show it.
func (m *detailModal) comment(o *composerOverlay, body string) (c confirm.Ask, ok bool, refusal tea.Cmd) {
	if body == "" || !m.loaded {
		return confirm.Ask{}, false, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.issue); refused {
		return confirm.Ask{}, false, cmd
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return confirm.Ask{
		Question: "Post this comment on " + num + "?",
		Run: func() tea.Cmd {
			m.closeComposer(o)
			op := m.svc.Comment(m.repo, n, body)
			m.addPending(body)
			return tea.Batch(o.Comp.Discard(), m.reload(), m.buildConv(), m.changed(),
				m.send(op, "comment on "+num))
		},
	}, true, nil
}

// addPending shows body as a comment being sent, which the conversation
// draws last until GitHub answers.
func (m *detailModal) addPending(body string) {
	who := m.viewer
	if who == "" {
		who = "you"
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent++
	m.pending = append(m.pending, core.TimelineItem{
		Kind: core.TimelineComment, ID: "pending:" + strconv.Itoa(m.sent),
		Actor: core.User{Login: who}, At: now, UpdatedAt: now, Body: body,
	})
}
