package issues

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// reactOverlay is the picker the modal opens over its panes. While one is
// open, the keys go to it alone.
type reactOverlay = prpanes.Overlay

// Marks of the kinds of reaction: boxes, as a checklist is drawn elsewhere.
const (
	kindOn  = "[x]"
	kindOff = "[ ]"
)

// openReact shows the kinds of reaction over the panes for the comment under
// the cursor, each ticked while the viewer has given it. Applying adds the
// ticked kinds that weren't and removes those that no longer are.
func (m *detailModal) openReact() tea.Cmd {
	s, ok := m.conv.Current()
	if !ok || s.Item == nil || (s.Kind != conv.KindComment && s.Kind != conv.KindRunComment) {
		return ui.Notify(toast.Info, "select a comment to react to")
	}
	if strings.HasPrefix(s.Item.ID, "pending:") {
		return ui.Notify(toast.Info, "the comment is still being sent")
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.issue); refused {
		return cmd
	}
	id := s.Item.ID
	glyphs := conv.NewStyles(m.theme, m.icons, m.icons.Set()).Glyphs.Reactions
	o := &prpanes.TickOverlay[string]{Heading: "React", Rows: len(conv.ReactionOrder), Flat: true, On: map[string]bool{}, Was: map[string]bool{}}
	for _, r := range s.Item.Reactions {
		if r.Mine {
			o.On[r.Content], o.Was[r.Content] = true, true
		}
	}
	o.Change = func(on, was map[string]bool) tea.Cmd {
		var cmds []tea.Cmd
		for _, content := range conv.ReactionOrder {
			if on[content] == was[content] {
				continue
			}
			op := m.svc.React(m.repo, m.number, id, content, on[content])
			cmds = append(cmds, m.buildConv(), m.changed(), m.send(op, "react to comment"))
		}
		return tea.Batch(cmds...)
	}
	items := make([]picker.Item, len(conv.ReactionOrder))
	var marked []any
	for i, content := range conv.ReactionOrder {
		name := strings.ToLower(strings.ReplaceAll(content, "_", " "))
		items[i] = picker.Item{Title: glyphs[content] + " " + name, Value: content}
		if o.On[content] {
			marked = append(marked, content)
		}
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	o.P = picker.New(nil,
		picker.WithContext(m.ctx),
		picker.WithItems(items),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
		picker.WithKeyMap(m.keys.pick),
		picker.WithStyles(st),
	)
	o.P.SetMarked(marked)
	m.over = o
	return o.P.Focus()
}

// pressOver gives msg to the picker, and drops it once it is done.
func (m *detailModal) pressOver(msg tea.Msg) tea.Cmd {
	cmd := m.over.Update(msg)
	if m.over.Closed() {
		m.over = nil
	}
	return cmd
}

// covered draws the picker over the dimmed panes v.
func (m *detailModal) covered(v string) string {
	area := m.paneLayout().Height
	r := panes.OverlayRect(panes.Overlay{Kind: m.over.Kind(), Items: m.over.Items(), Text: m.over.Text(), InsetMin: wideMin}, m.width, area)
	dim := m.theme.Subtle.Faint(true)
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		lines[i] = dim.Render(ansi.Strip(l))
	}
	if r.W > 0 && r.H > 0 {
		top := m.headerRows()
		box := strings.Split(m.frames.Frame(panes.Content{TopLeft: []string{m.over.Title()}, Body: m.over.View(max(r.W-2, 0), max(r.H-2, 0)), Focus: true}, r.W, r.H), "\n")
		for i, row := range box {
			y := top + r.Y + i
			if y >= len(lines) {
				break
			}
			lines[y] = ansi.Cut(lines[y], 0, r.X) + "\x1b[m" + row + "\x1b[m" + ansi.Cut(lines[y], r.X+r.W, m.width)
		}
	}
	return strings.Join(lines, "\n")
}
