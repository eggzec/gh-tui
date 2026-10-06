package tui

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// later is what a timer of the app would send, once a test says time is
// up.
type later struct{ msg tea.Msg }

// avatarsApp is a test app whose avatars fetch from a fake host, whose
// timers send at once a later, and whose first section draws the avatars
// of two people whenever it is told the avatars changed, as the heads of
// comments would.
func avatarsApp(t *testing.T) (*Model, *fakeSection) {
	t.Helper()
	fetch := func(_ context.Context, src ui.ImageSource, _ ui.ImageBox) (ui.Picture, error) {
		addr := src.URL
		return ui.Picture{PNG: []byte(addr), Width: 20, Height: 20}, nil
	}
	a := ui.NewImages(context.Background(), fetch, true)
	m, sections := newTestApp(t, WithImages(a))
	m.after = func(_ time.Duration, msg tea.Msg) tea.Cmd { return func() tea.Msg { return later{msg} } }
	s := sections[0]
	s.reply = func(msg tea.Msg) tea.Cmd {
		if _, ok := msg.(ui.ImagesMsg); ok {
			a.Line("https://avatars.githubusercontent.com/u/1?v=4")
			a.Line("https://avatars.githubusercontent.com/u/2?v=4")
		}
		return nil
	}
	return m, s
}

