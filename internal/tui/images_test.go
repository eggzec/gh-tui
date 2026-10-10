package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// probeApp is a test app whose image probe reads env, in mode, and whose
// tmux says its client runs in the terminal tmux, with cells 9×18.
func probeApp(t *testing.T, mode string, env imgcaps.Env, tmux string) (*Model, []*fakeSection) {
	t.Helper()
	m, sections := newTestApp(t)
	m.images.mode = mode
	ft := &fakeTmux{passthrough: "on", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: tmux, Cell: imgcaps.Cell{Width: 9, Height: 18}}}
	WithImageProbe(env, ft.run)(m)
	return m, sections
}

// fakeTmux is a tmux 3.4 that answers with its fields, and counts the
// times it runs.
type fakeTmux struct {
	passthrough string
	client      imgcaps.TmuxClient
	runs        int
}

func (f *fakeTmux) run(_ context.Context, args ...string) (string, error) {
	f.runs++
	q := args[len(args)-1]
	switch {
	case q == "#{version}":
		return "3.4", nil
	case q == "allow-passthrough":
		return f.passthrough, nil
	case strings.Contains(q, "session_attached") && !strings.HasPrefix(q, "#{client_tty}"):
		return strconv.Itoa(f.client.Attached), nil
	case strings.HasPrefix(q, "#{client_tty}"):
		c := f.client
		return fmt.Sprintf("%s\t%s\t%d\t%d\t%s\t%d\n", c.TTY, c.Termtype, c.Cell.Width, c.Cell.Height, f.passthrough, c.Attached), nil
	}
	return f.client.Termtype, nil
}

// answer passes msg to m, then what its command gives, as the program
// would, and returns what m wrote to the terminal meanwhile.
func answer(m *Model, msg tea.Msg) []string {
	_, cmd := m.Update(msg)
	return follow(m, cmd)
}

// follow runs cmd, passes its messages to m, and returns what they write
// to the terminal. A command that waits, a timer, isn't run: a test says
// when time is up.
func follow(m *Model, cmd tea.Cmd) []string {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(100 * time.Millisecond):
		return nil
	}
	switch msg := msg.(type) {
	case tea.BatchMsg:
		var out []string
		for _, c := range msg {
			out = append(out, follow(m, c)...)
		}
		return out
	case tea.RawMsg:
		s, _ := msg.Msg.(string)
		return []string{s}
	case nil:
		return nil
	}
	// The program asks XTGETTCAP for this message of its own.
	if fmt.Sprintf("%T", msg) == "tea.requestCapabilityMsg" {
		return []string{"XTGETTCAP " + fmt.Sprint(msg)}
	}
	return answer(m, msg)
}

func kittyReply(id uint32, payload string) uv.KittyGraphicsEvent {
	return uv.KittyGraphicsEvent{Options: kitty.Options{ID: int(id)}, Payload: []byte(payload)}
}

// da1 is a terminal's answer to DA1.
var da1 = uv.PrimaryDeviceAttributesEvent{62, 22}

