package pulls

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// convEditor is what the Conversation changes of a pull request: it resolves
// and opens again a review thread, and deletes a comment of the timeline or
// one of a review thread.
type convEditor interface {
	// ResolveThread makes the thread resolved, or open again when resolved is
	// false.
	ResolveThread(repo core.RepoRef, number int, id string, resolved bool) *optimistic.Op
	DeleteComment(repo core.RepoRef, number int, id string) *optimistic.Op
	DeleteReviewComment(repo core.RepoRef, number int, threadID, id string) *optimistic.Op
}

// convRecacheMsg tells the modal that a change of the conversation was sent or
// rolled back, so that the conversation reads its cache again.
type convRecacheMsg struct{ id int64 }

// recacheConv shows what the cache holds now in the conversation, which the
// conversation doesn't learn of by itself.
func (m *panesModal) recacheConv() tea.Cmd {
	c := &m.conv
	if !c.started {
		return nil
	}
	var cmd tea.Cmd
	c.model, cmd = c.model.Recache()
	m.refreshConv()
	return cmd
}

// sendConvOp sends a change of the conversation. Once it went through, it says
// done; whichever way it ended, the conversation reads the cache again, which
// shows a rollback, and the pull request is read again.
func (m *panesModal) sendConvOp(op ui.Op, what, done string) tea.Cmd {
	about := core.Target{Repo: m.repo, Number: m.number}.String()
	send := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(about, op), what)
	id, revalidate := m.id, m.revalidate()
	return func() tea.Msg {
		msg := send()
		cmds := tea.BatchMsg{
			func() tea.Msg { return msg },
			func() tea.Msg { return convRecacheMsg{id: id} },
			revalidate,
		}
		if d, ok := msg.(ui.DoneMsg); ok && d.Err == nil {
			cmds = append(cmds, ui.Notify(toast.Info, done))
		}
		return cmds
	}
}

// resolveConv asks to resolve the thread under the cursor, or to open it again
// when it is resolved.
func (m *panesModal) resolveConv() tea.Cmd {
	s, ok := m.conv.model.Current()
	if !ok || s.Thread == nil || (s.Kind != conv.KindThread && s.Kind != conv.KindReply) {
		return ui.Notify(toast.Info, "select a thread (or a reply in it) to resolve")
	}
	t := s.Thread
	id, where, resolved := t.ID, threadWhere(t), !t.Resolved
	verb, done := "Resolve", "thread resolved"
	if !resolved {
		verb, done = "Reopen", "thread reopened"
	}
	switch {
	case resolved && !t.ViewerCanResolve:
		return ui.Notify(toast.Info, "you can't resolve this thread")
	case !resolved && !t.ViewerCanUnresolve:
		return ui.Notify(toast.Info, "you can't reopen this thread")
	}
	m.ask(confirm.Ask{Question: verb + " thread " + where + "?", Run: func() tea.Cmd {
		op := m.svc.ResolveThread(m.repo, m.number, id, resolved)
		send := m.sendConvOp(op, "update thread "+where, done)
		return tea.Batch(m.recacheConv(), send)
	}})
	return nil
}

// threadWhere says where a thread is: its file and line, or its file alone
// when it is on the whole file.
func threadWhere(t *core.Thread) string {
	line := t.Line
	if line == 0 {
		line = t.OriginalLine
	}
	if t.File || line == 0 {
		return t.Path
	}
	return t.Path + ":" + strconv.Itoa(line)
}

// deleteConv asks to delete the comment or reply of the viewer under the
// cursor.
func (m *panesModal) deleteConv() tea.Cmd {
	s, ok := m.conv.model.Current()
	if !ok {
		return ui.Notify(toast.Info, "select your comment or reply to delete")
	}
	viewer := m.conv.viewer
	own := func(login string) bool { return viewer != "" && login == viewer }
	var question string
	var run func() tea.Cmd
	switch {
	case s.Kind == conv.KindReview:
		return ui.Notify(toast.Info, "a submitted review can't be deleted")
	case (s.Kind == conv.KindComment || s.Kind == conv.KindRunComment) && s.Item != nil:
		if !own(s.Item.Actor.Login) {
			return ui.Notify(toast.Info, "you can only delete your own comments")
		}
		id := s.Item.ID
		question = "Delete your comment?"
		run = func() tea.Cmd { return m.deleteWith(m.svc.DeleteComment(m.repo, m.number, id)) }
	case (s.Kind == conv.KindThread || s.Kind == conv.KindReply) && s.Thread != nil && s.Comment != nil:
		if !own(s.Comment.Author.Login) {
			return ui.Notify(toast.Info, "you can only delete your own comments")
		}
		threadID, id := s.Thread.ID, s.Comment.ID
		question = "Delete your reply?"
		if s.Kind == conv.KindThread {
			question = "Delete your comment?"
			if len(s.Thread.Comments) > 1 || s.Thread.TotalComments > 1 {
				question += " Its replies stay."
			}
		}
		run = func() tea.Cmd { return m.deleteWith(m.svc.DeleteReviewComment(m.repo, m.number, threadID, id)) }
	default:
		return ui.Notify(toast.Info, "select your comment or reply to delete")
	}
	m.ask(confirm.Ask{Question: question, Run: run})
	return nil
}

// deleteWith sends the deletion op made, after the conversation shows it gone.
func (m *panesModal) deleteWith(op *optimistic.Op) tea.Cmd {
	send := m.sendConvOp(op, "delete comment", "comment deleted")
	return tea.Batch(m.recacheConv(), send)
}
