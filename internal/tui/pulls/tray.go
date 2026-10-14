package pulls

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// The review tray of the Files tab: the comments that the user drafted on
// lines, on ranges of lines and as replies to threads, which are kept on disk
// per pull request and shown in the diff under their lines. Nothing here is
// sent to GitHub.

// TrayStore keeps the drafts of pull requests, by key. Its calls do I/O.
type TrayStore interface {
	Load(key string) ([]tray.Draft, error)
	Save(key string, drafts []tray.Draft) error
}

// draftPrefix starts the ID of the diff block of a draft, so that it is not
// taken for a thread.
const draftPrefix = "draft:"

// composerRows is the height of the composer over the diff.
const composerRows = 8

// trayState is the drafts of the modal's pull request.
type trayState struct {
	store  TrayStore
	drafts []tray.Draft
	// from is where a range of lines was started, or nil.
	from *diff.Pos
	comp *composeState
	// list is the open list of drafts, or nil, and replacing is the ID of the
	// outdated draft that the next comment on a line places there.
	list      *trayList
	replacing string
	// readFailed is set when the file of the drafts could not be read, and may
	// still hold some: nothing is saved over it.
	readFailed bool
	// pending is the user's pending review on GitHub, as it was last read, with
	// no ID if they have none. Its comments are among the drafts, tagged
	// OnGitHub.
	pending core.PendingReview
	// asked is set once the modal has asked for the pending review.
	asked bool
	// reconcile is set when a review failed part way into the pending review:
	// the next read of it drops the drafts that it holds now.
	reconcile *reconcileState
}

// reconcileState is what a review that failed part way sent: the IDs of its
// drafts, and the IDs of the comments the pending review held before it, from
// the read that the review was planned on, which a read since may have
// replaced.
type reconcileState struct {
	sent, before []string
}

// composeState is the draft being written.
type composeState struct {
	c composer.Model
	d tray.Draft
}

// trayLoadedMsg carries the drafts read from disk to the modal that asked.
type trayLoadedMsg struct {
	owner  *detailModal
	drafts []tray.Draft
	err    error
}

// trayKey returns the key of the drafts of this pull request.
func (m *detailModal) trayKey() string { return tray.Key(m.repo.String(), m.number) }

// loadTray reads the drafts of the pull request from disk.
func (m *detailModal) loadTray() tea.Cmd {
	store, id := m.tray.store, m.trayKey()
	if store == nil {
		return nil
	}
	return func() tea.Msg {
		d, err := store.Load(id)
		return trayLoadedMsg{owner: m, drafts: d, err: err}
	}
}

// saveTray writes the drafts to disk, and warns if it can't. It writes at
// once, in Update, as the files are small: writes in commands could land out
// of order, and bring back a deleted draft or overwrite the one of the quit.
func (m *detailModal) saveTray() tea.Cmd {
	if err := m.writeTray(); err != nil {
		return ui.Notify(toast.Warning, "Couldn't save the drafts: "+err.Error())
	}
	return nil
}

// writeTray writes the drafts to disk, unless there is no store, or the file
// of the drafts could not be read for a reason that says nothing of its
// content: writing then could lose the drafts that are in it.
func (m *detailModal) writeTray() error {
	if m.tray.store == nil {
		return nil
	}
	if m.tray.readFailed {
		return errors.New("the saved drafts could not be read, so they are left as they are")
	}
	// The comments of the pending review are GitHub's, not the tray's.
	local := slices.DeleteFunc(slices.Clone(m.tray.drafts), func(d tray.Draft) bool { return d.OnGitHub })
	return m.tray.store.Save(m.trayKey(), local)
}

// Save implements ui.Saver: the text of the open composers is kept as drafts
// when the app quits.
func (m *detailModal) Save() error {
	return errors.Join(m.saveComment(), m.saveReview(), m.saveSummary())
}

