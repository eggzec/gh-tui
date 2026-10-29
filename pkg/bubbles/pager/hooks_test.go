package pager

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// hostKey is a key that the keys of the preview leave unbound.
func hostKey(taken *[]string) Handler {
	return func(m Model, k tea.KeyPressMsg) (Model, tea.Cmd, bool) {
		*taken = append(*taken, k.String())
		return m, nil, k.String() == "x"
	}
}

func TestHandlerSeesOnlyUnboundKeys(t *testing.T) {
	var seen []string
	m := open(t, "lines.txt", numbered(50), WithSize(40, 10), WithHandler(hostKey(&seen)))
	m, _ = keys(t, m, "j", "x", "z")
	if len(seen) != 2 || seen[0] != "x" || seen[1] != "z" {
		t.Errorf("handler saw %v, want [x z]: j is the pager's", seen)
	}
	if m.top == 0 && m.row == 0 {
		t.Error("j did not scroll")
	}
}

func TestHandlerCanChangeThePager(t *testing.T) {
	h := func(m Model, k tea.KeyPressMsg) (Model, tea.Cmd, bool) {
		if k.String() != "x" {
			return m, nil, false
		}
		m.SetWrap(true)
		return m, nil, true
	}
	m := open(t, "lines.txt", numbered(5), WithHandler(h))
	m, _ = keys(t, m, "x")
	if !m.Wrap() {
		t.Error("the model the handler returned was dropped")
	}
}

func TestHostCapturingGivesEveryKeyToTheHandler(t *testing.T) {
	var seen []string
	on := true
	m := open(t, "lines.txt", numbered(50), WithSize(40, 10),
		WithHandler(hostKey(&seen)),
		WithHelp(nil, func() bool { return on }))
	if !m.Capturing() || m.Choosing() {
		t.Fatalf("Capturing %v, Choosing %v, want true, false", m.Capturing(), m.Choosing())
	}
	m, _ = keys(t, m, "j", "/")
	if len(seen) != 2 {
		t.Errorf("handler saw %v, want both keys", seen)
	}
	if m.top != 0 || m.row != 0 || m.prompt.Focused() {
		t.Error("the pager acted on a key while the host captured")
	}
	on = false
	m, _ = keys(t, m, "/")
	if !m.prompt.Focused() {
		t.Error("the pager ignored its key once the host stopped capturing")
	}
}

func TestChoosing(t *testing.T) {
	m := open(t, "lines.txt", numbered(5))
	m, _ = keys(t, m, "-")
	if !m.Choosing() || !m.Capturing() {
		t.Errorf("Choosing %v, Capturing %v after the option key, want both", m.Choosing(), m.Capturing())
	}
	m, _ = keys(t, m, "esc")
	if m.Choosing() {
		t.Error("cancel left the option mode open")
	}
}

func TestWithOptionsRestrictsTheOptionMode(t *testing.T) {
	m := open(t, "lines.txt", numbered(5), WithOptions(OptionChop))
	m, _ = keys(t, m, "-")
	if got := len(m.ShortHelp()); got != 2 {
		t.Errorf("option help has %d keys, want cancel and chop", got)
	}
	m, _ = keys(t, m, "N")
	if m.flash != noteNoOption+"N" {
		t.Errorf("an option left out said %q", m.flash)
	}
	if !m.LineNumbers() {
		t.Error("an option left out was toggled")
	}
	m, _ = keys(t, m, "-", "S")
	if !m.Wrap() {
		t.Error("an option offered did not work")
	}
}

func TestHelpListsTheHostsKeys(t *testing.T) {
	host := key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "host"))
	capturing := false
	m := open(t, "lines.txt", numbered(5),
		WithHelp(func() []key.Binding { return []key.Binding{host} }, func() bool { return capturing }))

	short := m.ShortHelp()
	if got := short[len(short)-1]; got.Help().Desc != "host" {
		t.Errorf("short help ends with %q, want the host's key", got.Help().Desc)
	}
	full := m.FullHelp()
	last := full[len(full)-1]
	if len(last) != 1 || last[0].Help().Desc != "host" || !last[0].Enabled() {
		t.Errorf("full help ends with %v, want the host's key alone", last)
	}

	capturing = true
	if short = m.ShortHelp(); len(short) != 1 || short[0].Help().Desc != "host" {
		t.Errorf("short help while the host captures is %v, want its keys only", short)
	}
	for _, row := range m.FullHelp()[:len(m.FullHelp())-1] {
		for _, b := range row {
			if b.Enabled() && len(b.Keys()) > 0 {
				t.Errorf("%v is enabled while the host captures", b.Help())
			}
		}
	}
}

func TestForwardSkipsThePrompt(t *testing.T) {
	m := open(t, "lines.txt", numbered(5))
	m, _ = keys(t, m, "/")
	if cmd := m.Forward(tea.PasteMsg{Content: "abc"}); cmd != nil {
		t.Error("Forward took a message that is not the pager's")
	}
	if got := m.prompt.Value(); got != "" {
		t.Errorf("Forward gave the prompt %q", got)
	}
	m, _ = m.Update(tea.PasteMsg{Content: "abc"})
	if got := m.prompt.Value(); got != "abc" {
		t.Errorf("Update gave the prompt %q, want the paste", got)
	}
}
