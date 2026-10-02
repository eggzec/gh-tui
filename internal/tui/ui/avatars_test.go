package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/pkg/termimg"
)

var testCell = imgcaps.Cell{Width: 10, Height: 20}

// fakeFetch serves each address as a PNG of its own name, or fails the
// addresses in fail with their error, and records the boxes asked for.
type fakeFetch struct {
	fail  map[string]error
	boxes []ImageBox
	urls  []string
}

func (f *fakeFetch) fetch(_ context.Context, addr string, box ImageBox) (Picture, error) {
	f.boxes, f.urls = append(f.boxes, box), append(f.urls, addr)
	if err := f.fail[addr]; err != nil {
		return Picture{}, err
	}
	return Picture{PNG: []byte(addr), Width: 20, Height: 20}, nil
}

func newTestAvatars(f *fakeFetch, g Graphics) *Avatars {
	a := NewAvatars(context.Background(), f.fetch, true)
	a.SetGraphics(g)
	return a
}

// load ends an update as the app does: it runs what a.Load returns and
// hands the avatars that arrive to a. It returns what was written to the
// terminal and the most urgent drawing asked for.
func load(t *testing.T, a *Avatars) (raw string, redraw Redraw) {
	t.Helper()
	var b strings.Builder
	var run func(cmd tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				run(c)
			}
		case tea.RawMsg:
			b.WriteString(msg.Msg.(string))
		default:
			sent, r, handled := a.Update(msg)
			if !handled {
				t.Fatalf("Update didn't take %T", msg)
			}
			redraw = max(redraw, r)
			run(sent)
		}
	}
	cmd, r := a.Load()
	redraw = r
	run(cmd)
	return b.String(), redraw
}

func rawOf(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	if cmd == nil {
		return ""
	}
	msg, ok := cmd().(tea.RawMsg)
	if !ok {
		t.Fatalf("command returned %T, not a raw sequence", cmd())
	}
	return msg.Msg.(string)
}

// avatarOf is the address GitHub gives for login's avatar.
func avatarOf(login string) string {
	return "https://avatars.githubusercontent.com/u/" + login + "?v=4"
}

// Where the terminal shows no images, or the config wants no avatars,
// an avatar takes no cells and nothing is fetched.
func TestAvatarsNotShown(t *testing.T) {
	f := &fakeFetch{}
	for name, a := range map[string]*Avatars{
		"no images":  newTestAvatars(f, Graphics{Cell: testCell}),
		"config off": NewAvatars(context.Background(), f.fetch, false),
		"no fetch":   NewAvatars(context.Background(), nil, true),
		"nil":        nil,
	} {
		t.Run(name, func(t *testing.T) {
			if a != nil {
				a.SetGraphics(Graphics{Images: name != "no images", Cell: testCell})
			}
			if got := a.Line(avatarOf("mona")); got != "" {
				t.Errorf("Line = %q, want nothing", got)
			}
			if got := a.Box(avatarOf("mona"), AvatarLarge); got != nil {
				t.Errorf("Box = %q, want nothing", got)
			}
			if cmd, r := a.Load(); cmd != nil || r != RedrawNone {
				t.Error("Load fetches")
			}
			if a.Close() != "" || a.Holding() {
				t.Error("holds images never sent")
			}
		})
	}
	if len(f.urls) != 0 {
		t.Errorf("fetched %q", f.urls)
	}
}