func TestImagesProbe(t *testing.T) {
	t.Parallel()
	kittyEnv := imgcaps.Env{Term: "xterm-kitty"}
	plain := imgcaps.Env{Term: "xterm-256color"}
	name := func(n string) func(uint32) []tea.Msg {
		return func(uint32) []tea.Msg { return []tea.Msg{tea.TerminalVersionMsg{Name: n}, da1} }
	}
	tests := []struct {
		name    string
		mode    string
		env     imgcaps.Env
		profile colorprofile.Profile
		// first and then are the terminal's answers to the two stages,
		// given the probe's id; then is only asked when set.
		first, then func(id uint32) []tea.Msg
		tmux        string
		want        ui.Graphics
		// writes are what the app writes to the terminal, in order.
		writes []string
	}{
		{
			name: "ghostty", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first:  name("ghostty 1.2.0"),
			then:   func(id uint32) []tea.Msg { return []tea.Msg{kittyReply(id, "OK"), da1} },
			want:   ui.Graphics{Images: true},
			writes: []string{"\x1b[>q\x1b[c", "\x1b_Ga=q,i=ID", "\x1b[16t\x1b[c"},
		},
		{
			name: "konsole is never sent the query", mode: imgcaps.ModeAuto, env: plain, profile: colorprofile.TrueColor,
			first: name("Konsole 25.04.0"), writes: []string{"\x1b[>q\x1b[c"},
		},
		{
			name: "kitty, on", mode: imgcaps.ModeOn, env: kittyEnv, profile: colorprofile.TrueColor,
			first: name("kitty(0.43.1)"), want: ui.Graphics{Images: true, Animate: true}, writes: []string{"\x1b[>q\x1b[c", "\x1b[16t\x1b[c"},
		},
		{
			name: "on, only the environment", mode: imgcaps.ModeOn, env: kittyEnv, profile: colorprofile.TrueColor,
			first: func(uint32) []tea.Msg { return []tea.Msg{da1} }, writes: []string{"\x1b[>q\x1b[c"},
		},
		{
			name: "auto, only the environment, isn't sent the query", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first: func(uint32) []tea.Msg { return []tea.Msg{da1} }, writes: []string{"\x1b[>q\x1b[c"},
		},
		{
			name: "kitty without a version isn't sent the query", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first: name("kitty"), writes: []string{"\x1b[>q\x1b[c"},
		},
		{
			name: "kitty without a version, on", mode: imgcaps.ModeOn, env: kittyEnv, profile: colorprofile.TrueColor,
			first: name("kitty"), writes: []string{"\x1b[>q\x1b[c"},
		},
		{
			name: "old kitty isn't sent the query", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first: name("kitty(0.27.1)"), writes: []string{"\x1b[>q\x1b[c"},
		},
		{
			name: "kitty on 256 colors asks for 24-bit", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.ANSI256,
			first:  name("kitty(0.43.1)"),
			then:   func(id uint32) []tea.Msg { return []tea.Msg{kittyReply(id, "OK"), da1} },
			want:   ui.Graphics{Images: true, Animate: true},
			writes: []string{"\x1b[>q\x1b[c", "XTGETTCAP RGB", "XTGETTCAP Tc", "\x1b_Ga=q,i=ID", "\x1b[16t\x1b[c"},
		},
		{
			name: "konsole on 256 colors isn't asked for 24-bit", mode: imgcaps.ModeAuto, env: plain, profile: colorprofile.ANSI256,
			first: name("Konsole 25.04.0"), writes: []string{"\x1b[>q\x1b[c"},
		},
		{
			name: "another probe's answer", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first: name("kitty(0.43.1)"),
			then:  func(id uint32) []tea.Msg { return []tea.Msg{kittyReply(id+1, "OK"), da1} },
		},
		{
			name: "kitty graphics refused", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first: name("kitty(0.43.1)"),
			then:  func(id uint32) []tea.Msg { return []tea.Msg{kittyReply(id, "EINVAL:bad"), da1} },
		},
		{
			name: "timeout in the first stage", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first: func(id uint32) []tea.Msg { return []tea.Msg{probeTimeoutMsg{id: id}} },
		},
		{
			name: "timeout in the second stage", mode: imgcaps.ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor,
			first: name("kitty(0.43.1)"),
			then:  func(id uint32) []tea.Msg { return []tea.Msg{kittyReply(id, "OK"), probeTimeoutMsg{id: id}} },
		},
		{
			name: "tmux in kitty", mode: imgcaps.ModeAuto, env: imgcaps.Env{Term: "tmux-256color", Tmux: true}, profile: colorprofile.ANSI256,
			// The size of a cell comes from tmux, and the terminal is sent
			// nothing.
			tmux: "kitty(0.43.1)", want: ui.Graphics{Images: true, Animate: true, Tmux: true, Cell: imgcaps.Cell{Width: 9, Height: 18}},
			writes: []string{},
		},
		{
			name: "tmux on 16 colors is sent nothing", mode: imgcaps.ModeAuto, env: imgcaps.Env{Term: "tmux", Tmux: true}, profile: colorprofile.ANSI,
			writes: []string{},
		},
		{
			name: "tmux, off, is sent nothing", mode: imgcaps.ModeOff, env: imgcaps.Env{Term: "tmux-256color", Tmux: true}, profile: colorprofile.TrueColor,
			writes: []string{},
		},
		{
			name: "the linux console is sent nothing", mode: imgcaps.ModeAuto, env: imgcaps.Env{Term: "linux"}, profile: colorprofile.ANSI256,
			writes: []string{},
		},
		{
			name: "a dumb terminal is sent nothing", mode: imgcaps.ModeAuto, env: imgcaps.Env{Term: "dumb"}, profile: colorprofile.TrueColor,
			writes: []string{},
		},
		{
			name: "off isn't asked for 24-bit", mode: imgcaps.ModeOff, env: plain, profile: colorprofile.ANSI256,
			first: name("ghostty 1.2.0"), writes: []string{"\x1b[>q"},
		},
		{
			name: "screen is sent nothing", mode: imgcaps.ModeAuto, env: imgcaps.Env{Term: "screen-256color", Screen: true}, profile: colorprofile.ANSI256,
			writes: []string{},
		},
		{name: "off", mode: imgcaps.ModeOff, env: kittyEnv, profile: colorprofile.TrueColor, writes: []string{"\x1b[>q"}},
		{name: "NO_COLOR", mode: imgcaps.ModeAuto, env: imgcaps.Env{Term: "xterm-kitty", NoColor: true}, profile: colorprofile.Ascii, writes: []string{"\x1b[>q"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, sections := probeApp(t, tt.mode, tt.env, tt.tmux)
			writes := answer(m, tea.ColorProfileMsg{Profile: tt.profile})
			id := m.images.id
			if tt.first != nil {
				for _, msg := range tt.first(id) {
					writes = append(writes, answer(m, msg)...)
				}
			}
			if tt.then != nil {
				for _, msg := range tt.then(id) {
					writes = append(writes, answer(m, msg)...)
				}
			}
			if !m.images.done {
				t.Fatal("no verdict")
			}
			if m.graphics != tt.want {
				t.Errorf("graphics = %+v, want %+v (%s)", m.graphics, tt.want, m.images.verdict.Reason)
			}
			if tt.writes != nil {
				got := make([]string, len(writes))
				for i, w := range writes {
					got[i] = strings.ReplaceAll(w, "i="+strconv.FormatUint(uint64(id), 10), "i=ID")
				}
				if len(got) != len(tt.writes) {
					t.Fatalf("writes = %q, want %d starting %q", got, len(tt.writes), tt.writes)
				}
				for i := range got {
					if !strings.HasPrefix(got[i], tt.writes[i]) {
						t.Errorf("write %d = %q, want it to start %q", i, got[i], tt.writes[i])
					}
				}
			}
			// The sections are told.
			for _, s := range sections {
				if !slices.ContainsFunc(s.msgs, func(msg tea.Msg) bool {
					g, ok := msg.(ui.GraphicsMsg)
					return ok && g.Graphics == tt.want
				}) {
					t.Errorf("section %s wasn't told the graphics", s.title)
				}
			}
		})
	}
}