// saveReview keeps the text of the open composer of a review comment as a
// draft of the tray.
func (m *detailModal) saveReview() error {
	c := m.tray.comp
	if c == nil || m.tray.store == nil || strings.TrimSpace(c.c.Value()) == "" {
		return nil
	}
	m.keepDraft(c.d, c.c.Value())
	return m.writeTray()
}

// update takes the messages of the tray, then the rest of the modal's.
func (m *detailModal) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case trayLoadedMsg:
		if msg.owner != m {
			return nil
		}
		if msg.err != nil {
			slog.WarnContext(m.ctx, "drafts not read", "span", "tui", "err", ui.ShortPath(msg.err.Error()))
			m.tray.readFailed = !errors.Is(msg.err, tray.ErrDamaged)
		} else {
			// Drafts made before the read came back stay.
			for i := range msg.drafts {
				id := msg.drafts[i].ID
				if !slices.ContainsFunc(m.tray.drafts, func(o tray.Draft) bool { return o.ID == id }) {
					m.tray.drafts = append(m.tray.drafts, msg.drafts[i])
				}
			}
		}
		return tea.Batch(m.settleDrafts(), m.showThreadChange())
	case pendingLoadedMsg:
		return m.receivePending(msg)
	case composer.SubmitMsg:
		// The comment's composer reports through the same messages.
		if c := m.tray.comp; c != nil && c.c.ID() == msg.ID {
			return m.endCompose(msg.ID, msg.Text)
		}
	case composer.CancelMsg:
		if c := m.tray.comp; c != nil && c.c.ID() == msg.ID {
			return m.endCompose(msg.ID, msg.Text)
		}
	}
	var fwd tea.Cmd
	if c := m.tray.comp; c != nil {
		if _, isKey := msg.(tea.KeyPressMsg); !isKey {
			c.c, fwd = c.c.Update(msg)
		}
	}
	return tea.Batch(fwd, m.updateMain(msg))
}

// endCompose closes the composer with id, keeping what was written as a draft.
func (m *detailModal) endCompose(id int64, text string) tea.Cmd {
	c := m.tray.comp
	if c == nil || c.c.ID() != id {
		return nil
	}
	m.tray.comp = nil
	if strings.TrimSpace(text) == "" {
		return nil
	}
	m.keepDraft(c.d, text)
	return tea.Batch(m.saveTray(), m.showThreadChange())
}

// keepDraft puts d with body among the drafts, in place of the one of its ID.
func (m *detailModal) keepDraft(d tray.Draft, body string) {
	d.Body = body
	if i := slices.IndexFunc(m.tray.drafts, func(o tray.Draft) bool { return o.ID == d.ID }); i >= 0 {
		m.tray.drafts[i] = d
		return
	}
	m.tray.drafts = append(m.tray.drafts, d)
}

// draftBlocks returns a block for each draft, under its line, or under its
// thread for a reply.
func (m *detailModal) draftBlocks() []diff.Block {
	blocks := make([]diff.Block, 0, len(m.tray.drafts))
	for i := range m.tray.drafts {
		d := m.tray.drafts[i]
		anchor := diff.Pos{Path: d.Path, Side: diffSide(d.Side), Line: d.Line}
		if d.ThreadID != "" {
			anchor = diff.Pos{Path: d.Path}
			if th, ok := m.threadByID(d.ThreadID); ok {
				anchor = threadAnchor(th)
			}
		}
		blocks = append(blocks, diff.Block{
			ID:     draftPrefix + d.ID,
			Anchor: anchor,
			Render: func(w int, _ bool) []string { return m.draftRows(d, w) },
		})
	}
	return blocks
}

// diffSide returns the side of the diff that a draft's side is.
func diffSide(side string) diff.Side {
	if side == tray.Left {
		return diff.OldSide
	}
	return diff.NewSide
}

// traySide returns the side of a draft that a side of the diff is.
func traySide(side diff.Side) string {
	if side == diff.OldSide {
		return tray.Left
	}
	return tray.Right
}

