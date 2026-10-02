package ui

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/pkg/termimg"
)

// Picture is an image ready to send to the terminal: a PNG of Width by
// Height pixels.
type Picture struct {
	PNG           []byte
	Width, Height int
}

// ImageBox is the room a picture is made to fit: Cols by Rows cells, each
// Cell pixels.
type ImageBox struct {
	Cols, Rows int
	Cell       imgcaps.Cell
}

// ImageFetch returns the image at addr made to fit box. It blocks, so the
// avatars call it in a tea.Cmd. An error that wraps ErrImageGone says the
// image won't load however often it is asked for, such as one the host
// doesn't have; any other, such as a network's, may pass.
type ImageFetch func(ctx context.Context, addr string, box ImageBox) (Picture, error)

// ErrImageGone is an image that won't load: the host refused it or
// doesn't have it, or it is no image the app can show.
var ErrImageGone = errors.New("image won't load")

// AvatarSize is the box an avatar takes, in cells. The box is the same
// whether the avatar has arrived or not, so nothing moves when it does.
type AvatarSize struct{ Cols, Rows int }

// The sizes of avatars: a cell is about twice as tall as it is wide, so
// each is about square.
var (
	// AvatarSmall goes before a login on one line, such as in the header
	// of a comment.
	AvatarSmall = AvatarSize{Cols: 2, Rows: 1}
	// AvatarLarge goes beside a profile.
	AvatarLarge = AvatarSize{Cols: 6, Rows: 3}
)

// avatarPixelsPerRow is how many pixels tall an avatar is asked for per
// row of its box: twice a common cell, so it stays sharp on a screen of
// twice the density. One size per box, not per font, keeps one address,
// and so one copy kept, for each avatar.
const avatarPixelsPerRow = 40

// maxAvatars is how many avatars the terminal holds at once: one for each
// ID of the app's pool.
const maxAvatars = 255

// AvatarsMsg tells every section and modal that the avatars drawn changed:
// some arrived, or the terminal began or stopped showing them. Those that
// show avatars draw them again.
type AvatarsMsg struct{}

// Redraw says when the views must draw the avatars again, after the
// avatars changed.
type Redraw int

const (
	// RedrawNone needs no drawing.
	RedrawNone Redraw = iota
	// RedrawSoon draws once avatars arriving together have, so many
	// arrivals cost one drawing.
	RedrawSoon
	// RedrawNow draws at once, before the next frame: an avatar took the
	// ID of another, whose cells would show the new one until drawn
	// again.
	RedrawNow
)

// avatarMsg carries a fetched avatar to the avatars it was asked by.
type avatarMsg struct {
	avatars *Avatars
	key     string
	pic     Picture
	err     error
}

// avatarState is how far an avatar got.
type avatarState int

const (
	avatarWanted avatarState = iota
	avatarLoading
	avatarReady
	avatarFailed
)

type avatar struct {
	url   string
	size  AvatarSize
	state avatarState
	pic   Picture
	// id is the image the terminal knows the avatar by, while sent.
	id   termimg.ID
	sent bool
	// drawn is the update the avatar was last drawn in, and queued is set
	// while it waits in wanted.
	drawn  uint64
	queued bool
	// gone is set when it failed in a way no retry mends.
	gone bool
}

// Avatars draws the avatars of people and of the owners of repositories,
// with kitty's Unicode placeholders, when the terminal shows them and the
// config wants them. Every section shares one, since the terminal knows
// the images of the whole app by IDs from one pool.
//
// An avatar is drawn as text: cells that name the image, which the
// terminal fills with it. The image is sent once, out of band, through
// tea.Raw, never in a view, and the cells survive scrolling, overlays and
// redraws like any other text. Until it arrives its box is blank, so
// nothing moves when it does. Where the terminal shows no images, the
// avatars take no cells at all, and the views look as they do without.
//
// What draws an avatar asks for it by drawing it, with [Avatars.Line] or
// [Avatars.Box]; the app calls [Avatars.Load] after each update, which
// fetches what was asked, and hands each avatar that arrives to
// [Avatars.Update], which sends it to the terminal. The app then tells the
// sections with an AvatarsMsg, and they draw again.
//
// The terminal holds at most maxAvatars at once. Once it holds that many,
// an avatar takes the ID of the one drawn least recently, and only of one
// drawn before it was: of those drawn in the same update, such as by one
// thread with more authors than that, the rest stay blank. Since every
// view draws its avatars again in the update that follows a change, an
// avatar on view is never taken from, so drawing again asks for nothing
// it took, and the fetches end.
//
// A nil *Avatars draws nothing. It is not safe for concurrent use: the app
// uses it from its updates alone.
type Avatars struct {
	ctx   context.Context
	fetch ImageFetch
	on    bool
	g     Graphics
	pool  *termimg.Pool
	byKey map[string]*avatar
	// wanted are the keys drawn and not yet fetched, or fetched and not
	// yet sent.
	wanted []string
	// gen counts the updates, which Load ends.
	gen uint64
	// offline holds the fetches while GitHub can't be reached, and
	// closed ends drawing once the images were deleted, as the app quits,
	// and deleted is what deleted them, which Close returns again.
	offline, closed bool
	deleted         string
	// hidden is set while tmux may drop what is sent, and sentHidden
	// holds the keys sent meanwhile.
	hidden     bool
	sentHidden map[string]bool
}