// A DA1 before the probe starts is dropped, and so is a second color
// profile that changes nothing; one that lets images be drawn where the
// first didn't starts the probe again.
func TestImagesProbeProfile(t *testing.T) {
	t.Parallel()
	m, _ := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-kitty"}, "")
	if writes := answer(m, da1); len(writes) != 0 || m.images.done {
		t.Fatalf("a DA1 before the probe wrote %q, done %v", writes, m.images.done)
	}
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI})
	if !m.images.done || m.graphics.Images {
		t.Fatalf("16 colors: done %v, graphics %+v, want no images", m.images.done, m.graphics)
	}
	if writes := answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI}); len(writes) != 0 {
		t.Errorf("the same profile again wrote %q", writes)
	}
	writes := answer(m, tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	if len(writes) != 1 || writes[0] != "\x1b[>q\x1b[c" || m.images.done {
		t.Fatalf("a better profile wrote %q, done %v, want the probe started again", writes, m.images.done)
	}
	answer(m, tea.TerminalVersionMsg{Name: "kitty(0.43.1)"})
	answer(m, da1)
	answer(m, kittyReply(m.images.id, "OK"))
	answer(m, da1)
	if !m.graphics.Images {
		t.Errorf("graphics = %+v after the probe, want images (%s)", m.graphics, m.images.verdict.Reason)
	}
	if writes := answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256}); len(writes) != 0 || !m.graphics.Images {
		t.Errorf("a profile after the verdict wrote %q, graphics %+v", writes, m.graphics)
	}
}

