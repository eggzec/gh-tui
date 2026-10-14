package pulls

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// The list of the drafts of the review tray, over the tab on view, and the
// drafts that the head's moving made outdated.

// trayList is the open list of drafts: the cursor stays on a draft by its ID,
// so that it keeps its row when the drafts change.
type trayList struct {
	id string
}

// draftOutdated is what the diff and the list call a draft whose line changed.
const draftOutdated = "outdated"

// openTrayList opens the list of the drafts.
func (m *detailModal) openTrayList() tea.Cmd {
	if len(m.tray.drafts) == 0 {
		return ui.Notify(toast.Info, "No drafts: put the cursor on a line of the diff and comment.")
	}
	m.tray.list = &trayList{id: m.tray.drafts[0].ID}
	m.tray.replacing = ""
	return nil
}

// listCursor returns the row of the list that has the cursor, from its draft.
func (m *detailModal) listCursor() int {
	l := m.tray.list
	i := slices.IndexFunc(m.tray.drafts, func(d tray.Draft) bool { return d.ID == l.id })
	return max(i, 0)
}

// moveList moves the cursor of the list by delta rows, staying in it.
func (m *detailModal) moveList(delta int) {
	n := len(m.tray.drafts)
	if n == 0 {
		return
	}
	m.tray.list.id = m.tray.drafts[min(max(m.listCursor()+delta, 0), n-1)].ID
}

// pressTrayList takes a key of the open list, which takes every key.
func (m *detailModal) pressTrayList(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	l := m.tray.list
	cur := m.listCursor()
	switch {
	case keymap.Matches(msg, k.overview.Up):
		m.moveList(-1)
	case keymap.Matches(msg, k.overview.Down):
		m.moveList(1)
	case keymap.Matches(msg, k.Back, k.Tray):
		m.tray.list = nil
	case keymap.Matches(msg, k.showThread) && cur < len(m.tray.drafts):
		m.tray.list = nil
		return m.showDraft(m.tray.drafts[cur].ID)
	case keymap.Matches(msg, k.deleteDraft) && cur < len(m.tray.drafts) && m.tray.drafts[cur].OnGitHub:
		return ui.Notify(toast.Info, onGitHubText)
	case keymap.Matches(msg, k.deleteDraft) && cur < len(m.tray.drafts):
		id := m.tray.drafts[cur].ID
		cmd := m.removeDraft(id)
		if len(m.tray.drafts) == 0 {
			m.tray.list = nil
		} else {
			l.id = m.tray.drafts[min(cur, len(m.tray.drafts)-1)].ID
		}
		return cmd
	case keymap.Matches(msg, k.comment) && cur < len(m.tray.drafts):
		d := m.tray.drafts[cur]
		if d.OnGitHub {
			return ui.Notify(toast.Info, onGitHubText)
		}
		if d.ThreadID != "" {
			return ui.Notify(toast.Info, "A reply stays with its thread.")
		}
		if !d.Outdated {
			return ui.Notify(toast.Info, "Only an outdated draft is placed on a new line.")
		}
		m.tray.list = nil
		// A range that was started is another comment's.
		m.tray.from = nil
		m.tray.replacing = d.ID
		return tea.Batch(m.showDraft(d.ID), ui.Notify(toast.Info, "Move to the new line, then comment to place the draft there."))
	}
	return nil
}

// showDraft shows the Files tab with the diff's cursor on draft id, which
// sits under its line, or under the header of its file if the diff can't
// place it.
func (m *detailModal) showDraft(id string) tea.Cmd {
	i := slices.IndexFunc(m.tray.drafts, func(d tray.Draft) bool { return d.ID == id })
	if i < 0 {
		return nil
	}
	path := m.tray.drafts[i].Path
	cmd := m.switchTo(filesTab)
	if m.files == nil {
		return cmd
	}
	return tea.Batch(cmd, m.seekDiff(path, func(d *diff.Model) { d.SeekBlock(draftPrefix + id) }))
}

// removeDraft deletes draft id, and saves the rest.
func (m *detailModal) removeDraft(id string) tea.Cmd {
	m.tray.drafts = slices.DeleteFunc(m.tray.drafts, func(o tray.Draft) bool { return o.ID == id })
	if m.tray.replacing == id {
		m.tray.replacing = ""
	}
	return tea.Batch(m.saveTray(), m.showThreadChange())
}

// draftWhere says where draft d is: its file and line, or the thread that it
// replies to.
func draftWhere(d tray.Draft) string {
	where := d.Path
	switch {
	case d.StartLine > 0 && d.StartLine < d.Line:
		where += ":" + strconv.Itoa(d.StartLine) + "-" + strconv.Itoa(d.Line)
	case d.Line > 0:
		where += ":" + strconv.Itoa(d.Line)
	}
	if d.ThreadID != "" {
		return "reply to " + where
	}
	return where
}

