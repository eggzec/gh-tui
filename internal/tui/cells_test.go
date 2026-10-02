package tui

import (
	"context"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The cell query's writes.
const (
	askCell   = "\x1b[16t\x1b[c"
	askWindow = "\x1b[14t\x1b[c"
)

// ghosttyApp is a test app 100×30 cells large whose probe found that the
// terminal, ghostty, draws images, and the cell query's first write.
func ghosttyApp(t *testing.T) (*Model, []*fakeSection, []string) {
	t.Helper()
	m, sections := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-ghostty"}, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	var writes []string
	for _, msg := range []tea.Msg{
		tea.ColorProfileMsg{Profile: colorprofile.TrueColor},
		tea.TerminalVersionMsg{Name: "ghostty 1.2.0"}, da1,
		kittyReply(m.images.id, "OK"),
	} {
		answer(m, msg)
	}
	writes = answer(m, da1)
	if !m.graphics.Images {
		t.Fatalf("graphics = %+v, want images (%s)", m.graphics, m.images.verdict.Reason)
	}
	return m, sections, writes
}

// The size of a cell comes from XTWINOPS 16, else from XTWINOPS 14 over
// the columns and rows, else from the fallback; DA1 after each question
// tells a terminal that ignored it from one that is slow.
func TestCellQuery(t *testing.T) {
	timeout := func(m *Model) tea.Msg { return cellTimeoutMsg{id: m.cells.id} }
	msgs := func(ms ...tea.Msg) func(*Model) []tea.Msg {
		return func(*Model) []tea.Msg { return ms }
	}
	tests := []struct {
		name    string
		answers func(m *Model) []tea.Msg
		want    imgcaps.Cell
		writes  []string
	}{
		{
			name:    "the cell size",
			answers: msgs(uv.CellSizeEvent{Width: 9, Height: 18}, da1),
			want:    imgcaps.Cell{Width: 9, Height: 18},
		},
		{
			name:    "the window size over the cells",
			answers: msgs(da1, uv.PixelSizeEvent{Width: 900, Height: 540}, da1),
			want:    imgcaps.Cell{Width: 9, Height: 18},
			writes:  []string{askWindow},
		},
		{
			name:    "an empty cell size asks the window",
			answers: msgs(uv.CellSizeEvent{}, da1, uv.PixelSizeEvent{Width: 1000, Height: 600}, da1),
			want:    imgcaps.Cell{Width: 10, Height: 20},
			writes:  []string{askWindow},
		},
		{
			name:    "neither",
			answers: msgs(da1, da1),
			want:    imgcaps.FallbackCell,
			writes:  []string{askWindow},
		},
		{
			name:    "a window size to the first question is ignored",
			answers: msgs(uv.PixelSizeEvent{Width: 900, Height: 540}, da1, da1),
			want:    imgcaps.FallbackCell,
			writes:  []string{askWindow},
		},
		{
			name:    "another query's timer",
			answers: func(m *Model) []tea.Msg { return []tea.Msg{cellTimeoutMsg{id: m.cells.id - 1}, da1, da1} },
			want:    imgcaps.FallbackCell,
			writes:  []string{askWindow},
		},
		{
			name:    "no answer in time",
			answers: func(m *Model) []tea.Msg { return []tea.Msg{timeout(m)} },
			want:    imgcaps.FallbackCell,
		},
		{
			name: "the window size, then no DA1 in time",
			answers: func(m *Model) []tea.Msg {
				return []tea.Msg{da1, uv.PixelSizeEvent{Width: 900, Height: 540}, timeout(m)}
			},
			want:   imgcaps.Cell{Width: 9, Height: 18},
			writes: []string{askWindow},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, sections, first := ghosttyApp(t)
			if !slices.Equal(first, []string{askCell}) {
				t.Fatalf("the verdict wrote %q, want %q", first, askCell)
			}
			var writes []string
			for _, msg := range tt.answers(m) {
				writes = append(writes, answer(m, msg)...)
			}
			if !slices.Equal(writes, tt.writes) {
				t.Errorf("writes = %q, want %q", writes, tt.writes)
			}
			if m.graphics.Cell != tt.want {
				t.Errorf("cell = %+v, want %+v", m.graphics.Cell, tt.want)
			}
			if m.cells.stage != 0 {
				t.Errorf("the query is still asking, stage %d", m.cells.stage)
			}
			want := ui.Graphics{Images: true, Cell: tt.want}
			for _, s := range sections {
				if !slices.ContainsFunc(s.msgs, func(msg tea.Msg) bool {
					g, ok := msg.(ui.GraphicsMsg)
					return ok && g.Graphics == want
				}) {
					t.Errorf("section %s wasn't told the cell", s.title)
				}
			}
		})
	}
}