// An avatar keeps a blank box of its size until it arrives, then shows in
// placeholder cells of the same size, after it was sent out of band.
func TestAvatarArrives(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true})
	addr := SizedAvatar(avatarOf("mona"), AvatarLarge)
	box := a.Box(addr, AvatarLarge)
	if len(box) != 3 || box[0] != "      " || box[2] != "      " {
		t.Fatalf("box before the avatar arrived = %q, want 3 blank lines of 6", box)
	}
	if cmd, _ := a.Load(); cmd != nil {
		t.Fatal("Load fetched before the size of a cell was known")
	}
	if a.SetGraphics(Graphics{Images: true, Cell: testCell}) {
		t.Error("a new cell size changed what is drawn")
	}
	raw, redraw := load(t, a)
	if redraw != RedrawSoon {
		t.Errorf("the avatar arrived with redraw %d, want soon", redraw)
	}
	if len(f.boxes) != 1 || f.boxes[0] != (ImageBox{Cols: 6, Rows: 3, Cell: testCell}) {
		t.Errorf("fetched boxes %+v, want one of 6×3 cells of %v", f.boxes, testCell)
	}
	got := a.Box(addr, AvatarLarge)
	if len(got) != 3 {
		t.Fatalf("%d lines, want 3", len(got))
	}
	id := placeholderID(t, got[0])
	want := termimg.Transmit(id, []byte(addr), 20, 20) + termimg.Place(id, 6, 3)
	if raw != want {
		t.Errorf("sent\n%q\nwant\n%q", raw, want)
	}
	for i, l := range termimg.Rows(id, 6, 3) {
		if got[i] != l {
			t.Errorf("line %d = %q, want %q", i, got[i], l)
		}
	}
	// Drawn again, it is fetched no more.
	a.Box(addr, AvatarLarge)
	if raw, _ := load(t, a); raw != "" || len(f.urls) != 1 {
		t.Errorf("drawn again, fetched %q and sent %q", f.urls, raw)
	}
}

// placeholderID returns the ID that line, a row of placeholders, names.
func placeholderID(t *testing.T, line string) termimg.ID {
	t.Helper()
	var color int
	rest, ok := strings.CutPrefix(line, "\x1b[38;5;")
	if !ok {
		t.Fatalf("not a row of placeholders: %q", line)
	}
	for rest != "" && rest[0] != 'm' {
		color = color*10 + int(rest[0]-'0')
		rest = rest[1:]
	}
	_, _, msb, _, ok := termimg.Cell(rest[1:])
	if !ok {
		t.Fatalf("not a row of placeholders: %q", line)
	}
	return termimg.NewID(msb, uint8(color))
}

func TestAvatarLine(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	if got := a.Line(avatarOf("mona")); got != "   " {
		t.Errorf("Line before it arrived = %q, want a blank box and a space", got)
	}
	load(t, a)
	if len(f.urls) != 1 || f.urls[0] != "https://avatars.githubusercontent.com/u/mona?s=40&v=4" {
		t.Errorf("fetched %q, want the address asking for the box's pixels", f.urls)
	}
	got := a.Line(avatarOf("mona"))
	if id := placeholderID(t, got); got != termimg.Rows(id, 2, 1)[0]+" " {
		t.Errorf("Line = %q", got)
	}
	// A deleted account has no avatar, but keeps the box, so its line
	// lines up with the others.
	if got := a.Line(""); got != "   " {
		t.Errorf("Line of no address = %q", got)
	}
}

// A failed avatar keeps its blank box; one that may mend is asked for
// again once GitHub answers again, and one the host doesn't have is not.
func TestAvatarFails(t *testing.T) {
	flaky, gone := avatarOf("flaky"), avatarOf("gone")
	f := &fakeFetch{fail: map[string]error{
		SizedAvatar(flaky, AvatarSmall): errors.New("connection reset"),
		SizedAvatar(gone, AvatarSmall):  fmt.Errorf("%w: 404", ErrImageGone),
	}}
	a := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	a.Line(flaky)
	a.Line(gone)
	if raw, redraw := load(t, a); raw != "" || redraw != RedrawNone {
		t.Errorf("failures sent %q, redraw %d", raw, redraw)
	}
	if got := a.Line(flaky); got != "   " {
		t.Errorf("Line of a failed avatar = %q", got)
	}
	load(t, a)
	if len(f.urls) != 2 {
		t.Errorf("failed avatars were fetched again at once: %q", f.urls)
	}
	if !a.Online() {
		t.Error("Online forgot no failure")
	}
	f.fail = nil
	a.Line(flaky)
	a.Line(gone)
	if _, redraw := load(t, a); redraw == RedrawNone || len(f.urls) != 3 || f.urls[2] != SizedAvatar(flaky, AvatarSmall) {
		t.Errorf("after Online: redraw %d, fetched %q; want only the one that may mend again", redraw, f.urls)
	}
	if a.Online() {
		t.Error("Online forgot an avatar the host doesn't have")
	}
}

// While GitHub can't be reached nothing is fetched, and what was drawn
// meanwhile is once it can.
func TestAvatarsOffline(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	a.SetOffline(true)
	a.Line(avatarOf("mona"))
	if cmd, _ := a.Load(); cmd != nil || len(f.urls) != 0 {
		t.Fatal("fetched while offline")
	}
	a.SetOffline(false)
	if _, redraw := load(t, a); redraw != RedrawSoon || len(f.urls) != 1 {
		t.Errorf("online again: redraw %d, fetched %q", redraw, f.urls)
	}
}