// Without WithImageProbe, as in every other test, the app reads no
// environment, runs no tmux and sends the terminal nothing, and draws no
// images.
func TestImagesNoProbe(t *testing.T) {
	t.Parallel()
	m, sections := newTestApp(t)
	m.images.tmux = func(context.Context, ...string) (string, error) {
		t.Error("tmux ran without WithImageProbe")
		return "", nil
	}
	if writes := answer(m, tea.ColorProfileMsg{Profile: colorprofile.TrueColor}); len(writes) != 0 {
		t.Errorf("wrote %q without WithImageProbe", writes)
	}
	if writes := answer(m, tea.TerminalVersionMsg{Name: "kitty(0.43.1)"}); len(writes) != 0 {
		t.Errorf("XTVERSION's answer wrote %q without WithImageProbe", writes)
	}
	if !m.images.done || m.graphics.Images {
		t.Errorf("done %v, graphics %+v, want a verdict without images", m.images.done, m.graphics)
	}
	for _, s := range sections {
		if !slices.ContainsFunc(s.msgs, func(msg tea.Msg) bool { _, ok := msg.(ui.GraphicsMsg); return ok }) {
			t.Errorf("section %s wasn't told the graphics", s.title)
		}
	}
}

// A terminal known to have 24-bit color whose profile said 16 colors,
// as over ssh without COLORTERM, is asked to say so, and the better
// profile it gets starts the probe again, which can turn images on.
// NO_COLOR keeps its profile.
func TestImagesProbeUpgradesProfile(t *testing.T) {
	t.Parallel()
	m, _ := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-256color"}, "")
	if writes := answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI}); !slices.Equal(writes, []string{ansi.RequestNameVersion}) {
		t.Fatalf("16 colors wrote %q, want XTVERSION alone", writes)
	}
	if m.graphics.Images || !m.images.done {
		t.Fatalf("16 colors: graphics %+v, done %v", m.graphics, m.images.done)
	}
	writes := answer(m, tea.TerminalVersionMsg{Name: "ghostty 1.2.0"})
	if !slices.Equal(writes, []string{"XTGETTCAP RGB", "XTGETTCAP Tc"}) {
		t.Fatalf("ghostty on 16 colors wrote %q, want RGB and Tc asked", writes)
	}
	// Asked once, though the terminal names itself again.
	if writes := answer(m, tea.TerminalVersionMsg{Name: "ghostty 1.2.0"}); len(writes) != 0 {
		t.Errorf("a second XTVERSION answer wrote %q", writes)
	}
	// The program raises the profile on the answer.
	if writes := answer(m, tea.ColorProfileMsg{Profile: colorprofile.TrueColor}); !slices.Equal(writes, []string{"\x1b[>q\x1b[c"}) {
		t.Fatalf("the raised profile wrote %q, want the probe started again", writes)
	}
	answer(m, tea.TerminalVersionMsg{Name: "ghostty 1.2.0"})
	answer(m, da1)
	answer(m, kittyReply(m.images.id, "OK"))
	answer(m, da1)
	if !m.graphics.Images {
		t.Errorf("graphics = %+v after the raised profile, want images (%s)", m.graphics, m.images.verdict.Reason)
	}

	m, _ = probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-ghostty", NoColor: true}, "")
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ASCII})
	if writes := answer(m, tea.TerminalVersionMsg{Name: "ghostty 1.2.0"}); len(writes) != 0 {
		t.Errorf("NO_COLOR: XTVERSION's answer wrote %q, want nothing", writes)
	}
}