// A resize asks again, and one during a query has it ask again once it
// ends; a DA1 the query didn't ask for is the probe's to drop.
func TestCellQueryResize(t *testing.T) {
	m, _, _ := ghosttyApp(t)
	if writes := answer(m, tea.WindowSizeMsg{Width: 120, Height: 40}); len(writes) != 0 {
		t.Errorf("a resize during the query wrote %q", writes)
	}
	answer(m, uv.CellSizeEvent{Width: 9, Height: 18})
	if writes := answer(m, da1); !slices.Equal(writes, []string{askCell}) {
		t.Fatalf("the end of the query wrote %q, want it asked again", writes)
	}
	answer(m, uv.CellSizeEvent{Width: 10, Height: 20})
	answer(m, da1)
	if m.graphics.Cell != (imgcaps.Cell{Width: 10, Height: 20}) {
		t.Errorf("cell = %+v after the font grew", m.graphics.Cell)
	}
	if writes := answer(m, da1); len(writes) != 0 || m.graphics.Cell != (imgcaps.Cell{Width: 10, Height: 20}) {
		t.Errorf("a stray DA1 wrote %q, cell %+v", writes, m.graphics.Cell)
	}
	if writes := answer(m, tea.WindowSizeMsg{Width: 80, Height: 24}); !slices.Equal(writes, []string{askCell}) {
		t.Errorf("a resize wrote %q, want the cell asked", writes)
	}
	// The window size is divided by the latest columns and rows.
	answer(m, da1)
	answer(m, uv.PixelSizeEvent{Width: 800, Height: 480})
	answer(m, da1)
	if m.graphics.Cell != (imgcaps.Cell{Width: 10, Height: 20}) {
		t.Errorf("cell = %+v from 800×480 over 80×24", m.graphics.Cell)
	}
}

// A terminal the probe didn't allow is asked nothing, however it resizes.
func TestCellQueryOnlyWithImages(t *testing.T) {
	m, _ := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-256color"}, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	answer(m, tea.TerminalVersionMsg{Name: "WezTerm 20240203"})
	if writes := answer(m, da1); len(writes) != 0 {
		t.Fatalf("the verdict wrote %q, want nothing", writes)
	}
	if m.graphics.Images {
		t.Fatal("images on in WezTerm")
	}
	if writes := answer(m, tea.WindowSizeMsg{Width: 120, Height: 40}); len(writes) != 0 {
		t.Errorf("a resize wrote %q", writes)
	}
	answer(m, uv.CellSizeEvent{Width: 9, Height: 18})
	if m.graphics.Cell.Valid() {
		t.Errorf("cell = %+v unasked", m.graphics.Cell)
	}
}

