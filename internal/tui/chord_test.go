package tui

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// chordKeys is the help of a section that goes to the top on "g g" or
// home, and that has an action of a lone g when alone is set.
type chordKeys struct{ alone bool }

func (k chordKeys) ShortHelp() []key.Binding {
	out := []key.Binding{keymap.Record(key.NewBinding(key.WithKeys("g g", "home"), key.WithHelp("gg", "top")), "files.top")}
	if k.alone {
		out = append(out, keymap.Record(key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "group")), "files.group"))
	}
	return out
}
func (k chordKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// chordApp returns an app whose focused section has chordKeys, and the
// timeouts that its chord waits asked for.
func chordApp(t *testing.T, alone bool) (*Model, *fakeSection, *[]chordTimeoutMsg) {
	t.Helper()
	m, fakes := newTestApp(t)
	fakes[0].keyMap = chordKeys{alone: alone}
	// What the app sent the section while it started isn't under test.
	fakes[0].msgs = nil
	var waits []chordTimeoutMsg
	m.after = func(d time.Duration, msg tea.Msg) tea.Cmd {
		if d != m.cfg.UI.ChordWait {
			t.Errorf("a chord waits %v, want the chord wait %v", d, m.cfg.UI.ChordWait)
		}
		if to, ok := msg.(chordTimeoutMsg); ok {
			waits = append(waits, to)
		}
		return nil
	}
	return m, fakes[0], &waits
}

func TestChordCompletes(t *testing.T) {
	t.Parallel()
	m, files, waits := chordApp(t, true)
	run(m, m.key(press("g")))
	if len(*waits) != 1 || len(files.msgs) != 0 {
		t.Fatalf("g waited %d times and reached the section %d times, want one wait and no key", len(*waits), len(files.msgs))
	}
	run(m, m.key(press("g")))
	if !files.got(isActionPress("files.top")) || files.got(isKey("g")) {
		t.Errorf("g g reached the section as %v, want the press of files.top alone", files.msgs)
	}
	// The wait of the first g that ends late does nothing.
	run(m, func() tea.Msg { return (*waits)[0] })
	if files.got(isKey("g")) {
		t.Error("the stale wait counted the g alone")
	}
}

func TestChordSurvivesALoneModifier(t *testing.T) {
	t.Parallel()
	m, files, _ := chordApp(t, true)
	run(m, m.key(press("g")))
	// A terminal that reports every key as an escape code sends the shift
	// held for a capital before the key itself.
	for _, code := range []rune{tea.KeyLeftShift, tea.KeyCapsLock, tea.KeyIsoLevel5Shift} {
		_, cmd := m.Update(tea.KeyPressMsg{Code: code, Mod: tea.ModShift})
		run(m, cmd)
		_, cmd = m.Update(tea.KeyReleaseMsg{Code: code})
		run(m, cmd)
	}
	run(m, m.key(press("g")))
	if !files.got(isActionPress("files.top")) || files.got(isKey("g")) {
		t.Errorf("g, shift, g reached the section as %v, want the press of files.top alone", files.msgs)
	}
}

func TestChordTimeoutCountsTheKeyAlone(t *testing.T) {
	t.Parallel()
	m, files, waits := chordApp(t, true)
	run(m, m.key(press("g")))
	run(m, func() tea.Msg { return (*waits)[0] })
	if !files.got(isKey("g")) || files.got(isActionPress("files.top")) {
		t.Errorf("the section got %v after the wait, want the g alone", files.msgs)
	}
	if m.chord.waiting {
		t.Error("the g still waits after its timeout")
	}
}

func TestChordReplaysTheKeyBeforeAnother(t *testing.T) {
	t.Parallel()
	m, files, _ := chordApp(t, true)
	run(m, m.key(press("g")))
	run(m, m.key(press("j")))
	if len(files.msgs) != 2 || !isKey("g")(files.msgs[0]) || !isKey("j")(files.msgs[1]) {
		t.Errorf("the section got %v, want g and then j", files.msgs)
	}
}

func TestChordStaleTimeoutDoesNothing(t *testing.T) {
	t.Parallel()
	m, files, waits := chordApp(t, true)
	run(m, m.key(press("g")))
	run(m, m.key(press("j")))
	run(m, m.key(press("g")))
	if len(*waits) != 2 {
		t.Fatalf("got %d waits, want 2", len(*waits))
	}
	files.msgs = nil
	run(m, func() tea.Msg { return (*waits)[0] })
	if len(files.msgs) != 0 || !m.chord.waiting {
		t.Errorf("the wait of an earlier g reached the section with %v, or ended the later one", files.msgs)
	}
	run(m, func() tea.Msg { return (*waits)[1] })
	if len(files.msgs) != 1 || !isKey("g")(files.msgs[0]) {
		t.Errorf("the wait of the later g gave %v, want the g", files.msgs)
	}
}

func TestChordLoneKeyDoesNothingWhereNothingBindsIt(t *testing.T) {
	t.Parallel()
	m, files, waits := chordApp(t, false)
	run(m, m.key(press("g")))
	run(m, func() tea.Msg { return (*waits)[0] })
	// The held g reaches the section as any other key does; it binds none.
	for _, msg := range files.msgs {
		if _, ok := keymap.Pressed(msg); ok {
			t.Errorf("the section got the action press %v from a lone g", msg)
		}
	}
}

func TestChordHomeIsNotHeld(t *testing.T) {
	t.Parallel()
	m, files, waits := chordApp(t, false)
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyHome}))
	if len(*waits) != 0 || !files.got(isKey("home")) {
		t.Errorf("home waited %d times and gave %v, want it taken at once", len(*waits), files.msgs)
	}
}

func TestChordIsNotHeldWhileTyping(t *testing.T) {
	t.Parallel()
	t.Run("command line", func(t *testing.T) {
		t.Parallel()
		m, _, waits := chordApp(t, true)
		run(m, m.key(press(":")))
		run(m, m.key(press("g")))
		if len(*waits) != 0 || m.chord.waiting {
			t.Error("a g typed into the command line waited for a sequence")
		}
	})
	t.Run("a section that types", func(t *testing.T) {
		t.Parallel()
		m, files, waits := chordApp(t, true)
		files.capturing = true
		run(m, m.key(press("g")))
		if len(*waits) != 0 || !files.got(isKey("g")) {
			t.Errorf("a g typed into a query waited %d times and gave %v, want it typed at once", len(*waits), files.msgs)
		}
	})
}

func TestChordQuitIsNotHeld(t *testing.T) {
	t.Parallel()
	m, _, waits := chordApp(t, true)
	if cmd := m.key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil || len(*waits) != 0 {
		t.Error("ctrl+c waited for a sequence, want it to quit at once")
	}
}