// An answer that comes after the verdict changes nothing, and no answer,
// early or late, whole or in pieces, reaches the app as keys: the reader
// decodes each one into an event of its own, which the probe drops.
func TestLateAnswers(t *testing.T) {
	t.Parallel()
	m, sections := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-kitty"}, "")
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	answer(m, probeTimeoutMsg{id: m.images.id})
	if m.graphics.Images || !m.images.done {
		t.Fatalf("graphics = %+v after the timeout, want none", m.graphics)
	}

	input := "\x1b_Gi=" + strconv.FormatUint(uint64(m.images.id), 10) + ";OK\x1b\\" + "\x1bP>|kitty(0.43.1)\x1b\\" + "\x1b[?62;22c"
	// Each answer is split in two reads, as a slow link may deliver it.
	reads := []string{input[:5], input[5:20], input[20:31], input[31:40], input[40:]}
	events := readEvents(t, reads)
	if len(events) == 0 {
		t.Fatal("no events")
	}
	for _, ev := range events {
		switch ev.(type) {
		case uv.KeyPressEvent, uv.KeyReleaseEvent:
			t.Fatalf("an answer decoded as a key: %#v, from %q", ev, reads)
		}
		if writes := answer(m, ev); len(writes) != 0 {
			t.Errorf("a late %T wrote %q", ev, writes)
		}
	}
	if m.graphics.Images {
		t.Error("a late answer turned images on")
	}
	for _, s := range sections {
		for _, msg := range s.msgs {
			if _, ok := msg.(tea.KeyPressMsg); ok {
				t.Errorf("section %s got a key: %v", s.title, msg)
			}
		}
	}
}

// readEvents decodes reads, bytes the terminal sends in pieces, a few
// milliseconds apart, with the reader the program reads input with.
func readEvents(t *testing.T, reads []string) []uv.Event {
	t.Helper()
	pr, pw := io.Pipe()
	r := uv.NewTerminalReader(pr, "xterm-kitty")
	// Well above the gaps, so a loaded machine doesn't split them.
	r.EscTimeout = 500 * time.Millisecond
	events := make(chan uv.Event, 64)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = r.StreamEvents(ctx, events) }()
	for _, part := range reads {
		if _, err := io.WriteString(pw, part); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	var out []uv.Event
	for {
		select {
		case ev := <-events:
			out = append(out, ev)
			if _, ok := ev.(uv.PrimaryDeviceAttributesEvent); ok {
				return out
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("events so far %#v, want DA1 last", out)
		}
	}
}

// The app asks XTVERSION once, in the probe, and the terminal record
// carries the verdict, once both came.
func TestTerminalRecordHasImages(t *testing.T) {
	// Not parallel: it replaces the default logger, which the whole process shares.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	m, _ := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-ghostty"}, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	msgs := []tea.Msg{
		tea.ColorProfileMsg{Profile: colorprofile.TrueColor}, tea.TerminalVersionMsg{Name: "ghostty 1.2.0"}, da1,
		kittyReply(m.images.id, "OK"),
	}
	writes := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		writes = append(writes, answer(m, msg)...)
	}
	if n := strings.Count(strings.Join(writes, ""), ansi.RequestNameVersion); n != 1 {
		t.Errorf("XTVERSION asked %d times in %q, want once", n, writes)
	}
	if strings.Contains(buf.String(), `"terminal"`) {
		t.Fatalf("terminal record before the verdict:\n%s", buf.String())
	}
	answer(m, da1)
	var rec map[string]any
	for line := range strings.Lines(buf.String()) {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if r["msg"] == "terminal" {
			rec = r
		}
	}
	if rec == nil || rec["images"] != true || rec["xtversion"] != "ghostty 1.2.0" || rec["images_mode"] != "auto" {
		t.Errorf("terminal record = %v, want the verdict in it", rec)
	}
}