// Inside tmux every sequence goes through its passthrough.
func TestAvatarsTmux(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Tmux: true, Cell: testCell})
	a.Line(avatarOf("mona"))
	raw, _ := load(t, a)
	if !strings.HasPrefix(raw, "\x1bPtmux;") || strings.Count(raw, "\x1bPtmux;") != 2 {
		t.Errorf("sent %q, want the transmission and the placement each wrapped", raw)
	}
	if got := rawOf(t, a.Resend()); got != raw {
		t.Errorf("Resend sent %q, want %q again", got, raw)
	}
	if got := a.Close(); !strings.HasPrefix(got, "\x1bPtmux;") || !strings.Contains(got, "a=d,d=I") {
		t.Errorf("Close = %q", got)
	}
}

// Close deletes every image sent, and sends none after; a terminal that
// begins to show images again, such as one tmux is attached from anew,
// is sent them again.
func TestAvatarsCloseAndResend(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	a.Line(avatarOf("mona"))
	a.Line(avatarOf("hubot"))
	load(t, a)

	if !a.SetGraphics(Graphics{Cell: testCell}) {
		t.Error("images off: no change")
	}
	if !a.SetGraphics(Graphics{Images: true, Cell: testCell}) {
		t.Error("images on again: no change")
	}
	raw, redraw := load(t, a)
	if n := strings.Count(raw, "a=t,"); n != 2 || redraw != RedrawSoon {
		t.Errorf("sent %d avatars again (redraw %d), want 2: %q", n, redraw, raw)
	}
	if len(f.urls) != 2 {
		t.Errorf("sending again fetched again: %q", f.urls)
	}

	ids := []termimg.ID{placeholderID(t, a.Line(avatarOf("hubot"))), placeholderID(t, a.Line(avatarOf("mona")))}
	if !a.Holding() {
		t.Error("holds no images")
	}
	got := a.Close()
	if strings.Count(got, "a=d,d=I") != 2 || !strings.Contains(got, termimg.Delete(ids[0])) || !strings.Contains(got, termimg.Delete(ids[1])) {
		t.Errorf("Close = %q, want both deleted", got)
	}
	if a.Holding() {
		t.Error("holds images after Close")
	}
	if again := a.Close(); again != got {
		t.Errorf("Close again = %q, want the same deletes, in case they never arrived", again)
	}
	a.Line(avatarOf("octocat"))
	if raw, _ := load(t, a); raw != "" || a.Resend() != nil {
		t.Errorf("sent %q after Close", raw)
	}
}

// Once the terminal holds as many avatars as the pool has IDs, a new one
// takes the ID of the one drawn least recently, deleting its image, and
// has the views draw at once so its old cells go blank; the one taken from
// is fetched again if drawn again.
func TestAvatarsTakeTheLeastDrawn(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	for i := range maxAvatars {
		a.Line(avatarOf(fmt.Sprint("old", i)))
		load(t, a)
	}
	// All but old0 are drawn again, later.
	for i := 1; i < maxAvatars; i++ {
		a.Line(avatarOf(fmt.Sprint("old", i)))
	}
	load(t, a)
	victim := placeholderID(t, a.Line(avatarOf("old0")))
	load(t, a)
	for i := 1; i < maxAvatars; i++ {
		a.Line(avatarOf(fmt.Sprint("old", i)))
	}
	a.Line(avatarOf("new"))
	raw, redraw := load(t, a)
	if redraw != RedrawNow {
		t.Errorf("redraw %d, want now", redraw)
	}
	if !strings.HasPrefix(raw, termimg.Delete(victim)) || !strings.Contains(raw, "i="+strconv.FormatUint(uint64(victim), 10)+",") {
		t.Errorf("sent %.80q…, want old0's image deleted and the new one under its ID", raw)
	}
	if got := a.Line(avatarOf("new")); placeholderID(t, got) != victim {
		t.Errorf("the new avatar has another ID")
	}
	if got := a.Line(avatarOf("old0")); got != "   " {
		t.Errorf("the avatar taken from = %q, want a blank box", got)
	}
	if a.held() != maxAvatars {
		t.Errorf("holds %d, want %d", a.held(), maxAvatars)
	}
}