// draftRows renders draft d in w cells: a line that says what it is, and its
// text under it.
func (m *detailModal) draftRows(d tray.Draft, w int) []string {
	head := " " + m.theme.Accent.Render(m.icons.Dot) + " "
	where := ""
	switch {
	case d.ThreadID != "":
		where = " reply"
	case d.StartLine > 0 && d.StartLine != d.Line:
		where = fmt.Sprintf(" lines %d-%d", d.StartLine, d.Line)
	}
	if d.Outdated {
		where += " " + draftOutdated
	}
	if d.OnGitHub {
		where += " " + draftOnGitHub
	}
	lines := []string{ansi.Truncate(head+m.theme.Accent.Bold(true).Render("draft")+m.st.age.Render(where), w, m.icons.Ellipsis)}
	indent := strings.Repeat(" ", ansi.StringWidth(head))
	for raw := range strings.SplitSeq(d.Body, "\n") {
		for _, l := range ui.Wrap(ui.OneLine(raw), max(w-len(indent), 1)) {
			lines = append(lines, indent+m.st.title.Render(strings.TrimRight(l, " ")))
		}
	}
	return lines
}

// draftUnderCursor returns the draft whose row the cursor of the diff is on.
func (m *detailModal) draftUnderCursor() (tray.Draft, bool) {
	f := m.files
	if f == nil || f.focus != diffPane {
		return tray.Draft{}, false
	}
	b, ok := f.diff.CursorBlock()
	if !ok || !strings.HasPrefix(b.ID, draftPrefix) {
		return tray.Draft{}, false
	}
	id := strings.TrimPrefix(b.ID, draftPrefix)
	i := slices.IndexFunc(m.tray.drafts, func(d tray.Draft) bool { return d.ID == id })
	if i < 0 {
		return tray.Draft{}, false
	}
	return m.tray.drafts[i], true
}

// pressTray takes the keys of the tray on the diff: to comment on the line
// under the cursor, on a range that was started, or on the thread, or to edit
// the draft; to start a range; and to delete the draft. It reports whether it
// took the key.
func (m *detailModal) pressTray(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := m.keys
	switch {
	case keymap.Matches(msg, k.comment):
		return m.comment(), true
	case keymap.Matches(msg, k.rangeSel):
		return m.startRange(), true
	case keymap.Matches(msg, k.deleteDraft):
		d, ok := m.draftUnderCursor()
		if !ok {
			return nil, false
		}
		if d.OnGitHub {
			return ui.Notify(toast.Info, onGitHubText), true
		}
		m.tray.drafts = slices.DeleteFunc(m.tray.drafts, func(o tray.Draft) bool { return o.ID == d.ID })
		return tea.Batch(m.saveTray(), m.showThreadChange()), true
	}
	return nil, false
}

// startRange starts a range of lines at the line under the cursor, or ends
// the one that was started.
func (m *detailModal) startRange() tea.Cmd {
	t := &m.tray
	if t.from != nil {
		t.from = nil
		return ui.Notify(toast.Info, "Range cleared.")
	}
	pos, ok := m.files.diff.Position()
	if !ok {
		return ui.Notify(toast.Info, "Put the cursor on a line to start a range.")
	}
	t.from = &pos
	// A range is a new comment, so it ends a placement.
	t.replacing = ""
	return ui.Notify(toast.Info, "Range starts at line "+strconv.Itoa(pos.Line)+": move, then comment.")
}

