package filterform

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// Glyphs of the form, rendered in the styles that hold them.
const (
	cursorGlyph   = "▌"
	onGlyph       = "●"
	offGlyph      = "○"
	boxOn         = "[x]"
	boxOff        = "[ ]"
	removeGlyph   = "✕"
	dropGlyph     = "▾"
	addText       = "+ add"
	ruleGlyph     = "─"
	chosenMark    = "✓ "
	notChosenMark = "· "
)

// Styles holds the styles of a form.
type Styles struct {
	// Tab styles the names of the tabs, and ActiveTab the one on view.
	Tab       lipgloss.Style
	ActiveTab lipgloss.Style
	// Gutter marks the row in focus.
	Gutter lipgloss.Style
	// Label styles the names of the rows, and FocusedLabel the one in
	// focus.
	Label        lipgloss.Style
	FocusedLabel lipgloss.Style
	// Option styles the choices not taken, Selected the one taken, and
	// Active the one taken in the row in focus.
	Option   lipgloss.Style
	Selected lipgloss.Style
	Active   lipgloss.Style
	// Chip styles an item of a Multi field, and ActiveChip the one under
	// the cursor.
	Chip       lipgloss.Style
	ActiveChip lipgloss.Style
	// Remove styles the ✕ after a chip.
	Remove lipgloss.Style
	// Add styles "+ add" at the end of the chips.
	Add lipgloss.Style
	// Value styles the text of a Text or Person field.
	Value lipgloss.Style
	// Hint styles the hint of an empty field and the placeholder of an
	// empty query.
	Hint lipgloss.Style
	// Rule styles the line above the query.
	Rule lipgloss.Style
	// Query styles the query line.
	Query lipgloss.Style
	// Cursor colors the cursor of the text inputs with its foreground.
	Cursor lipgloss.Style
	// Spinner styles the spinner of a field that is loading.
	Spinner lipgloss.Style
	// Error styles a load that failed.
	Error lipgloss.Style
	// Help styles the help line.
	Help help.Styles
	// Picker styles the picker of a Multi or Person field. Its frame is
	// drawn inside the form, so keep it light.
	Picker picker.Styles
}

// DefaultStyles returns calm styles for a light or dark terminal, with the
// accent color only on what is in focus.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	border := ld(lipgloss.Color("#d5d9e2"), lipgloss.Color("#3a4050"))
	errColor := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))

	pk := picker.DefaultStyles(isDark)
	pk.Frame = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(border).
		PaddingLeft(1)

	return Styles{
		Tab:          lipgloss.NewStyle().Foreground(muted),
		ActiveTab:    lipgloss.NewStyle().Foreground(accent).Bold(true),
		Gutter:       lipgloss.NewStyle().Foreground(accent),
		Label:        lipgloss.NewStyle().Foreground(muted),
		FocusedLabel: lipgloss.NewStyle().Foreground(text).Bold(true),
		Option:       lipgloss.NewStyle().Foreground(subtle),
		Selected:     lipgloss.NewStyle().Foreground(text),
		Active:       lipgloss.NewStyle().Foreground(accent).Bold(true),
		Chip:         lipgloss.NewStyle().Foreground(text),
		ActiveChip:   lipgloss.NewStyle().Foreground(accent).Bold(true),
		Remove:       lipgloss.NewStyle().Foreground(subtle),
		Add:          lipgloss.NewStyle().Foreground(subtle),
		Value:        lipgloss.NewStyle().Foreground(text),
		Hint:         lipgloss.NewStyle().Foreground(subtle),
		Rule:         lipgloss.NewStyle().Foreground(border),
		Query:        lipgloss.NewStyle().Foreground(text),
		Cursor:       lipgloss.NewStyle().Foreground(accent),
		Spinner:      lipgloss.NewStyle().Foreground(accent),
		Error:        lipgloss.NewStyle().Foreground(errColor),
		Help:         help.DefaultStyles(isDark),
		Picker:       pk,
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles and passes them on to the inputs, the spinner,
// the help line and an open picker.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.spin.Style = s.Spinner
	m.help.Styles = s.Help
	in := inputStyles(s)
	m.text.SetStyles(in)
	m.query.SetStyles(in)
	if m.picking {
		m.pick.SetStyles(s.Picker)
	}
	m.cache = renderCache{}
	m.glyphs = glyphs{
		gutter: s.Gutter.Render(cursorGlyph) + " ",
		remove: " " + s.Remove.Render(removeGlyph),
	}
	m.render()
}

// glyphs holds pieces rendered once per style, and the rule once per
// width.
type glyphs struct {
	gutter, remove string
	rule           string
	ruleWidth      int
}

func inputStyles(s Styles) textinput.Styles {
	st := textinput.StyleState{
		Text:        s.Value,
		Placeholder: s.Hint,
		Suggestion:  s.Hint,
		Prompt:      lipgloss.NewStyle(),
	}
	return textinput.Styles{
		Focused: st,
		Blurred: st,
		Cursor:  textinput.CursorStyle{Color: s.Cursor.GetForeground(), Shape: tea.CursorBlock},
	}
}