// The window's pixels are divided by the columns and rows the query was
// asked with, though a resize came meanwhile.
func TestCellQueryWindowOfTheAsk(t *testing.T) {
	m, _, _ := ghosttyApp(t)
	answer(m, da1)
	answer(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	answer(m, uv.PixelSizeEvent{Width: 900, Height: 540})
	if writes := answer(m, da1); !slices.Equal(writes, []string{askCell}) {
		t.Errorf("the end of the query wrote %q, want it asked again for the resize", writes)
	}
	if m.graphics.Cell != (imgcaps.Cell{Width: 9, Height: 18}) {
		t.Errorf("cell = %+v, want 900×540 over the 100×30 asked with", m.graphics.Cell)
	}
}

// A query that times out after a size was found keeps it: the fallback
// is for a terminal that never said.
func TestCellQueryKeepsFoundCell(t *testing.T) {
	m, _, _ := ghosttyApp(t)
	answer(m, uv.CellSizeEvent{Width: 9, Height: 18})
	answer(m, da1)
	answer(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	answer(m, cellTimeoutMsg{id: m.cells.id})
	if m.graphics.Cell != (imgcaps.Cell{Width: 9, Height: 18}) {
		t.Errorf("cell = %+v after a timeout, want the size found before", m.graphics.Cell)
	}
}

// The answers of a query that timed out, which come late, are neither
// taken for the next query's nor end it: a terminal answers in order, so
// what comes before the late DA1 is the old query's.
func TestCellQueryLateAnswers(t *testing.T) {
	m, _, _ := ghosttyApp(t)
	answer(m, cellTimeoutMsg{id: m.cells.id})
	if writes := answer(m, tea.WindowSizeMsg{Width: 120, Height: 40}); !slices.Equal(writes, []string{askCell}) {
		t.Fatalf("a resize wrote %q, want the cell asked", writes)
	}
	// The first query's answers, late.
	answer(m, uv.CellSizeEvent{Width: 20, Height: 40})
	if writes := answer(m, da1); len(writes) != 0 || m.cells.stage != cellAskCell {
		t.Fatalf("a late DA1 wrote %q, stage %d, want the query still asking", writes, m.cells.stage)
	}
	answer(m, uv.CellSizeEvent{Width: 9, Height: 18})
	answer(m, da1)
	if m.graphics.Cell != (imgcaps.Cell{Width: 9, Height: 18}) {
		t.Errorf("cell = %+v, want the second query's answer", m.graphics.Cell)
	}
}

// A query that ends after images went off doesn't ask again.
func TestCellQueryAgainOnlyWithImages(t *testing.T) {
	m, _ := newTestApp(t)
	if writes := answer(m, cellSizedMsg{cell: imgcaps.Cell{Width: 9, Height: 18}, from: cellFromCell, again: true}); len(writes) != 0 {
		t.Errorf("wrote %q without images", writes)
	}
}

// tmuxApp is a test app inside tmux, whose tmux is ft.
func tmuxApp(t *testing.T, ft *fakeTmux) (*Model, []*fakeSection) {
	t.Helper()
	m, sections := newTestApp(t)
	m.images.mode = imgcaps.ModeAuto
	WithImageProbe(imgcaps.Env{Term: "tmux-256color", Tmux: true}, ft.run)(m)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, sections
}

// Inside tmux the terminal is never asked, since tmux would hand its
// answers to the active pane, which may not be the app's: tmux says the
// size of its client's cells, on every resize too, whether or not the
// app's pane is active.
func TestTmuxCells(t *testing.T) {
	ft := &fakeTmux{passthrough: "on", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)", Cell: imgcaps.Cell{Width: 9, Height: 18}}}
	m, _ := tmuxApp(t, ft)
	if writes := answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256}); len(writes) != 0 {
		t.Fatalf("the verdict wrote %q, want nothing", writes)
	}
	if m.graphics.Cell != (imgcaps.Cell{Width: 9, Height: 18}) {
		t.Fatalf("cell = %+v, want tmux's", m.graphics.Cell)
	}
	ft.client.Cell = imgcaps.Cell{Width: 10, Height: 20}
	runs := ft.runs
	if writes := answer(m, tea.WindowSizeMsg{Width: 120, Height: 40}); len(writes) != 0 {
		t.Errorf("a resize wrote %q", writes)
	}
	if ft.runs != runs+1 || m.graphics.Cell != (imgcaps.Cell{Width: 10, Height: 20}) {
		t.Errorf("a resize ran tmux %d times, cell %+v, want once and the new size", ft.runs-runs, m.graphics.Cell)
	}
	// A terminal that gives tmux no pixels, as over ssh, keeps the size
	// found before.
	ft.client.Cell = imgcaps.Cell{}
	answer(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.graphics.Cell != (imgcaps.Cell{Width: 10, Height: 20}) {
		t.Errorf("cell = %+v, want the size found before", m.graphics.Cell)
	}
}

// Focus asks tmux one cheap question of its client, since it may have been
// attached from another terminal; only another terminal has it asked
// everything again, and a verdict that changes is told and logged.
func TestTmuxFocusRechecks(t *testing.T) {
	buf := captureLog(t)
	ft := &fakeTmux{passthrough: "on", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)", Cell: imgcaps.Cell{Width: 9, Height: 18}}}
	m, sections := tmuxApp(t, ft)
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	// The terminal record, which the first verdict is logged in.
	answer(m, terminalWaitMsg{})

	runs := ft.runs
	if writes := answer(m, tea.FocusMsg{}); len(writes) != 0 {
		t.Errorf("focus wrote %q", writes)
	}
	if ft.runs != runs+1 {
		t.Errorf("focus in the same terminal ran tmux %d times, want once", ft.runs-runs)
	}
	if n := len(records(t, buf, "images")); n != 0 {
		t.Errorf("%d images records for a verdict that held", n)
	}

	ft.client = imgcaps.TmuxClient{TTY: "/dev/pts/7", Termtype: "WezTerm 20240203"}
	runs = ft.runs
	if writes := answer(m, tea.FocusMsg{}); len(writes) != 0 {
		t.Errorf("focus in WezTerm wrote %q", writes)
	}
	if ft.runs != runs+4 {
		t.Errorf("focus in another terminal ran tmux %d times, want the client asked, then everything", ft.runs-runs)
	}
	if m.graphics.Images || m.images.verdict.Terminal != "WezTerm 20240203" {
		t.Errorf("graphics = %+v, verdict %+v after attaching from WezTerm", m.graphics, m.images.verdict)
	}
	for _, s := range sections {
		if !slices.ContainsFunc(s.msgs, func(msg tea.Msg) bool {
			g, ok := msg.(ui.GraphicsMsg)
			return ok && !g.Graphics.Images
		}) {
			t.Errorf("section %s wasn't told images went off", s.title)
		}
	}
	if recs := records(t, buf, "images"); len(recs) != 1 || recs[0]["images"] != false {
		t.Errorf("images records = %v, want one turning images off", recs)
	} else if _, ok := recs[0]["images_waited_ms"]; ok {
		t.Errorf("images record = %v, want no wait counted from startup", recs[0])
	}
	runs = ft.runs
	answer(m, tea.WindowSizeMsg{Width: 90, Height: 30})
	if ft.runs != runs {
		t.Errorf("a resize without images ran tmux %d times", ft.runs-runs)
	}

	// The same name from another terminal device is another attach.
	ft.client = imgcaps.TmuxClient{TTY: "/dev/pts/9", Termtype: "WezTerm 20240203"}
	runs = ft.runs
	answer(m, tea.FocusMsg{})
	if ft.runs != runs+4 {
		t.Errorf("focus from another device ran tmux %d times, want everything asked", ft.runs-runs)
	}

	ft.client = imgcaps.TmuxClient{TTY: "/dev/pts/3", Termtype: "ghostty 1.2.0", Cell: imgcaps.Cell{Width: 10, Height: 20}}
	if writes := answer(m, tea.FocusMsg{}); len(writes) != 0 {
		t.Errorf("focus in ghostty wrote %q", writes)
	}
	if !m.graphics.Images || !m.graphics.Tmux || m.graphics.Cell != (imgcaps.Cell{Width: 10, Height: 20}) {
		t.Errorf("graphics = %+v after attaching from ghostty", m.graphics)
	}
}

// A second focus while tmux is still being asked asks nothing more, and
// a resize meanwhile has the size of a cell asked again once the answer
// in flight came, since that answer may predate the resize.
func TestTmuxFocusWhileAsking(t *testing.T) {
	ft := &fakeTmux{passthrough: "on", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)", Cell: imgcaps.Cell{Width: 9, Height: 18}}}
	m, _ := tmuxApp(t, ft)
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	_, first := m.Update(tea.FocusMsg{})
	if first == nil {
		t.Fatal("focus asked tmux nothing")
	}
	if _, cmd := m.Update(tea.FocusMsg{}); cmd != nil {
		t.Error("a second focus asked tmux again while the first question was in flight")
	}
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40}); cmd != nil {
		t.Error("a resize asked tmux while the first question was in flight")
	}
	ft.client.Cell = imgcaps.Cell{Width: 10, Height: 20}
	runs := ft.runs
	follow(m, first)
	if ft.runs != runs+2 {
		t.Errorf("tmux ran %d times for the answer in flight, want it and the question put off", ft.runs-runs)
	}
	if m.graphics.Cell != (imgcaps.Cell{Width: 10, Height: 20}) || m.images.asking {
		t.Errorf("cell = %+v, asking %v, want the size after the resize", m.graphics.Cell, m.images.asking)
	}
}