// comment opens the composer on a new draft for the line or range under the
// cursor, on a reply to the thread under it, or on the draft under it.
func (m *detailModal) comment() tea.Cmd {
	if m.tray.comp != nil || !m.loaded {
		return nil
	}
	f := m.files
	if d, ok := m.draftUnderCursor(); ok {
		if d.OnGitHub {
			return ui.Notify(toast.Info, onGitHubText)
		}
		m.tray.replacing = ""
		return m.openComposer(d, "Edit draft")
	}
	head := f.head
	if head == "" {
		head = m.detail.HeadSHA
	}
	if head == "" {
		return ui.Notify(toast.Info, "The head commit is not known yet, so there is nothing to comment on.")
	}
	if i := slices.IndexFunc(m.tray.drafts, func(o tray.Draft) bool { return o.ID == m.tray.replacing }); i >= 0 {
		pos, ok := f.diff.Position()
		if th, onThread := m.threadUnderCursor(); !ok && onThread && th.Line > 0 {
			side := diff.NewSide
			if th.Side == core.SideLeft {
				side = diff.OldSide
			}
			pos, ok = diff.Pos{Path: th.Path, Side: side, Line: th.Line}, true
		}
		if !ok {
			return ui.Notify(toast.Info, "Put the cursor on a line to place the draft.")
		}
		// The outdated draft moves here, with its text, on the head, even
		// where a thread is: a re-placed comment is not a reply.
		d := m.tray.drafts[i]
		d.Commit, d.Path, d.Side, d.Line, d.StartLine, d.Outdated = head, pos.Path, traySide(pos.Side), pos.Line, 0, false
		m.noteText(&d)
		m.tray.replacing = ""
		return m.openComposer(d, "Place draft")
	}
	if th, ok := m.threadUnderCursor(); ok {
		side := tray.Right
		if th.Side == core.SideLeft {
			side = tray.Left
		}
		d := tray.Draft{ID: m.newDraftID(), Commit: head, Path: th.Path, ThreadID: th.ID, Side: side, Line: th.Line}
		return m.openComposer(d, "Draft reply")
	}
	pos, ok := f.diff.Position()
	if !ok {
		return ui.Notify(toast.Info, "Put the cursor on a line to comment.")
	}
	d := tray.Draft{ID: m.newDraftID(), Commit: head, Path: pos.Path, Side: traySide(pos.Side), Line: pos.Line}
	if from := m.tray.from; from != nil {
		if from.Path != pos.Path || from.Side != pos.Side {
			return ui.Notify(toast.Info, "A range stays in one file and on one side.")
		}
		d.StartLine, d.Line = min(from.Line, pos.Line), max(from.Line, pos.Line)
		m.tray.from = nil
		if d.StartLine == d.Line {
			d.StartLine = 0
		}
	}
	m.noteText(&d)
	return m.openComposer(d, "Draft comment")
}

// newDraftID returns an ID that no draft has.
func (m *detailModal) newDraftID() string {
	n := m.now().UnixNano()
	for {
		id := strconv.FormatInt(n, 36)
		if !slices.ContainsFunc(m.tray.drafts, func(d tray.Draft) bool { return d.ID == id }) {
			return id
		}
		n++
	}
}

// openComposer opens the composer on d, over the bottom of the diff.
func (m *detailModal) openComposer(d tray.Draft, title string) tea.Cmd {
	if d.Created.IsZero() {
		d.Created = m.now()
	}
	c := composer.New(m.trayKey()+"/"+d.ID,
		composer.WithTitle(title),
		composer.WithPlaceholder("Not sent: it waits in the review"),
		composer.WithKeyMap(composer.NewKeyMap(ui.Lookup(m.rawKeys, "composer"))),
		composer.WithSize(m.width, composerHeight(m.height)),
	)
	c.SetValue(d.Body)
	m.tray.comp = &composeState{c: c, d: d}
	return m.tray.comp.c.Focus()
}

// composerHeight returns the height of the composer in a modal of height h.
func composerHeight(h int) int { return max(min(composerRows, h/2), 3) }

// pressCompose gives a key to the open composer.
func (m *detailModal) pressCompose(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	m.tray.comp.c, cmd = m.tray.comp.c.Update(msg)
	return cmd
}

// trayBindings returns the keys of the tray for the help of the diff.
func (m *detailModal) trayBindings() []key.Binding {
	del := m.keys.deleteDraft
	_, onDraft := m.draftUnderCursor()
	del.SetEnabled(del.Enabled() && onDraft)
	return []key.Binding{m.keys.comment, m.keys.rangeSel, del}
}