// records returns the records logged to buf with msg name.
func records(t *testing.T, buf *bytes.Buffer, name string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if r["msg"] == name {
			out = append(out, r)
		}
	}
	return out
}

// captureLog sends slog's records to a buffer for the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// Through tmux the terminal record says so, names the terminal of tmux's
// client as tmux does, and how long tmux took.
func TestTerminalRecordThroughTmux(t *testing.T) {
	// Not parallel: it replaces the default logger, which the whole process shares.
	buf := captureLog(t)
	m, _ := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "tmux-256color", Tmux: true}, "kitty(0.43.1)")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	answer(m, terminalWaitMsg{})
	recs := records(t, buf, "terminal")
	if len(recs) != 1 {
		t.Fatalf("terminal records = %v, want one", recs)
	}
	rec := recs[0]
	if rec["images"] != true || rec["images_tmux"] != true || rec["client_termtype"] != "kitty(0.43.1)" {
		t.Errorf("terminal record = %v, want images through tmux in kitty", rec)
	}
	if _, ok := rec["images_waited_ms"].(float64); !ok {
		t.Errorf("terminal record = %v, want images_waited_ms", rec)
	}
	if _, ok := rec["xtversion"]; ok {
		t.Errorf("terminal record = %v, want no xtversion through tmux", rec)
	}
}

// A verdict that comes after the terminal record, as one does when a
// better profile starts the probe again, is logged as a record of its
// own; the first verdict, decided by the profile alone, waited for
// nothing.
func TestImagesRecordAfterTerminal(t *testing.T) {
	// Not parallel: it replaces the default logger, which the whole process shares.
	buf := captureLog(t)
	m, _ := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-256color"}, "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI})
	answer(m, tea.TerminalVersionMsg{Name: "ghostty 1.2.0"})
	recs := records(t, buf, "terminal")
	if len(recs) != 1 || recs[0]["images"] != false {
		t.Fatalf("terminal records = %v, want one without images", recs)
	}
	if _, ok := recs[0]["images_waited_ms"]; ok {
		t.Errorf("terminal record = %v, want no images_waited_ms for a verdict that asked nothing", recs[0])
	}
	if len(records(t, buf, "images")) != 0 {
		t.Fatal("an images record before the second verdict")
	}
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	answer(m, tea.TerminalVersionMsg{Name: "ghostty 1.2.0"})
	answer(m, da1)
	answer(m, kittyReply(m.images.id, "OK"))
	answer(m, da1)
	if n := len(records(t, buf, "terminal")); n != 1 {
		t.Errorf("%d terminal records, want one", n)
	}
	imgs := records(t, buf, "images")
	if len(imgs) != 1 {
		t.Fatalf("images records = %v, want one", imgs)
	}
	if r := imgs[0]; r["images"] != true || r["span"] != "tui" || r["images_mode"] != "auto" {
		t.Errorf("images record = %v, want images on, span tui", r)
	}
	if _, ok := imgs[0]["images_waited_ms"].(float64); !ok {
		t.Errorf("images record = %v, want images_waited_ms", imgs[0])
	}
}