// collect runs cmd and the commands it batches and sequences, and returns
// their messages. None of the commands it is given waits.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, collect(c)...)
		}
		return out
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
		var out []tea.Msg
		for i := range v.Len() {
			out = append(out, collect(v.Index(i).Interface().(tea.Cmd))...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// feed gives each of msgs to m, and returns what the updates wrote to the
// terminal and the messages they sent.
func feed(m *Model, msgs ...tea.Msg) (raw []string, sent []tea.Msg) {
	for _, msg := range msgs {
		_, cmd := m.Update(msg)
		for _, out := range collect(cmd) {
			if r, ok := out.(tea.RawMsg); ok {
				raw = append(raw, r.Msg.(string))
				continue
			}
			sent = append(sent, out)
		}
	}
	return raw, sent
}

func told(s *fakeSection) int {
	n := 0
	for _, msg := range s.msgs {
		if _, ok := msg.(ui.ImagesMsg); ok {
			n++
		}
	}
	return n
}

// startAvatars has the app of avatarsApp find that the terminal shows
// images, inside tmux or not, and fetch the avatars its section drew.
// It returns the arrivals.
func startAvatars(t *testing.T, m *Model, s *fakeSection, tmux bool) []tea.Msg {
	t.Helper()
	m.Update(graphicsDecidedMsg{graphics: ui.Graphics{Images: true, Tmux: tmux}})
	if told(s) != 1 {
		t.Fatalf("told the sections %d times when images began, want once", told(s))
	}
	_, arrivals := feed(m, cellSizedMsg{cell: imgcaps.Cell{Width: 10, Height: 20}, from: cellFromTmux})
	if len(arrivals) != 2 {
		t.Fatalf("%d avatars fetched, want the 2 drawn: %#v", len(arrivals), arrivals)
	}
	return arrivals
}

// The app tells the sections when avatars begin to be drawn, fetches what
// they drew once the size of a cell is known, and sends each avatar to the
// terminal as it arrives. The sections draw the avatars that arrive
// together once.
func TestAvatarsFlow(t *testing.T) {
	m, s := avatarsApp(t)
	arrivals := startAvatars(t, m, s, false)
	raw, sent := feed(m, arrivals...)
	if len(raw) != 2 {
		t.Fatalf("wrote %q, want the two avatars", raw)
	}
	for _, r := range raw {
		if !strings.HasPrefix(r, "\x1b_Ga=t,f=100,t=d,") || !strings.Contains(r, "a=p,U=1,") {
			t.Errorf("wrote %q, want an avatar and its placement", r)
		}
	}
	if told(s) != 1 {
		t.Errorf("told the sections at once, %d times", told(s)-1)
	}
	if len(sent) != 1 {
		t.Fatalf("sent %#v, want one timer for both arrivals", sent)
	}
	feed(m, sent[0].(later).msg)
	if told(s) != 2 {
		t.Errorf("told the sections %d times, want once more for both", told(s))
	}
	if m.ClearImages() == "" {
		t.Error("ClearImages deletes nothing")
	}
}

// While the terminal holds images, quitting deletes them first, while the
// alternate screen still shows, and quits a frame later. Every quit or
// interrupt meanwhile is dropped, so none ends the program before the
// deletes are written.
func TestAvatarsQuit(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  tea.Msg
	}{{"quit", tea.QuitMsg{}}, {"interrupt", tea.InterruptMsg{}}} {
		t.Run(tt.name, func(t *testing.T) {
			m, s := avatarsApp(t)
			feed(m, startAvatars(t, m, s, false)...)
			msg := m.Filter(m, tt.msg)
			if _, ok := msg.(imagesClearMsg); !ok {
				t.Fatalf("the %s passed as %T, want the images deleted first", tt.name, msg)
			}
			raw, sent := feed(m, msg)
			if len(raw) != 1 || strings.Count(raw[0], "a=d,d=I,") != 2 {
				t.Errorf("wrote %q, want both images deleted", raw)
			}
			if len(sent) != 1 {
				t.Fatalf("sent %#v, want the quit after a frame", sent)
			}
			// A second quit, as of a key pressed twice, or a signal.
			for _, again := range []tea.Msg{tea.QuitMsg{}, tea.InterruptMsg{}} {
				if got := m.Filter(m, again); got != nil {
					t.Errorf("%T while the deletes are written passed as %T", again, got)
				}
			}
			_, quit := feed(m, sent[0].(later).msg)
			if len(quit) != 1 || quit[0] != tt.msg {
				t.Fatalf("after the frame sent %#v, want the %s", quit, tt.name)
			}
			if got := m.Filter(m, quit[0]); got != tt.msg {
				t.Errorf("the %s after the deletes passed as %T", tt.name, got)
			}
			// What ends the program otherwise writes the deletes again.
			if got := m.ClearImages(); got != raw[0] {
				t.Errorf("ClearImages = %q, want the deletes again", got)
			}
		})
	}
}

// Inside tmux with allow-passthrough on, tmux drops what a pane not on
// view sends; the avatars sent while the app's pane was blurred are sent
// again when it gains focus, and only those. With all, nothing is.
func TestAvatarsSentWhileHidden(t *testing.T) {
	for _, passthrough := range []string{"on", "all"} {
		t.Run(passthrough, func(t *testing.T) {
			m, s := avatarsApp(t)
			arrivals := startAvatars(t, m, s, true)
			m.images.seen.Passthrough = passthrough
			feed(m, arrivals[0])
			feed(m, tea.BlurMsg{})
			feed(m, arrivals[1])
			raw, _ := feed(m, tea.FocusMsg{})
			want := 1
			if passthrough == "all" {
				want = 0
			}
			if n := strings.Count(strings.Join(raw, ""), "a=t,"); n != want {
				t.Errorf("focus sent %d avatars again, want %d: %q", n, want, raw)
			}
			if raw, _ := feed(m, tea.BlurMsg{}, tea.FocusMsg{}); len(raw) != 0 {
				t.Errorf("a focus after nothing was sent wrote %q", raw)
			}
		})
	}
}

// Inside tmux, the app sends the avatars again only once tmux says that
// its client is another terminal that shows images too, not on every
// focus.
func TestAvatarsResendOnAttach(t *testing.T) {
	m, s := avatarsApp(t)
	feed(m, startAvatars(t, m, s, true)...)
	if raw, _ := feed(m, tea.FocusMsg{}); len(raw) != 0 {
		t.Errorf("a focus wrote %q", raw)
	}
	raw, _ := feed(m, tmuxMovedMsg{})
	if len(raw) != 1 || strings.Count(raw[0], "a=t,") != 2 || !strings.HasPrefix(raw[0], "\x1bPtmux;") {
		t.Errorf("an attach from another terminal wrote %q, want both avatars once, through tmux", raw)
	}
}

// Without images, the sections are never told of avatars and nothing is
// fetched or deleted.
func TestAvatarsWithoutImages(t *testing.T) {
	m, s := avatarsApp(t)
	m.Update(graphicsDecidedMsg{graphics: ui.Graphics{}})
	raw, _ := feed(m, tea.FocusMsg{}, cellSizedMsg{cell: imgcaps.Cell{Width: 10, Height: 20}, from: cellFromCell})
	if told(s) != 0 || len(raw) != 0 {
		t.Errorf("told %d times, wrote %q", told(s), raw)
	}
	if _, ok := m.Filter(m, tea.QuitMsg{}).(tea.QuitMsg); !ok || m.ClearImages() != "" {
		t.Error("quitting deletes images never sent")
	}
}

// Inside tmux, a focus in the same terminal sends nothing again; an attach
// from another terminal that shows images too sends the avatars again,
// once each.
func TestTmuxAttachResendsAvatars(t *testing.T) {
	ft := &fakeTmux{passthrough: "on", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)", Cell: imgcaps.Cell{Width: 9, Height: 18}}}
	m, _ := tmuxApp(t, ft)
	m.after = func(_ time.Duration, msg tea.Msg) tea.Cmd { return func() tea.Msg { return later{msg} } }
	fetch := func(_ context.Context, src ui.ImageSource, _ ui.ImageBox) (ui.Picture, error) {
		addr := src.URL
		return ui.Picture{PNG: []byte(addr), Width: 20, Height: 20}, nil
	}
	a := ui.NewImages(context.Background(), fetch, true)
	WithImages(a)(m)
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	a.Line("https://avatars.githubusercontent.com/u/1?v=4")
	if writes := answer(m, terminalWaitMsg{}); len(writes) != 1 || !strings.Contains(writes[0], "a=t,") {
		t.Fatalf("the avatar wrote %q", writes)
	}
	if writes := answer(m, tea.FocusMsg{}); len(writes) != 0 {
		t.Errorf("focus in the same terminal wrote %q", writes)
	}
	ft.client = imgcaps.TmuxClient{TTY: "/dev/pts/3", Termtype: "ghostty 1.2.0", Cell: imgcaps.Cell{Width: 9, Height: 18}}
	writes := answer(m, tea.FocusMsg{})
	if len(writes) != 1 || strings.Count(writes[0], "a=t,") != 1 {
		t.Errorf("an attach from ghostty wrote %q, want the avatar once", writes)
	}
}

// A second terminal attached to the session turns images off and deletes
// those sent from the first, which is still attached and would keep them;
// once it detaches, images are sent again.
func TestTmuxSharedDeletesAvatars(t *testing.T) {
	ft := &fakeTmux{passthrough: "on", client: imgcaps.TmuxClient{TTY: "/dev/pts/1", Termtype: "kitty(0.43.1)", Cell: imgcaps.Cell{Width: 9, Height: 18}, Attached: 1}}
	m, _ := tmuxApp(t, ft)
	m.after = func(_ time.Duration, msg tea.Msg) tea.Cmd { return func() tea.Msg { return later{msg} } }
	fetch := func(_ context.Context, src ui.ImageSource, _ ui.ImageBox) (ui.Picture, error) {
		return ui.Picture{PNG: []byte(src.URL), Width: 20, Height: 20}, nil
	}
	a := ui.NewImages(context.Background(), fetch, true)
	WithImages(a)(m)
	answer(m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	a.Line("https://avatars.githubusercontent.com/u/1?v=4")
	if writes := answer(m, terminalWaitMsg{}); len(writes) != 1 || !strings.Contains(writes[0], "a=t,") {
		t.Fatalf("the avatar wrote %q", writes)
	}
	ft.client.Attached = 2
	writes := answer(m, tea.FocusMsg{})
	if m.graphics.Images || len(writes) != 1 || strings.Count(writes[0], "a=d,d=I") != 1 || !strings.HasPrefix(writes[0], "\x1bPtmux;") {
		t.Errorf("a second client wrote %q, images %v; want the avatar deleted through tmux", writes, m.graphics.Images)
	}
	if a.Holding() {
		t.Error("holds images after they were deleted")
	}
	ft.client.Attached = 1
	writes = answer(m, tea.FocusMsg{})
	a.Line("https://avatars.githubusercontent.com/u/1?v=4")
	writes = append(writes, answer(m, terminalWaitMsg{})...)
	if !m.graphics.Images || !slices.ContainsFunc(writes, func(w string) bool { return strings.Contains(w, "a=t,") }) {
		t.Errorf("one client left: images %v, wrote %q, want the avatar sent again", m.graphics.Images, writes)
	}
}

// The repository screen's header shows the owner's avatar before the
// repository, as its icon, in a box kept from the start, and looks as it
// does without avatars where the terminal shows no images.
func TestHeaderAvatar(t *testing.T) {
	plain := newHeaderApp(t).header
	src := &uitest.ImageHost{}
	if got := newHeaderApp(t, WithImages(uitest.Avatars(src, false))).header; got != plain {
		t.Errorf("without images the header is\n%q\nwant\n%q", got, plain)
	}

	// The terminal is found to show images once the app has started.
	a := uitest.Avatars(src, false)
	m := newHeaderApp(t, WithImages(a))
	m.after = func(_ time.Duration, msg tea.Msg) tea.Cmd { return func() tea.Msg { return later{msg} } }
	a.SetGraphics(ui.Graphics{Images: true, Cell: uitest.TestCell})
	feed(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if h := ansi.Strip(m.header); !strings.HasPrefix(h, "─    "+testRepo.String()+" ─ main ") {
		t.Errorf("header before the owner's avatar is known = %q, want a blank box before the repository", h)
	}
	if got := ansi.StringWidth(m.header); got != 80 {
		t.Errorf("header is %d wide, want 80", got)
	}
	owner := "https://avatars.githubusercontent.com/u/170000000?v=4"
	_, arrivals := feed(m, repoInfoMsg{repo: core.Repo{Ref: testRepo, DefaultBranch: "main", OwnerAvatarURL: owner}})
	_, due := feed(m, arrivals...)
	feed(m, due[0].(later).msg)
	if got := src.Asked(); len(got) != 1 || got[0] != "https://avatars.githubusercontent.com/u/170000000?s=40&v=4" {
		t.Errorf("fetched %q, want the owner's avatar", got)
	}
	if n := uitest.Placeholders(t, m.header); n != 2 {
		t.Errorf("%d placeholder cells in the header, want 2:\n%q", n, m.header)
	}
	// They survive the composition of the whole screen.
	if n := uitest.Placeholders(t, m.View().Content); n != 2 {
		t.Errorf("%d placeholder cells on screen, want the header's 2", n)
	}
	for _, w := range []int{80, 12, 4} {
		feed(m, tea.WindowSizeMsg{Width: w, Height: 24})
		if got := ansi.StringWidth(m.header); got != w {
			t.Errorf("header is %d wide, want %d", got, w)
		}
		uitest.Placeholders(t, m.header)
	}
	// Other screens name no repository and show no avatar.
	run(m, m.key(press("n")))
	if n := uitest.Placeholders(t, m.header); n != 0 {
		t.Errorf("the notifications' header shows %d placeholder cells", n)
	}
}
