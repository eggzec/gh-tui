package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// chordState is the first key of a sequence of two, such as the g of "g g",
// held until the second comes or the wait for it ends.
type chordState struct {
	// held is the press that is waiting; the zero press means none waits.
	held tea.KeyPressMsg
	// gen tells the wait of this key from that of an earlier one, whose
	// timeout is then dropped.
	gen int
	// waiting is whether a key is held.
	waiting bool
}

// chordTimeoutMsg ends the wait for the second key of the sequence that the
// key of generation gen started.
type chordTimeoutMsg struct{ gen int }

// key takes a key press. A key that starts a sequence of two that is bound
// where the key would go is held until the next key: if that completes the
// sequence, its action is pressed in the place of both keys; if it doesn't,
// or none comes within the chord wait, the held key counts alone first, and
// then the next one is taken as usual.
func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	if _, ok := keymap.Pressed(msg); ok {
		// An action's press stands for a whole key already.
		return m.pressKey(msg)
	}
	if m.chord.waiting {
		held := m.chord.held
		m.chord = chordState{gen: m.chord.gen}
		if action := m.sequenceAction(held.String(), msg.String()); action != "" {
			return m.pressKey(keymap.ActionPress(action))
		}
		replay := m.pressKey(held)
		return tea.Batch(replay, m.key(msg))
	}
	if m.startsChord(msg) {
		m.chord = chordState{held: msg, gen: m.chord.gen + 1, waiting: true}
		return m.after(m.chordWait, chordTimeoutMsg{gen: m.chord.gen})
	}
	return m.pressKey(msg)
}

// chordTimeout counts the held key alone, when the wait that msg ends is the
// one for the key that is held.
func (m *Model) chordTimeout(msg chordTimeoutMsg) tea.Cmd {
	if !m.chord.waiting || m.chord.gen != msg.gen {
		return nil
	}
	held := m.chord.held
	m.chord = chordState{gen: m.chord.gen}
	return m.pressKey(held)
}

// startsChord reports whether msg is the first key of a sequence bound in a
// layer that would take it. Nothing waits while a layer types, since the
// keys are then text, nor for ctrl+c, which always quits.
func (m *Model) startsChord(msg tea.KeyPressMsg) bool {
	if m.line.Focused() || keymap.Matches(msg, forceQuit) {
		return false
	}
	layers := m.layersNow()
	if slices.ContainsFunc(layers, func(l keyhelp.Layer) bool { return l.Typing }) {
		return false
	}
	name := msg.String()
	for i := range layers {
		for _, b := range layers[i].Bindings {
			if !b.Enabled() {
				continue
			}
			for _, seq := range keymap.Sequences(b) {
				if seq[0] == name {
					return true
				}
			}
		}
	}
	return false
}

// sequenceAction returns the action of the enabled binding with the
// sequence "first second" among the keys that reach something now, or "".
// It takes the first match in any layer, even when a higher layer binds the
// lone first key: the config's clash rule keeps the two apart.
func (m *Model) sequenceAction(first, second string) string {
	for _, l := range m.layersNow() {
		for _, b := range l.Bindings {
			if !b.Enabled() || !slices.Contains(keymap.Sequences(b), [2]string{first, second}) {
				continue
			}
			if actions := keymap.Actions(b); len(actions) > 0 {
				return actions[0]
			}
		}
	}
	return ""
}