// NewAvatars returns avatars fetched with fetch on ctx. on is the config's
// images.avatars: without it, as without fetch, no avatar is drawn.
func NewAvatars(ctx context.Context, fetch ImageFetch, on bool) *Avatars {
	return &Avatars{
		ctx: ctx, fetch: fetch, on: on,
		pool: termimg.NewPool(termimg.RandomMSB()), byKey: make(map[string]*avatar),
	}
}

// Shown reports whether avatars are drawn: the config wants them and the
// terminal shows images.
func (a *Avatars) Shown() bool {
	return a != nil && a.on && a.fetch != nil && a.g.Images
}

// Line returns the avatar at addr, the address GitHub gave for it, in a
// box of AvatarSmall, and a space after it, to put before a login on its
// line, or "" when no avatar is drawn.
func (a *Avatars) Line(addr string) string {
	rows := a.Box(SizedAvatar(addr, AvatarSmall), AvatarSmall)
	if rows == nil {
		return ""
	}
	return rows[0] + " "
}

// Box returns the lines of the avatar at addr, size cells, each size.Cols
// wide: the cells that show it once it arrived, and blanks until then or
// when it couldn't be fetched, or none at all when no avatar is drawn.
// An empty addr, as of a deleted account, draws a blank box. The lines
// carry the color that names the image, so they must reach the terminal
// as they are: never inside a style that reverses them or sets their
// foreground, nor one that wraps.
func (a *Avatars) Box(addr string, size AvatarSize) []string {
	if !a.Shown() {
		return nil
	}
	size = AvatarSize{Cols: max(size.Cols, 1), Rows: max(size.Rows, 1)}
	if addr != "" {
		key := avatarKey(addr, size)
		e, ok := a.byKey[key]
		if !ok {
			e = &avatar{url: addr, size: size}
			a.byKey[key] = e
		}
		e.drawn = a.gen
		switch {
		case e.state == avatarReady && e.sent:
			return termimg.Rows(e.id, size.Cols, size.Rows)
		case (e.state == avatarWanted || e.state == avatarReady) && !e.queued:
			e.queued = true
			a.wanted = append(a.wanted, key)
		}
	}
	blank := strings.Repeat(" ", size.Cols)
	lines := make([]string, size.Rows)
	for i := range lines {
		lines[i] = blank
	}
	return lines
}

// SetOffline holds the fetches while GitHub can't be reached; those drawn
// meanwhile start once it can.
func (a *Avatars) SetOffline(offline bool) {
	if a != nil {
		a.offline = offline
	}
}

// Load ends an update: it returns the command that fetches the avatars
// drawn in it, once the size of a cell is known, which they are made to
// fit, and sends those that arrived before and wait to be, and says when
// the views must draw again.
func (a *Avatars) Load() (tea.Cmd, Redraw) {
	if a == nil {
		return nil, RedrawNone
	}
	defer func() { a.gen++ }()
	if !a.Shown() || a.closed || len(a.wanted) == 0 {
		return nil, RedrawNone
	}
	fetching := !a.offline && a.g.Cell.Valid()
	var (
		cmds  []tea.Cmd
		seq   strings.Builder
		later []string
		took  bool
	)
	for _, key := range a.wanted {
		e, ok := a.byKey[key]
		if !ok {
			continue
		}
		switch e.state {
		case avatarWanted:
			if !fetching {
				later = append(later, key)
				continue
			}
			e.state, e.queued = avatarLoading, false
			cmds = append(cmds, a.fetchCmd(key, e))
		case avatarReady:
			e.queued = false
			if s, sent, t := a.send(key, e); sent {
				seq.WriteString(s)
				took = took || t
			}
		default:
			e.queued = false
		}
	}
	a.wanted = later
	redraw := RedrawNone
	switch {
	case took:
		cmds, redraw = append(cmds, a.raw(seq.String())), RedrawNow
	case seq.Len() > 0:
		// The views drew these blank in this update.
		cmds, redraw = append(cmds, a.raw(seq.String())), RedrawSoon
	}
	return tea.Batch(cmds...), redraw
}