// The images command says what the startup check found, and changes
// nothing.
func TestImagesCommand(t *testing.T) {
	t.Parallel()
	passthroughOff := &fakeTmux{passthrough: "off", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)"}}
	tests := []struct {
		name  string
		setup func(t *testing.T) *Model
		want  []string
		not   []string
	}{
		{
			name: "ghostty",
			setup: func(t *testing.T) *Model {
				t.Helper()
				m, _, _ := ghosttyApp(t)
				answer(m, uv.CellSizeEvent{Width: 9, Height: 18})
				answer(m, da1)
				return m
			},
			want: []string{"Images are on.", "Reason: the terminal draws kitty placeholders.", "Terminal: ghostty 1.2.0\n",
				"Through tmux: no\n", "images.enabled: auto\n", "Cell size: 9×18 pixels\n"},
			not: []string{"tmux.conf"},
		},
		{
			name: "tmux without passthrough",
			setup: func(t *testing.T) *Model {
				t.Helper()
				m, _ := newTestApp(t)
				m.images.mode = imgcaps.ModeAuto
				WithImageProbe(imgcaps.Env{Term: "tmux-256color", Tmux: true}, passthroughOff.run)(m)
				answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
				return m
			},
			want: []string{"Images are off.", "Reason: tmux's allow-passthrough is off.", "Terminal: kitty(0.43.1)\n",
				"Through tmux: yes\n", "images.enabled: auto\n", "To draw them, add this line to tmux.conf:", "7 set -g allow-passthrough on\n",
				"tmux source-file ~/.tmux.conf"},
			not: []string{"Cell size:"},
		},
		{
			name: "off",
			setup: func(t *testing.T) *Model {
				t.Helper()
				m, _ := probeApp(t, imgcaps.ModeOff, imgcaps.Env{Term: "xterm-kitty"}, "")
				answer(m, tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
				answer(m, tea.TerminalVersionMsg{Name: "kitty(0.43.1)"})
				return m
			},
			want: []string{"Images are off.", "Reason: images are turned off in the settings.", "Terminal: kitty(0.43.1)\n", "images.enabled: off\n"},
			not:  []string{"tmux.conf"},
		},
		{
			name: "before the verdict",
			setup: func(t *testing.T) *Model {
				t.Helper()
				m, _ := probeApp(t, imgcaps.ModeAuto, imgcaps.Env{Term: "xterm-kitty"}, "")
				return m
			},
			want: []string{"Images are off until the terminal answers.", "Terminal: unknown\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.setup(t)
			graphics, verdict := m.graphics, m.images.verdict
			runCommand(t, m, "images")
			title, got := shownText(t, m)
			got = trimLines(got)
			if title != "Images" {
				t.Errorf("title %q, want Images", title)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("pager = %q, want %q in it", got, w)
				}
			}
			for _, w := range tt.not {
				if strings.Contains(got, w) {
					t.Errorf("pager = %q, want no %q", got, w)
				}
			}
			if m.graphics != graphics || m.images.verdict != verdict {
				t.Errorf("the command changed graphics %+v to %+v, verdict %+v to %+v", graphics, m.graphics, verdict, m.images.verdict)
			}
			drive(m, m.key(press("q")))
			if m.modal != nil {
				t.Errorf("q left %T open, want the pager closed", m.modal)
			}
		})
	}
}

// The images command takes no argument.
func TestImagesCommandTakesNoArgument(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	runCommand(t, m, "images now")
	if got := toasted(m); !strings.Contains(got, "The images command takes no argument.") {
		t.Errorf("toast = %q", got)
	}
}

// trimLines drops the spaces the pager pads its lines with, and joins
// the rows it wrapped a line to, which it indents by two columns.
func trimLines(s string) string {
	var lines []string
	for l := range strings.SplitSeq(s, "\n") {
		l = strings.TrimRight(l, " ")
		if rest, ok := strings.CutPrefix(l, "  "); ok && len(lines) > 0 {
			lines[len(lines)-1] += rest
			continue
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}

// At a narrow width the images pager wraps its lines rather than cut
// them, so the line to add to tmux.conf shows whole.
func TestImagesCommandNarrow(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	m.images.mode = imgcaps.ModeAuto
	ft := &fakeTmux{passthrough: "off", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)"}}
	WithImageProbe(imgcaps.Env{Term: "tmux-256color", Tmux: true}, ft.run)(m)
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	runCommand(t, m, "images")
	_, text := shownText(t, m)
	if !m.modal.(*textModal).pager.Wrap() {
		t.Error("the images pager doesn't wrap")
	}
	text = trimLines(text)
	for _, want := range []string{"\n7 set -g allow-passthrough on\n", "~/.tmux.conf, and come back to gh-tui."} {
		if !strings.Contains(text, want) {
			t.Errorf("pager at 40 columns = %q, want %q whole", text, want)
		}
	}
	drive(m, m.key(press("q")))
	runCommand(t, m, "config")
	if m.modal.(*textModal).pager.Wrap() {
		t.Error("the config pager wraps, want its lines scrolled sideways")
	}
}