// trayListView draws the list of drafts in the modal's size.
func (m *detailModal) trayListView() string {
	w, h := m.width, m.height
	title := " " + m.theme.Accent.Bold(true).Render("Drafts") + m.st.age.Render(" ("+strconv.Itoa(len(m.tray.drafts))+")")
	lines := []string{ansi.Truncate(title, w, ""), ""}
	cur := m.listCursor()
	rows := max(h-len(lines), 1)
	// Two lines for each draft, kept in the window around the cursor.
	per := 2
	fit := max(rows/per, 1)
	first := min(max(cur-fit/2, 0), max(len(m.tray.drafts)-fit, 0))
	for i := first; i < len(m.tray.drafts) && i < first+fit; i++ {
		d := m.tray.drafts[i]
		mark := "  "
		if i == cur {
			mark = m.theme.Accent.Render(m.icons.Dot) + " "
		}
		head := " " + mark + m.st.title.Render(draftWhere(d))
		if d.Outdated {
			head += " " + m.theme.Warning.Render(draftOutdated)
		}
		if d.OnGitHub {
			head += " " + m.st.age.Render(draftOnGitHub)
		}
		first, _, _ := strings.Cut(d.Body, "\n")
		lines = append(lines, ansi.Truncate(head, w, m.icons.Ellipsis),
			ansi.Truncate("     "+m.st.age.Render(ui.OneLine(first)), w, m.icons.Ellipsis))
	}
	// Every line is as wide as the modal, so that its frame keeps its size.
	return strings.Join(ui.FitLines(lines, w, h), "\n")
}

// trayListLayer is the keys of the open list, for help.
func (m *detailModal) trayListLayer() keyhelp.Layer {
	k := m.keys
	cur := m.listCursor()
	show := renamed(k.showThread, "show in diff")
	place := renamed(k.comment, "place on a new line")
	place.SetEnabled(place.Enabled() && cur < len(m.tray.drafts) && m.tray.drafts[cur].Outdated)
	keys := []key.Binding{k.overview.Up, k.overview.Down, show, place, k.deleteDraft, k.Back}
	return ui.ContextLayer(ctxModal, keys, []key.Binding{show, k.deleteDraft, k.Back})
}

// checkDrafts settles the drafts written on another commit than the head the
// diff shows, as far as the files fetched tell: a draft whose line is there as it
// was moves to the head, and one whose line is gone or changed is marked
// outdated, and stays on its commit. It reports whether any draft changed.
func (m *detailModal) checkDrafts() bool {
	f := m.files
	if f == nil || f.head == "" {
		return false
	}
	changed := false
	for i := range m.tray.drafts {
		d := &m.tray.drafts[i]
		// A reply is to a thread, whichever commit it was written on.
		if d.OnGitHub || d.Outdated || d.ThreadID != "" || d.Commit == "" || d.Commit == f.head {
			continue
		}
		pos := diff.Pos{Path: d.Path, Side: diffSide(d.Side), Line: d.Line}
		text, shown, known := f.diff.Line(pos)
		if !known {
			continue
		}
		// A draft kept before the text was recorded has none to compare, and
		// moves if its line shows; an empty line is a text.
		same := shown && (!d.HasText && d.Text == "" || d.Text == text)
		if same && d.StartLine > 0 {
			pos.Line = d.StartLine
			start, startShown, _ := f.diff.Line(pos)
			same = startShown && (!d.HasText || d.StartText == start)
		}
		if same {
			d.Commit = f.head
		} else {
			d.Outdated = true
		}
		changed = true
	}
	return changed
}

// noteText records on draft d the text of the lines it is on, as the diff
// shows them, to tell later whether they changed.
func (m *detailModal) noteText(d *tray.Draft) {
	pos := diff.Pos{Path: d.Path, Side: diffSide(d.Side), Line: d.Line}
	var shown bool
	d.Text, shown, _ = m.files.diff.Line(pos)
	d.StartText = ""
	if d.StartLine > 0 {
		pos.Line = d.StartLine
		d.StartText, _, _ = m.files.diff.Line(pos)
	}
	d.HasText = shown
}

// settleDrafts runs checkDrafts, and saves and shows what it changed.
func (m *detailModal) settleDrafts() tea.Cmd {
	if !m.checkDrafts() {
		return nil
	}
	return tea.Batch(m.saveTray(), m.showThreadChange())
}