// Setting allow-passthrough on, as :images suggests, and reloading tmux's
// config turns images on at the next focus, without a restart.
func TestTmuxPassthroughTurnedOn(t *testing.T) {
	ft := &fakeTmux{passthrough: "off", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)", Cell: imgcaps.Cell{Width: 9, Height: 18}}}
	m, _ := tmuxApp(t, ft)
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	if m.graphics.Images {
		t.Fatal("images on with passthrough off")
	}
	runs := ft.runs
	answer(m, tea.FocusMsg{})
	if ft.runs != runs+1 {
		t.Errorf("focus with nothing changed ran tmux %d times, want once", ft.runs-runs)
	}
	ft.passthrough = "on"
	answer(m, tea.FocusMsg{})
	if !m.graphics.Images || m.graphics.Cell != (imgcaps.Cell{Width: 9, Height: 18}) {
		t.Errorf("graphics = %+v after passthrough went on (%s)", m.graphics, m.images.verdict.Reason)
	}
}

// A verdict made when tmux couldn't name its client's terminal is
// compared with what tmux said then, so a focus that finds the same
// asks tmux one question, not everything.
func TestTmuxFocusUnnamedClient(t *testing.T) {
	ft := &fakeTmux{passthrough: "on", client: imgcaps.TmuxClient{TTY: "/dev/pts/1"}}
	m, _ := tmuxApp(t, ft)
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	answer(m, tea.FocusMsg{})
	runs := ft.runs
	answer(m, tea.FocusMsg{})
	if ft.runs != runs+1 {
		t.Errorf("focus ran tmux %d times, want once", ft.runs-runs)
	}
}

// Outside tmux, focus asks nothing.
func TestFocusOutsideTmux(t *testing.T) {
	m, _, _ := ghosttyApp(t)
	answer(m, cellTimeoutMsg{id: m.cells.id})
	m.images.tmux = func(context.Context, ...string) (string, error) {
		t.Error("tmux ran outside tmux")
		return "", nil
	}
	if writes := answer(m, tea.FocusMsg{}); len(writes) != 0 {
		t.Errorf("focus wrote %q", writes)
	}
}
