package orderpick

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	space = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	del   = tea.KeyPressMsg{Code: tea.KeyDelete}
	bksp  = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

func letter(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

// testKeys returns the keys of an ordered picker, as the app sets them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	table := map[string][]string{
		"global.next_pane":             {"tab"},
		"global.prev_pane":             {"shift+tab"},
		"move_down":                    {"J"},
		"move_up":                      {"K"},
		"flip":                         {"space"},
		"remove":                       {"x", "delete"},
		"picker.up":                    {"up", "ctrl+p"},
		"picker.down":                  {"down", "ctrl+n"},
		"picker.page_up":               {"pgup"},
		"picker.page_down":             {"pgdown"},
		"picker.choose":                {"enter"},
		"picker.cancel":                {"esc"},
		"picker.next_scope":            {"tab"},
		"picker.prev_scope":            {"shift+tab"},
		"picker_normal.up":             {"k", "up"},
		"picker_normal.down":           {"j", "down"},
		"picker_normal.page_up":        {"ctrl+b", "pgup"},
		"picker_normal.page_down":      {"ctrl+f", "pgdown"},
		"picker_normal.half_page_up":   {"ctrl+u"},
		"picker_normal.half_page_down": {"ctrl+d"},
		"picker_normal.top":            {"g", "home"},
		"picker_normal.bottom":         {"G", "end"},
		"picker_normal.search":         {"/"},
		"picker_normal.toggle":         {"space"},
		"picker_normal.back":           {"backspace"},
	}
	return NewKeyMap(keytest.Table(table))
}

// fields is a catalog of sort fields, each with a natural direction.
var fields = []Item{
	{Value: "created", Title: "Created", AscLabel: "oldest first", DescLabel: "newest first", Desc: true},
	{Value: "updated", Title: "Updated", AscLabel: "oldest first", DescLabel: "newest first", Desc: true},
	{Value: "title", Title: "Title", AscLabel: "A to Z", DescLabel: "Z to A"},
	{Value: "comments", Title: "Comments", AscLabel: "fewest first", DescLabel: "most first", Desc: true},
}

// open returns a focused picker of fields.
func open(tb testing.TB, opts ...Option) Model {
	tb.Helper()
	m := New(append([]Option{WithItems(fields), WithKeyMap(testKeys(tb)), WithSize(80, 14)}, opts...)...)
	m.Focus()
	return m
}

// press sends msgs in order, and returns the messages meant for the parent.
func press(tb testing.TB, m Model, msgs ...tea.Msg) (after Model, sent []tea.Msg) {
	tb.Helper()
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd == nil {
			continue
		}
		switch got := cmd().(type) {
		case ChosenMsg, CancelMsg:
			sent = append(sent, got)
		case nil:
		default:
			m, _ = m.Update(got)
		}
	}
	return m, sent
}

// values returns the values of the chosen items, in order.
func values(m Model) []any {
	out := make([]any, 0, len(m.Chosen()))
	for _, c := range m.Chosen() {
		out = append(out, c.Value)
	}
	return out
}

// selected returns the value of the item under the cursor of the left pane.
func selected(m Model) any {
	it, _ := m.left.Selected()
	if it.Value == nil {
		return nil
	}
	return fields[it.Value.(int)].Value
}

// asciiStyles returns styles that draw ASCII alone.
func asciiStyles() Styles {
	st := DefaultStyles(true)
	st.Frame = st.Frame.Border(lipgloss.ASCIIBorder())
	st.FocusFrame = st.FocusFrame.Border(lipgloss.ASCIIBorder())
	st.Picker.PromptGlyph, st.Picker.CursorGlyph, st.Picker.Ellipsis = ">", ">", "..."
	st.CursorGlyph, st.Ellipsis = ">", "..."
	return st
}

func equalValues(a, b []any) bool { return slices.Equal(a, b) }
