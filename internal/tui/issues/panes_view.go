package issues

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// headerRows is the rows above the panes: those of the labels, none for an
// issue without any, so the panes have the room.
func (m *detailModal) headerRows() int {
	if m.head == "" {
		return 0
	}
	return strings.Count(m.head, "\n") + 1
}

// paneLayout lays the panes out in what the header and the prompt leave.
func (m *detailModal) paneLayout() panes.Layout {
	h := max(m.height-m.promptHeight()-m.headerRows(), 0)
	return issueLayout(m.width, h, m.nav.Focus(), m.zoom.Zoomed())
}

// convRect is the room of the conversation: the inside of its box, which is
// empty while another pane is zoomed.
func (m *detailModal) convRect() panes.Rect {
	b, ok := m.paneLayout().Box(paneConversation, panes.Whole)
	if !ok {
		return panes.Rect{}
	}
	return b.Rect.Inner()
}

// composed renders the header, the panes and the prompt under them while one
// is open.
func (m *detailModal) composed() string {
	var lines []string
	if m.head != "" {
		for l := range strings.SplitSeq(m.head, "\n") {
			lines = append(lines, ui.Seal(l, m.width))
		}
	}
	l := m.paneLayout()
	if l.Height > 0 {
		lines = append(lines, strings.Split(m.drawPanes(l), "\n")...)
	}
	if m.composing != composeNone {
		lines = append(lines, strings.Split(m.prompt.View(), "\n")...)
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	blank := ui.Seal("", m.width)
	for len(lines) < m.height {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

// drawPanes draws the boxes of l with their titles, and the strip of the
// panes on the border of a zoomed one.
func (m *detailModal) drawPanes(l panes.Layout) string {
	return m.frames.Draw(l, func(b panes.Box) panes.Content {
		c := panes.Content{Focus: b.Pane == l.Focus}
		title := "[" + strconv.Itoa(int(b.Pane)) + "] " + paneName(b.Pane)
		if l.Zoomed {
			// The digits are in the strip on the bottom border.
			title = paneName(b.Pane)
			c.BottomLeft = m.frames.StripForms(paneDefs, b.Pane)
		}
		c.TopLeft = titleForms(title, b, m.icons.Ellipsis)
		c.Body = m.paneBody(b)
		return c
	})
}

// titleForms are the forms of a title for box b: the whole title, then the
// title cut to the room the border leaves it.
func titleForms(title string, b panes.Box, ellipsis string) []string {
	room := b.Rect.W - 5
	if room <= 0 || ansi.StringWidth(title) <= room {
		return []string{title}
	}
	return []string{title, ansi.Truncate(title, room, ellipsis)}
}

// paneBody is what pane b shows.
func (m *detailModal) paneBody(b panes.Box) string {
	in := b.Rect.Inner()
	switch b.Pane {
	case paneDescription:
		return m.describe(in.W)
	case paneConversation:
		if m.convOn && in.H > 0 {
			return m.conv.View()
		}
		return ""
	}
	if s, ok := m.sideBody(b); ok {
		return s
	}
	// Blockers shows its list in a later change.
	return m.theme.Muted.Render("none")
}

// describe renders who wrote the issue and when, whom it is assigned to, how
// many comments it has, and its text.
func (m *detailModal) describe(w int) string {
	if !m.loaded || w <= 0 {
		return ""
	}
	it := m.issue
	now := m.now()
	meta := login(it.Author) + " opened " + m.dates.Prose(it.CreatedAt, now)
	if it.UpdatedAt.After(it.CreatedAt) {
		meta += ", updated " + m.dates.Prose(it.UpdatedAt, now)
	}
	lines := []string{m.theme.Subtle.Render(meta)}
	if len(it.Assignees) > 0 {
		names := make([]string, len(it.Assignees))
		for i, a := range it.Assignees {
			names[i] = login(a)
		}
		lines = append(lines, m.theme.Subtle.Render("assigned to "+strings.Join(names, ", ")))
	}
	lines = append(lines, m.theme.Subtle.Render(plural(it.Comments, "comment")), "")
	if strings.TrimSpace(it.Body) != "" {
		lines = append(lines, m.md.Render(it.Body, w))
	}
	return strings.Join(lines, "\n")
}