func (a *Avatars) fetchCmd(key string, e *avatar) tea.Cmd {
	ctx, fetch, addr := a.ctx, a.fetch, e.url
	box := ImageBox{Cols: e.size.Cols, Rows: e.size.Rows, Cell: a.g.Cell}
	return func() tea.Msg {
		pic, err := fetch(ctx, addr, box)
		return avatarMsg{avatars: a, key: key, pic: pic, err: err}
	}
}

// Update takes msg if it is a fetched avatar of a, and returns what sends
// it to the terminal, and when the views must draw again.
func (a *Avatars) Update(msg tea.Msg) (cmd tea.Cmd, redraw Redraw, handled bool) {
	m, ok := msg.(avatarMsg)
	if !ok || a == nil || m.avatars != a {
		return nil, RedrawNone, false
	}
	e, ok := a.byKey[m.key]
	if !ok || e.state != avatarLoading {
		return nil, RedrawNone, true
	}
	if m.err != nil || len(m.pic.PNG) == 0 {
		// The box stays blank. One that may mend is asked for again
		// once GitHub answers again (Online).
		e.state, e.gone = avatarFailed, m.err == nil || errors.Is(m.err, ErrImageGone)
		if e.gone && m.err != nil {
			slog.DebugContext(a.ctx, "avatar unavailable", "span", "tui", "host", hostOf(e.url), "err", m.err.Error())
		}
		return nil, RedrawNone, true
	}
	e.state, e.pic = avatarReady, m.pic
	if !a.Shown() || a.closed {
		return nil, RedrawNone, true
	}
	seq, sent, took := a.send(m.key, e)
	switch {
	case took:
		return a.raw(seq), RedrawNow, true
	case sent:
		return a.raw(seq), RedrawSoon, true
	}
	return nil, RedrawNone, true
}

// send returns what sends e, under key, to the terminal and places it in
// cells of its size, giving it an ID if it has none. When every ID is
// taken, it takes that of the avatar drawn least recently, deleting its
// image and forgetting it, if that one was drawn before e; otherwise e
// isn't sent, and waits until it is drawn again. took reports an ID
// taken, whose old cells must be drawn again at once.
func (a *Avatars) send(key string, e *avatar) (seq string, sent, took bool) {
	if !e.sent && a.held() >= maxAvatars {
		vk, v := a.leastDrawn()
		if v == nil || v.drawn >= e.drawn {
			return "", false, false
		}
		seq = termimg.Delete(v.id)
		a.pool.Release(vk)
		delete(a.byKey, vk)
		took = true
	}
	id, _, _ := a.pool.Get(key)
	e.id, e.sent = id, true
	if a.hidden {
		a.sentHidden[key] = true
	}
	return seq + termimg.Transmit(id, e.pic.PNG, e.pic.Width, e.pic.Height) +
		termimg.Place(id, e.size.Cols, e.size.Rows), true, took
}

// held counts the avatars the terminal holds.
func (a *Avatars) held() int {
	n := 0
	for _, e := range a.byKey {
		if e.sent {
			n++
		}
	}
	return n
}

// leastDrawn returns the avatar held that was drawn least recently.
func (a *Avatars) leastDrawn() (string, *avatar) {
	var (
		key   string
		least *avatar
	)
	for k, e := range a.byKey {
		if e.sent && (least == nil || e.drawn < least.drawn || e.drawn == least.drawn && k < key) {
			key, least = k, e
		}
	}
	return key, least
}

// SetGraphics takes what the terminal shows, and reports whether that
// changed whether avatars are drawn. When the terminal began to show
// images, as one tmux was attached from anew does, the avatars that
// arrived are sent again at the end of the update.
func (a *Avatars) SetGraphics(g Graphics) (changed bool) {
	if a == nil {
		return false
	}
	was := a.Shown()
	a.g = g
	now := a.Shown()
	if now == was {
		return false
	}
	if !now {
		// What the terminal kept of the images is gone, or will be
		// drawn by no cell.
		for _, e := range a.byKey {
			e.sent = false
		}
		a.pool = termimg.NewPool(termimg.RandomMSB())
		return true
	}
	for _, key := range slices.Sorted(maps.Keys(a.byKey)) {
		if e := a.byKey[key]; e.state == avatarReady && !e.queued {
			e.queued = true
			a.wanted = append(a.wanted, key)
		}
	}
	return true
}

