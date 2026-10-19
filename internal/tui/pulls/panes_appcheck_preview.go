package pulls

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/appcheck"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// checkNotes are the annotations that a read of the preview brought, and the
// state of that read.
type checkNotes struct {
	state appcheck.Notes
	notes []core.Annotation
}

// mergeNotesMsg carries the annotations that the read for the preview of a
// check run brought.
type mergeNotesMsg struct {
	owner *panesModal
	check int64
	notes []core.Annotation
	err   error
}

// checkOf returns the check run of an App that the row with the ID id is.
func (p *mergePane) checkOf(id string) (core.CommitCheck, bool) {
	rest, ok := strings.CutPrefix(id, "check/")
	if !ok {
		return core.CommitCheck{}, false
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || n == 0 {
		return core.CommitCheck{}, false
	}
	i := slices.IndexFunc(p.data.Checks.Runs, func(c core.CommitCheck) bool { return c.ID == n })
	if i < 0 {
		return core.CommitCheck{}, false
	}
	return p.data.Checks.Runs[i], true
}

// hoveredCheck is the check run of an App that the preview of the Merge pane
// shows, when it is shown and the modal can read annotations.
func (m *panesModal) hoveredCheck() (core.CommitCheck, bool) {
	p := m.merge
	if p == nil || m.checks == nil || m.focus != panes.Merge || m.closed {
		return core.CommitCheck{}, false
	}
	if _, ok := m.layout().Box(panes.Merge, panes.Preview); !ok {
		return core.CommitCheck{}, false
	}
	return p.checkOf(m.hover[panes.Merge-1].id)
}

// loadNotes reads the annotations of the check run that the preview shows,
// once. It runs after every message, and does nothing when they are at hand,
// being read, or couldn't be read: a failed read is not tried again.
func (m *panesModal) loadNotes() tea.Cmd {
	cc, ok := m.hoveredCheck()
	if !ok {
		return nil
	}
	p := m.merge
	if _, seen := p.notes[cc.ID]; seen {
		return nil
	}
	svc, q := m.checks, notesQuery(m.repo, cc)
	// What is at hand is not kept here: the count of a running check grows, and
	// checkBody takes it from the check each time.
	if _, kept := keptNotes(svc, q, cc); kept {
		return nil
	}
	p.notes[cc.ID] = &checkNotes{state: appcheck.NotesLoading}
	p.version++
	ctx, owner := m.ctx, m
	return func() tea.Msg {
		notes, err := readNotes(ctx, svc, q)
		return mergeNotesMsg{owner: owner, check: cc.ID, notes: notes, err: err}
	}
}

// receiveNotes keeps the annotations that a read brought. A read that failed is
// kept as settled.
func (m *panesModal) receiveNotes(msg mergeNotesMsg) {
	if msg.owner != m {
		return
	}
	n := &checkNotes{notes: msg.notes}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return
		}
		n = &checkNotes{state: appcheck.NotesFailed}
	}
	m.merge.notes[msg.check] = n
	m.merge.version++
}

// checkBody draws the page of the check run of the row id to width, or reports
// false for a row that is no check run of an App.
func (m *panesModal) checkBody(id string, width int) (string, bool) {
	p := m.merge
	cc, ok := p.checkOf(id)
	if !ok {
		return "", false
	}
	d := appcheck.Data{Check: cc}
	if n := p.notes[cc.ID]; n != nil {
		d.Notes, d.State = n.notes, n.state
	} else if m.checks == nil {
		// Nothing reads annotations, so only those the check came with show.
		d.Notes = cc.Notes
	} else if notes, kept := keptNotes(m.checks, notesQuery(m.repo, cc), cc); kept {
		d.Notes = notes
	} else {
		d.State = appcheck.NotesLoading
	}
	if p.page == nil {
		md := markdown.New(m.theme.Thread(m.icons).Markdown)
		md.SetGlyphs(m.icons.Markdown)
		p.page = &appcheck.Options{Styles: appcheck.NewStyles(m.theme, m.icons), MD: md}
	}
	return appcheck.Build(d, *p.page, width).String(), true
}
