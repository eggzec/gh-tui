package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// armMsg arms the question that asked for it: until it comes back, the
// default key answers nothing, so that the press that opened the question,
// or a key held down, never answers it.
type armMsg struct{}

// Arm returns the command that arms a question just asked; the message it
// sends goes back to the asker, which tells it apart with [IsArm].
func Arm() tea.Cmd { return func() tea.Msg { return armMsg{} } }

// IsArm reports whether msg arms a question.
func IsArm(msg tea.Msg) bool {
	_, ok := msg.(armMsg)
	return ok
}

// Recheck returns the Confirm that asks question and, once the user says
// yes, asks again: by then what the question is about may have changed,
// such as a list that refreshed behind it. now returns the change as it is
// by then, or ok unset when it no longer applies, with refusal saying why
// when the viewer may no longer make it. The change is made only when it
// still asks question, which names the change and its target; otherwise
// nothing is sent, and the user is told that name changed meanwhile.
func Recheck(question, name string, now func() (c confirm.Ask, ok bool, refusal tea.Cmd)) confirm.Ask {
	return confirm.Ask{Question: question, Run: func() tea.Cmd {
		c, ok, refusal := now()
		switch {
		case ok && c.Question == question:
			return c.Run()
		case refusal != nil:
			return refusal
		}
		return Notify(toast.Info, Meanwhile(name))
	}}
}

// Meanwhile says that name changed while its change was being confirmed,
// so it wasn't sent.
func Meanwhile(name string) string {
	return name + " changed meanwhile, so nothing was sent."
}

// ConfirmLayer returns the layer of the keys that answer a question, which
// take every key while the question is open: those that answer it, and the
// rest to do nothing.
func ConfirmLayer(k confirm.KeyMap) keyhelp.Layer {
	return ContextHelp("confirm", k, false)
}

// Confirm returns the styles of a confirmation: the question in the
// accent, and the keys that answer it muted, cut with the ellipsis of ic.
func (t Theme) Confirm(ic Icons) confirm.Styles {
	return confirm.Styles{Question: t.Accent.Bold(true), Keys: t.Muted, DefaultKey: t.Accent, Ellipsis: ic.Ellipsis, Separator: ic.Separator, Key: ic.Key}
}

// OverLastLines returns view with its last lines replaced by lines, such
// as the question of a Confirm over the bottom of a modal.
func OverLastLines(view string, lines []string) string {
	end := len(view)
	for range lines {
		i := strings.LastIndexByte(view[:end], '\n')
		if i < 0 {
			return strings.Join(lines, "\n")
		}
		end = i
	}
	return view[:end+1] + strings.Join(lines, "\n")
}

// ConfirmModal asks a Confirm in a small modal of its own, for a change
// asked outside a modal, such as from a row of a list. Yes runs the change
// and closes it; no closes it as if nothing happened.
type ConfirmModal struct {
	ask   confirm.Model
	keys  confirm.KeyMap
	icons Icons
	// answered is set once the user answered, so that a second yes, such
	// as a repeated key, arriving before the modal closes does nothing.
	answered bool
	// armed is set once Init's message came back: until then the default
	// key answers nothing.
	armed bool
	// view is rendered whenever the size or the styles change.
	view          string
	width, height int
}

// NewConfirmModal returns the modal that asks c, answered with keys, cut
// with the ellipsis of ic where it must be.
func NewConfirmModal(c confirm.Ask, keys confirm.KeyMap, ic Icons) *ConfirmModal {
	return &ConfirmModal{ask: confirm.New(c, confirm.WithKeyMap(keys), confirm.WithStyles(confirm.Styles{})), keys: keys, icons: ic}
}

// OpenConfirm opens the modal that asks c, and arms it once it is open.
func OpenConfirm(c confirm.Ask, keys confirm.KeyMap, ic Icons) tea.Cmd {
	return OpenModalLoading(NewConfirmModal(c, keys, ic), nil, Arm())
}

// Title implements Modal.
func (m *ConfirmModal) Title() string { return "Confirm" }

// Question returns what the modal asks.
func (m *ConfirmModal) Question() string { return m.ask.Ask().Question }

// Update implements Modal. It takes the answer from the keys and ignores
// every other message.
func (m *ConfirmModal) Update(msg tea.Msg) tea.Cmd {
	if s, ok := msg.(SettingsMsg); ok {
		s.ReloadIcons(&m.icons)
		return nil
	}
	if IsArm(msg) {
		m.armed = true
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || m.answered {
		return nil
	}
	cmd, done := m.ask.Answer(k, m.armed)
	if !done {
		return nil
	}
	m.answered = true
	// Leaving returns to the modal that asked, if there is one.
	return tea.Batch(LeaveModal(m), cmd)
}

// View implements Modal.
func (m *ConfirmModal) View() string { return m.view }

// SetSize implements Modal.
func (m *ConfirmModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.render()
}

// Fit implements Fitter: as wide as the question and its keys, or as wide
// as there is room for and on as many lines as the question wraps to.
func (m *ConfirmModal) Fit(maxWidth, maxHeight int) (width, height int) {
	return m.ask.Size(maxWidth, maxHeight)
}

// SetTheme implements Modal.
func (m *ConfirmModal) SetTheme(t Theme) {
	m.ask.SetStyles(t.Confirm(m.icons))
	m.render()
}

// Commands implements Commanded: the confirmation takes only the commands that act on the app.
func (m *ConfirmModal) Commands() []string { return nil }

// KeyLayers implements Keyed: the keys that answer the question.
func (m *ConfirmModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{ConfirmLayer(m.keys)}
}

func (m *ConfirmModal) render() {
	m.ask.SetSize(m.width, m.height)
	m.view = m.ask.View()
}