// Resend returns what sends every avatar the terminal holds to it again,
// as a terminal that tmux is attached from anew needs, since it never had
// them. They are at most maxAvatars small images.
func (a *Avatars) Resend() tea.Cmd {
	if !a.Shown() || a.closed {
		return nil
	}
	var b strings.Builder
	for _, key := range slices.Sorted(maps.Keys(a.byKey)) {
		if e := a.byKey[key]; e.sent {
			b.WriteString(termimg.Transmit(e.id, e.pic.PNG, e.pic.Width, e.pic.Height) +
				termimg.Place(e.id, e.size.Cols, e.size.Rows))
		}
	}
	return a.raw(b.String())
}

// Online asks again, as they are next drawn, for the avatars that failed
// in a way that may mend, now that GitHub answers again, and reports
// whether there were any, which the views must draw again to ask.
func (a *Avatars) Online() bool {
	if a == nil {
		return false
	}
	failed := false
	for key, e := range a.byKey {
		if e.state == avatarFailed && !e.gone {
			delete(a.byKey, key)
			failed = true
		}
	}
	return failed && a.Shown()
}

// Holding reports whether the terminal holds images the app sent.
func (a *Avatars) Holding() bool {
	return a != nil && !a.closed && a.held() > 0
}

// Close returns what deletes every image sent from the terminal, which
// keeps them after the app has gone, and sends no more. The app writes it
// as it quits. Called again, it returns the same deletes, for when they
// may not have reached the terminal; deleting twice does no harm.
func (a *Avatars) Close() string {
	if a == nil {
		return ""
	}
	if a.closed {
		return a.deleted
	}
	a.closed = true
	var b strings.Builder
	for _, key := range slices.Sorted(maps.Keys(a.byKey)) {
		if e := a.byKey[key]; e.sent {
			b.WriteString(termimg.Delete(e.id))
			e.sent = false
		}
	}
	a.deleted = a.wrap(b.String())
	return a.deleted
}

// Hide says that what is sent may not reach the terminal, as inside tmux
// with allow-passthrough on, which drops the passthrough of a pane not on
// view; the avatars sent until Show are sent again then.
func (a *Avatars) Hide() {
	if a != nil && !a.hidden {
		a.hidden, a.sentHidden = true, make(map[string]bool)
	}
}

// Show ends Hide, and returns what sends again the avatars sent while
// hidden that the terminal should still hold.
func (a *Avatars) Show() tea.Cmd {
	if a == nil || !a.hidden {
		return nil
	}
	sent := a.sentHidden
	a.hidden, a.sentHidden = false, nil
	if !a.Shown() || a.closed {
		return nil
	}
	var b strings.Builder
	for _, key := range slices.Sorted(maps.Keys(sent)) {
		if e, ok := a.byKey[key]; ok && e.sent {
			b.WriteString(termimg.Transmit(e.id, e.pic.PNG, e.pic.Width, e.pic.Height) +
				termimg.Place(e.id, e.size.Cols, e.size.Rows))
		}
	}
	return a.raw(b.String())
}

// raw returns the command that writes seq to the terminal.
func (a *Avatars) raw(seq string) tea.Cmd {
	if seq == "" {
		return nil
	}
	return tea.Raw(a.wrap(seq))
}

// wrap wraps seq in tmux's passthrough when the app runs inside tmux.
func (a *Avatars) wrap(seq string) string {
	if seq == "" || !a.g.Tmux {
		return seq
	}
	return termimg.Tmux(seq)
}

// hostOf returns the host of addr, for the log, which the path, naming
// the account, stays out of.
func hostOf(addr string) string {
	u, err := url.Parse(addr)
	if err != nil {
		return ""
	}
	return u.Host
}

func avatarKey(addr string, size AvatarSize) string {
	return strconv.Itoa(size.Cols) + "x" + strconv.Itoa(size.Rows) + " " + addr
}

// SizedAvatar returns raw, the address of an avatar as GitHub gave it,
// asking for the pixels a box of size wants, with the s parameter its
// avatar hosts take, or "" when raw is no address.
func SizedAvatar(raw string, size AvatarSize) string {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return ""
	}
	q := u.Query()
	q.Set("s", strconv.Itoa(max(size.Rows, 1)*avatarPixelsPerRow))
	u.RawQuery = q.Encode()
	return u.String()
}