// A view with more authors than the pool has IDs, drawn again whenever
// avatars change, settles: those that don't fit stay blank, and nothing
// is fetched or sent again and again.
func TestAvatarsPastThePool(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	header := avatarOf("owner")
	drawThread := func() (shown int) {
		for i := range 300 {
			if strings.HasPrefix(a.Line(avatarOf(fmt.Sprint("author", i))), "\x1b[") {
				shown++
			}
		}
		return shown
	}
	// A header, drawn on its own first, then a thread.
	a.Line(header)
	load(t, a)
	drawThread()
	rounds, sends := 0, 0
	for {
		raw, redraw := load(t, a)
		sends += strings.Count(raw, "a=t,")
		if redraw == RedrawNone {
			break
		}
		rounds++
		if rounds > 5 {
			t.Fatalf("still changing after %d rounds: fetched %d, sent %d", rounds, len(f.urls), sends)
		}
		// Every view draws again in the update that follows.
		a.Line(header)
		drawThread()
	}
	if len(f.urls) > 301+1 {
		t.Errorf("fetched %d, want each avatar about once", len(f.urls))
	}
	if sends > maxAvatars+1 {
		t.Errorf("sent %d images, want at most the pool's %d and one taken back", sends, maxAvatars)
	}
	shown := drawThread()
	if a.held() != maxAvatars || shown == 0 || shown > maxAvatars {
		t.Errorf("holds %d, thread shows %d", a.held(), shown)
	}
	// Drawn again, the thread fetches and sends nothing more.
	before := len(f.urls)
	if raw, redraw := load(t, a); raw != "" || redraw != RedrawNone || len(f.urls) != before {
		t.Errorf("drawn again: sent %d, redraw %d, fetched %d more", strings.Count(raw, "a=t,"), redraw, len(f.urls)-before)
	}
	// Re-sending is bounded by the pool.
	if n := strings.Count(rawOf(t, a.Resend()), "a=t,"); n != maxAvatars {
		t.Errorf("Resend sent %d, want %d", n, maxAvatars)
	}
}

// An avatar that arrives for avatars no longer drawn, or of another
// instance, is told apart.
func TestAvatarsUpdateOthers(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	b := newTestAvatars(f, Graphics{Images: true, Cell: testCell})
	a.Line(avatarOf("mona"))
	cmd, _ := a.Load()
	msg := cmd()
	if _, _, handled := b.Update(msg); handled {
		t.Error("another instance took the avatar")
	}
	if _, _, handled := a.Update(tea.KeyPressMsg{}); handled {
		t.Error("took a key")
	}
	if _, redraw, handled := a.Update(msg); !handled || redraw != RedrawSoon {
		t.Errorf("handled %v, redraw %d", handled, redraw)
	}
}

func TestSizedAvatar(t *testing.T) {
	tests := []struct {
		raw  string
		size AvatarSize
		want string
	}{
		{"https://avatars.githubusercontent.com/u/1?v=4", AvatarLarge, "https://avatars.githubusercontent.com/u/1?s=120&v=4"},
		{"https://avatars.githubusercontent.com/in/29110?v=4", AvatarSmall, "https://avatars.githubusercontent.com/in/29110?s=40&v=4"},
		{"https://ghe.example.com/avatars/u/3?", AvatarSmall, "https://ghe.example.com/avatars/u/3?s=40"},
		{"https://avatars.ghe.example.com/u/3?s=8", AvatarSmall, "https://avatars.ghe.example.com/u/3?s=40"},
		{"", AvatarSmall, ""},
	}
	for _, tt := range tests {
		if got := SizedAvatar(tt.raw, tt.size); got != tt.want {
			t.Errorf("SizedAvatar(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

// What is sent between Hide and Show is sent again at Show, and only that.
func TestAvatarsHideAndShow(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true, Tmux: true, Cell: testCell})
	a.Line(avatarOf("mona"))
	load(t, a)
	a.Hide()
	a.Line(avatarOf("hubot"))
	hidden, _ := load(t, a)
	got := rawOf(t, a.Show())
	if got != hidden || strings.Count(got, "a=t,") != 1 {
		t.Errorf("Show sent %q, want what was sent while hidden, %q", got, hidden)
	}
	if a.Show() != nil {
		t.Error("Show without Hide sends")
	}
}
