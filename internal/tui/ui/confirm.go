package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Confirm is a change that waits for the user to say yes. A modal asks it
// on its last line, as a step inside its frame; a list asks it in a
// [ConfirmModal].
type Confirm struct {
	// Question asks, such as "Close issue #12?".
	Question string
	// Run makes the change once the user says yes, and returns what sends
	// it.
	Run func() tea.Cmd
	// Default is the answer that enter gives: true for yes. Only a question
	// that is safe and easy to undo defaults to yes; the zero value, no, is
	// for everything destructive, hard to undo or bulk.
	Default bool
}

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
func Recheck(question, name string, now func() (c Confirm, ok bool, refusal tea.Cmd)) Confirm {
	return Confirm{Question: question, Run: func() tea.Cmd {
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

// ConfirmKeys answer a Confirm. They only mean something while a question
// is open.
type ConfirmKeys struct {
	Yes key.Binding `keymap:"yes" help:"yes"`
	No  key.Binding `keymap:"no" help:"no"`
	// Default gives the answer that the question shows as its default.
	Default key.Binding `keymap:"default" help:"answer the default"`
}

// NewConfirmKeys returns the keys that answer a question, from the context
// confirm: y for yes, and n or esc for no by default, and enter for the
// answer the question shows as its default, which is no unless the change is
// safe and easy to undo. A press that arrives before the question is armed,
// or a repeated one, never gives that answer.
func NewConfirmKeys(keys config.Keymap) ConfirmKeys {
	var k ConfirmKeys
	keymap.Fill(&k, Lookup(keys, "confirm"))
	// The answers are named by their first key, as the line that asks has
	// little room.
	for _, b := range []*key.Binding{&k.Yes, &k.No, &k.Default} {
		if ks := b.Keys(); len(ks) > 0 {
			b.SetHelp(keymap.Label(ks[0]), b.Help().Desc)
		}
	}
	return k
}

// ShortHelp implements help.KeyMap.
func (k ConfirmKeys) ShortHelp() []key.Binding { return []key.Binding{k.Yes, k.No, k.Default} }

// FullHelp implements help.KeyMap.
func (k ConfirmKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// Layer returns the layer of the keys, which take every key while the
// question is open: those that answer it, and the rest to do nothing.
func (k ConfirmKeys) Layer() keyhelp.Layer {
	return ContextHelp("confirm", k, false)
}

// Answer takes msg as the answer to c: yes runs c and returns what sends
// the change, and no drops it. The default key gives the default of c, but
// only once the question is armed (see [Arm]) and not for a repeated press.
// Either way done is set, and the question closes. Any other key does
// nothing and leaves the question open.
func (k ConfirmKeys) Answer(c Confirm, msg tea.KeyPressMsg, armed bool) (cmd tea.Cmd, done bool) {
	switch {
	case key.Matches(msg, k.Yes):
		return c.Run(), true
	case key.Matches(msg, k.No):
		return nil, true
	case key.Matches(msg, k.Default) && armed && !msg.IsRepeat:
		if c.Default {
			return c.Run(), true
		}
		return nil, true
	}
	return nil, false
}

// ConfirmStyles style the line of a Confirm.
type ConfirmStyles struct {
	Question, Keys lipgloss.Style
	// DefaultKey styles the key that gives the default answer.
	DefaultKey lipgloss.Style
	// Ellipsis ends a question cut to its room; empty, it is "…".
	Ellipsis string
	// Separator joins the answers to the default; empty, it is the Unicode
	// set's.
	Separator string
}

// ellipsis is the ellipsis of st, or the Unicode set's while it has none.
func (st ConfirmStyles) ellipsis() string {
	return Icons{Ellipsis: st.Ellipsis}.OrUnicode().Ellipsis
}

// Confirm returns the styles of a confirmation: the question in the
// accent, and the keys that answer it muted, cut with the ellipsis of ic.
func (t Theme) Confirm(ic Icons) ConfirmStyles {
	return ConfirmStyles{Question: t.Accent.Bold(true), Keys: t.Muted, DefaultKey: t.Accent, Ellipsis: ic.Ellipsis, Separator: ic.Separator}
}

// ConfirmLines is the most lines a question wraps to, so that a long one,
// such as a merge into a long branch, keeps its end at 80 columns.
const ConfirmLines = 2

// Line renders c on a line of w cells: the question, cut to leave room,
// and the keys that answer it against the right edge.
func (c Confirm) Line(st ConfirmStyles, k ConfirmKeys, w int) string {
	_, keys := c.keys(st, k)
	return Spread(st.Question.Render(OneLine(c.Question)), keys, w, st.ellipsis())
}

// Lines renders c on at most n lines of w cells: the question, wrapped at
// its spaces to leave room for the keys that answer it, which stand
// against the right edge of the last line. What doesn't fit in n lines,
// or a word longer than a line, such as a branch name, is cut.
func (c Confirm) Lines(st ConfirmStyles, k ConfirmKeys, w, n int) []string {
	_, keys := c.keys(st, k)
	room := w - ansi.StringWidth(keys) - 1
	if n <= 1 || room < 1 {
		return []string{c.Line(st, k, w)}
	}
	// A question may name what GitHub calls something, such as a title.
	wrapped := wrapWords(OneLine(c.Question), room)
	if len(wrapped) > n {
		rest := strings.Join(wrapped[n-1:], " ")
		wrapped = append(wrapped[:n-1], termtext.Truncate(rest, room, st.ellipsis()))
	}
	lines := make([]string, len(wrapped))
	last := len(wrapped) - 1
	for i, l := range wrapped {
		l = st.Question.Render(l)
		if i < last {
			lines[i] = Fit(termtext.Truncate(l, w, st.ellipsis()), w)
		} else {
			lines[i] = Spread(l, keys, w, st.ellipsis())
		}
	}
	return lines
}

// wrapWords wraps s into lines of at most w cells, breaking only at
// spaces, since ansi's wrapping breaks at hyphens too, which splits branch
// names. A word longer than w is left whole on a line of its own.
func wrapWords(s string, w int) []string {
	var lines []string
	var line string
	for word := range strings.FieldsSeq(s) {
		switch {
		case line == "":
			line = word
		case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	return append(lines, line)
}

// answers names the keys that answer, such as "y/n".
func (k ConfirmKeys) answers() string {
	var keys []string
	for _, b := range []key.Binding{k.Yes, k.No} {
		if h := b.Help().Key; h != "" {
			keys = append(keys, h)
		}
	}
	return strings.Join(keys, "/")
}

// keys renders what stands against the right edge: the answers, and, after
// the separator, the key that gives the default and what it answers, such
// as "y/n · enter: no". plain is the same without styles. Without a default
// key there is only the answers.
func (c Confirm) keys(st ConfirmStyles, k ConfirmKeys) (plain, styled string) {
	plain, styled = k.answers(), st.Keys.Render(k.answers())
	dk := k.Default.Help().Key
	if dk == "" {
		return plain, styled
	}
	answer := "no"
	if c.Default {
		answer = "yes"
	}
	sep := Icons{Separator: st.Separator}.OrUnicode().Separator
	plain += sep + dk + ": " + answer
	styled += st.Keys.Render(sep) + st.DefaultKey.Render(dk) + st.Keys.Render(": "+answer)
	return plain, styled
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
	ask           Confirm
	keys          ConfirmKeys
	st            ConfirmStyles
	icons         Icons
	width, height int
	// answered is set once the user answered, so that a second yes, such
	// as a repeated key, arriving before the modal closes does nothing.
	answered bool
	// armed is set once Init's message came back: until then the default
	// key answers nothing.
	armed bool
	// view is rendered whenever the size or the styles change.
	view string
}

// NewConfirmModal returns the modal that asks c, answered with keys, cut
// with the ellipsis of ic where it must be.
func NewConfirmModal(c Confirm, keys ConfirmKeys, ic Icons) *ConfirmModal {
	return &ConfirmModal{ask: c, keys: keys, icons: ic}
}

// OpenConfirm opens the modal that asks c, and arms it once it is open.
func OpenConfirm(c Confirm, keys ConfirmKeys, ic Icons) tea.Cmd {
	return OpenModalLoading(NewConfirmModal(c, keys, ic), nil, Arm())
}

// Title implements Modal.
func (m *ConfirmModal) Title() string { return "Confirm" }

// Question returns what the modal asks.
func (m *ConfirmModal) Question() string { return m.ask.Question }

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
	cmd, done := m.keys.Answer(m.ask, k, m.armed)
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
	plain, _ := m.ask.keys(m.st, m.keys)
	w := min(maxWidth, ansi.StringWidth(m.ask.Question)+2+ansi.StringWidth(plain))
	return w, min(maxHeight, len(m.ask.Lines(m.st, m.keys, w, ConfirmLines)))
}

// SetTheme implements Modal.
func (m *ConfirmModal) SetTheme(t Theme) {
	m.st = t.Confirm(m.icons)
	m.render()
}

// Commands implements Commanded: the confirmation takes only the commands that act on the app.
func (m *ConfirmModal) Commands() []string { return nil }

// KeyLayers implements Keyed: the keys that answer the question.
func (m *ConfirmModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{m.keys.Layer()}
}

func (m *ConfirmModal) render() {
	if m.width <= 0 || m.height <= 0 {
		m.view = ""
		return
	}
	lines := PadLines(m.ask.Lines(m.st, m.keys, m.width, min(m.height, ConfirmLines)), m.width, m.height)
	m.view = strings.Join(lines, "\n")
}
