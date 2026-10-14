package pulls

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// saveTray writes the drafts to disk, and warns if it can't.
func (m *detailModal) saveTray() tea.Cmd {
	store, id, drafts := m.tray.store, m.trayKey(), slices.Clone(m.tray.drafts)
	if store == nil {
		return nil
	}
	return func() tea.Msg {
		if err := store.Save(id, drafts); err != nil {
			return ui.Notify(toast.Warning, "Couldn't save the drafts: "+err.Error())()
		}
		return nil
	}
}

// Save implements ui.Saver: the text of the open composer is kept as a draft
// when the app quits.
func (m *detailModal) Save() error {
	c := m.tray.comp
	if c == nil || m.tray.store == nil || strings.TrimSpace(c.c.Value()) == "" {
		return nil
	}
	m.keepDraft(c.d, c.c.Value())
	return m.tray.store.Save(m.trayKey(), m.tray.drafts)
}

// update takes the messages of the tray, then the rest of the modal's.
func (m *detailModal) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case trayLoadedMsg:
		if msg.owner != m {
			return nil
		}
		if msg.err == nil {
			// Drafts made before the read came back stay.
			for i := range msg.drafts {
				id := msg.drafts[i].ID
				if !slices.ContainsFunc(m.tray.drafts, func(o tray.Draft) bool { return o.ID == id }) {
					m.tray.drafts = append(m.tray.drafts, msg.drafts[i])
				}
			}
		}
		return m.showThreadChange()
	case composer.SubmitMsg:
		return m.endCompose(msg.ID, msg.Text)
	case composer.CancelMsg:
		return m.endCompose(msg.ID, msg.Text)
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
		return m.openComposer(d, "Edit draft")
	}
	if th, ok := m.threadUnderCursor(); ok {
		d := tray.Draft{ID: m.newDraftID(), Commit: f.head, Path: th.Path, ThreadID: th.ID, Side: tray.Right, Line: th.Line}
		return m.openComposer(d, "Draft reply")
	}
	pos, ok := f.diff.Position()
	if !ok {
		return ui.Notify(toast.Info, "Put the cursor on a line to comment.")
	}
	d := tray.Draft{ID: m.newDraftID(), Commit: f.head, Path: pos.Path, Side: traySide(pos.Side), Line: pos.Line}
	title := "Draft comment"
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
	return m.openComposer(d, title)
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
