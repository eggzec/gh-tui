package ui

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/pkg/termimg"
)

// Picture is an image ready to send to the terminal: a PNG of Width by
// Height pixels, which takes Cols by Rows cells of the box it was made to
// fit.
type Picture struct {
	PNG           []byte
	Width, Height int
	Cols, Rows    int
}

// ImageBox is the room a picture is made to fit: Cols by Rows cells, each
// Cell pixels.
type ImageBox struct {
	Cols, Rows int
	Cell       imgcaps.Cell
}

// ImageSource is where an image comes from: an address on the web, such
// as an avatar's, or a file of a repository, by its blob.
type ImageSource struct {
	// URL is the address of an image on the web, as GitHub gave it.
	URL string
	// Repo and SHA name a file of a repository by its blob, which is read
	// as the repository's other content is, with the user's credentials.
	// Size is the file's size from its tree entry, if known.
	Repo core.RepoRef
	SHA  string
	Size int64
}

// file reports whether s is a file of a repository.
func (s ImageSource) file() bool { return s.SHA != "" }

// key names the image of s: two sources of one key are one image.
func (s ImageSource) key() string {
	if s.file() {
		// A blob's name is the hash of its content, but it is read
		// through its repository, which another may not be readable as.
		return "blob " + s.Repo.String() + " " + s.SHA
	}
	return s.URL
}

// ImageFetch returns the image of src made to fit box. It blocks, so the
// images call it in a tea.Cmd. An error that wraps ErrImageGone says the
// image won't load however often it is asked for, such as one the host
// doesn't have; any other, such as a network's, may pass.
type ImageFetch func(ctx context.Context, src ImageSource, box ImageBox) (Picture, error)

// ErrImageGone is an image that won't load: the host refused it or
// doesn't have it, or it is no image the app can show.
var ErrImageGone = errors.New("image won't load")

// ImageSize is a box in cells, which an image takes or is made to fit.
type ImageSize struct{ Cols, Rows int }

// The sizes of avatars: a cell is about twice as tall as it is wide, so
// each is about square. An avatar's box is the same whether it has
// arrived or not, so nothing moves when it does.
var (
	// AvatarSmall goes before a login on one line, such as in the header
	// of a comment.
	AvatarSmall = ImageSize{Cols: 2, Rows: 1}
	// AvatarLarge goes beside a profile.
	AvatarLarge = ImageSize{Cols: 6, Rows: 3}
)

// avatarPixelsPerRow is how many pixels tall an avatar is asked for per
// row of its box: twice a common cell, so it stays sharp on a screen of
// twice the density. One size per box, not per font, keeps one address,
// and so one copy kept, for each avatar.
const avatarPixelsPerRow = 40

// maxImages is how many images the terminal holds at once: one for each
// ID of the app's pool.
const maxImages = 255

// maxHeldBytes is how many bytes of PNG the images the terminal holds may
// take together. The app keeps a copy of each, to send again to a
// terminal that tmux is attached from anew, and avatars are a few
// kilobytes while a file fitted to a pane may be megabytes.
const maxHeldBytes = 64 << 20

// ImagesMsg tells every section and modal that the images drawn changed:
// some arrived, or failed, or the terminal began or stopped showing them.
// Those that show images draw them again.
type ImagesMsg struct{}

// Redraw says when the views must draw the images again, after the images
// changed.
type Redraw int

const (
	// RedrawNone needs no drawing.
	RedrawNone Redraw = iota
	// RedrawSoon draws once images arriving together have, so many
	// arrivals cost one drawing.
	RedrawSoon
	// RedrawNow draws at once, before the next frame: an image took the
	// ID of another, whose cells would show the new one until drawn
	// again.
	RedrawNow
)

// ImageState is how far an image drawn with [Images.Fit] got.
type ImageState int

const (
	// ImageOff is an image not drawn: the terminal shows none, or the
	// app has no way to fetch them.
	ImageOff ImageState = iota
	// ImageLoading is an image on its way.
	ImageLoading
	// ImageShown is an image drawn.
	ImageShown
	// ImageFailed is an image that couldn't be fetched or shown.
	ImageFailed
)

// imageMsg carries a fetched image to the images it was asked by.
type imageMsg struct {
	images *Images
	key    string
	pic    Picture
	err    error
}

// imageState is how far an image got.
type imageState int

const (
	imageWanted imageState = iota
	imageLoading
	imageReady
	imageFailed
)

type entry struct {
	src  ImageSource
	size ImageSize
	// fit is set for an image that takes the cells its picture fits,
	// rather than its whole box.
	fit   bool
	state imageState
	pic   Picture
	// id is the image the terminal knows the image by, while sent.
	id   termimg.ID
	sent bool
	// drawn is the update the image was last drawn in, and queued is set
	// while it waits in wanted.
	drawn  uint64
	queued bool
	// gone is set when it failed in a way no retry mends.
	gone bool
}

// cells returns the cells the image takes once sent.
func (e *entry) cells() ImageSize {
	if e.fit {
		return ImageSize{Cols: max(e.pic.Cols, 1), Rows: max(e.pic.Rows, 1)}
	}
	return e.size
}

// sendSeq returns what sends e to the terminal and places it in its cells.
func (e *entry) sendSeq() string {
	c := e.cells()
	return termimg.Transmit(e.id, e.pic.PNG, e.pic.Width, e.pic.Height) + termimg.Place(e.id, c.Cols, c.Rows)
}

// Images draws images with kitty's Unicode placeholders, when the
// terminal shows them: the avatars of people and of the owners of
// repositories, when the config wants them, image files of repositories,
// and the images of comments and bodies. Every section shares one, since
// the terminal knows the images of the whole app by IDs from one pool.
//
// An image is drawn as text: cells that name it, which the terminal fills
// with it. The image is sent once, out of band, through tea.Raw, never in
// a view, and the cells survive scrolling, overlays and redraws like any
// other text. Where the terminal shows no images, nothing is drawn, and
// the views look as they do without.
//
// What draws an image asks for it by drawing it, with [Images.Line],
// [Images.Box] or [Images.Fit]; the app calls [Images.Load] after each
// update, which fetches what was asked, and hands each image that arrives
// to [Images.Update], which sends it to the terminal. The app then tells
// the sections with an ImagesMsg, and they draw again.
//
// The terminal holds at most maxImages at once, of at most maxHeldBytes
// together. Once it holds that much, an image takes the room of those
// drawn least recently, and only of those drawn before it was: of those
// drawn in the same update, such as by one thread with more authors than
// that, the rest stay blank. Since every view draws its images again in
// the update that follows a change, an image on view is never taken from,
// so drawing again asks for nothing it took, and the fetches end.
//
// A nil *Images draws nothing. It is not safe for concurrent use: the app
// uses it from its updates alone.
type Images struct {
	ctx   context.Context
	fetch ImageFetch
	// avatars is the config's images.avatars, and maxRows its
	// images.max_rows.
	avatars bool
	maxRows int
	g       Graphics
	pool    *termimg.Pool
	byKey   map[string]*entry
	// wanted are the keys drawn and not yet fetched, or fetched and not
	// yet sent.
	wanted []string
	// gen counts the updates, which Load ends.
	gen uint64
	// offline holds the fetches from the web while GitHub can't be
	// reached, and closed ends drawing once the images were deleted, as
	// the app quits, and deleted is what deleted them, which Close
	// returns again.
	offline, closed bool
	deleted         string
	// hidden is set while tmux may drop what is sent, and sentHidden
	// holds the keys sent meanwhile.
	hidden     bool
	sentHidden map[string]bool
}

// NewImages returns images fetched with fetch on ctx. avatars is the
// config's images.avatars: without it no avatar is drawn, and without
// fetch no image at all.
func NewImages(ctx context.Context, fetch ImageFetch, avatars bool) *Images {
	return &Images{
		ctx: ctx, fetch: fetch, avatars: avatars,
		pool: termimg.NewPool(termimg.RandomMSB()), byKey: make(map[string]*entry),
	}
}

// drawing reports whether images are drawn at all: the terminal shows
// them and the app can fetch them.
func (a *Images) drawing() bool {
	return a != nil && a.fetch != nil && a.g.Images
}

// Shown reports whether avatars are drawn: the config wants them and the
// terminal shows images.
func (a *Images) Shown() bool {
	return a.drawing() && a.avatars
}

// Drawing reports whether images other than avatars are drawn, such as
// image files and the images of markdown: the terminal shows images.
func (a *Images) Drawing() bool {
	return a.drawing()
}

// SetMaxRows sets the tallest the images of markdown may be, in rows: the
// config's images.max_rows.
func (a *Images) SetMaxRows(n int) {
	if a != nil {
		a.maxRows = n
	}
}

// PictureRows returns the tallest the images of markdown are drawn in a
// view of height rows: at most the config's images.max_rows, and never
// taller than the view less two rows, so one never fills it. It returns 0
// when images aren't drawn.
func (a *Images) PictureRows(height int) int {
	if !a.drawing() {
		return 0
	}
	rows := max(height-2, 1)
	if a.maxRows > 0 {
		rows = min(rows, a.maxRows)
	}
	return rows
}

// Pictures returns what draws the images of markdown on the web, fitted
// to the room the markdown gives them and at most rows tall, as
// PictureRows says, or nil for 0 rows, so markdown renders as it does
// without images.
func (a *Images) Pictures(rows int) func(url string, width int) []string {
	if rows <= 0 || !a.drawing() {
		return nil
	}
	return func(url string, width int) []string {
		lines, _ := a.Fit(ImageSource{URL: url}, ImageSize{Cols: width, Rows: rows})
		return lines
	}
}

// Line returns the avatar at addr, the address GitHub gave for it, in a
// box of AvatarSmall, and a space after it, to put before a login on its
// line, or "" when no avatar is drawn.
func (a *Images) Line(addr string) string {
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
func (a *Images) Box(addr string, size ImageSize) []string {
	if !a.Shown() {
		return nil
	}
	size = ImageSize{Cols: max(size.Cols, 1), Rows: max(size.Rows, 1)}
	if addr != "" {
		if e := a.want(ImageSource{URL: addr}, size, false); e.state == imageReady && e.sent {
			return termimg.Rows(e.id, size.Cols, size.Rows)
		}
	}
	blank := strings.Repeat(" ", size.Cols)
	lines := make([]string, size.Rows)
	for i := range lines {
		lines[i] = blank
	}
	return lines
}

// Fit returns the lines of the image of src, scaled down to fit in size
// cells, its aspect kept: as many lines, of as many cells, as the image
// takes once it arrived, or none until then. The state says which, so the
// caller can show what it showed without images instead, until the image
// arrives or when it fails. Like those of Box, the lines must reach the
// terminal as they are.
func (a *Images) Fit(src ImageSource, size ImageSize) ([]string, ImageState) {
	if !a.drawing() || !src.file() && src.URL == "" {
		return nil, ImageOff
	}
	size = ImageSize{Cols: min(max(size.Cols, 1), termimg.MaxCells), Rows: min(max(size.Rows, 1), termimg.MaxCells)}
	e := a.want(src, size, true)
	switch {
	case e.state == imageReady && e.sent:
		c := e.cells()
		return termimg.Rows(e.id, c.Cols, c.Rows), ImageShown
	case e.state == imageFailed:
		return nil, ImageFailed
	}
	return nil, ImageLoading
}

// want returns the image of src in a box of size, drawn in this update,
// and asks for it if it is still to be fetched or sent.
func (a *Images) want(src ImageSource, size ImageSize, fit bool) *entry {
	key := imageKey(src, size, fit)
	e, ok := a.byKey[key]
	if !ok {
		e = &entry{src: src, size: size, fit: fit}
		a.byKey[key] = e
	}
	e.drawn = a.gen
	if (e.state == imageWanted || e.state == imageReady && !e.sent) && !e.queued {
		e.queued = true
		a.wanted = append(a.wanted, key)
	}
	return e
}

// SetOffline holds the fetches from the web while GitHub can't be
// reached; those drawn meanwhile start once it can. Files of repositories
// are still read, as the files service serves those it kept.
func (a *Images) SetOffline(offline bool) {
	if a != nil {
		a.offline = offline
	}
}

// Load ends an update: it returns the command that fetches the images
// drawn in it, once the size of a cell is known, which they are made to
// fit, and sends those that arrived before and wait to be, and says when
// the views must draw again.
func (a *Images) Load() (tea.Cmd, Redraw) {
	if a == nil {
		return nil, RedrawNone
	}
	defer func() {
		a.prune()
		a.gen++
	}()
	if !a.drawing() || a.closed || len(a.wanted) == 0 {
		return nil, RedrawNone
	}
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
		case imageWanted:
			if !a.g.Cell.Valid() || a.offline && !e.src.file() {
				later = append(later, key)
				continue
			}
			e.state, e.queued = imageLoading, false
			cmds = append(cmds, a.fetchCmd(key, e))
		case imageReady:
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

// prune forgets the images that arrived but weren't sent, since the
// terminal held as much as it may, and weren't drawn in this update: the
// app would otherwise keep their pictures for as long as it runs. One
// drawn again is fetched again, mostly from the fetcher's own memory.
// While images aren't drawn nothing is pruned, so those that arrived are
// sent again once they are.
func (a *Images) prune() {
	if !a.drawing() || a.closed {
		return
	}
	for key, e := range a.byKey {
		if e.state == imageReady && !e.sent && !e.queued && e.drawn < a.gen {
			delete(a.byKey, key)
		}
	}
}

func (a *Images) fetchCmd(key string, e *entry) tea.Cmd {
	ctx, fetch, src := a.ctx, a.fetch, e.src
	box := ImageBox{Cols: e.size.Cols, Rows: e.size.Rows, Cell: a.g.Cell}
	return func() tea.Msg {
		pic, err := fetch(ctx, src, box)
		return imageMsg{images: a, key: key, pic: pic, err: err}
	}
}

// Update takes msg if it is a fetched image of a, and returns what sends
// it to the terminal, and when the views must draw again.
func (a *Images) Update(msg tea.Msg) (cmd tea.Cmd, redraw Redraw, handled bool) {
	m, ok := msg.(imageMsg)
	if !ok || a == nil || m.images != a {
		return nil, RedrawNone, false
	}
	e, ok := a.byKey[m.key]
	if !ok || e.state != imageLoading {
		return nil, RedrawNone, true
	}
	if m.err != nil || len(m.pic.PNG) == 0 {
		// An avatar's box stays blank, while a file shows what it showed
		// without images, so its view draws again. One that may mend is
		// asked for again once GitHub answers again (Online).
		e.state, e.gone = imageFailed, m.err == nil || errors.Is(m.err, ErrImageGone)
		if e.gone && m.err != nil {
			slog.DebugContext(a.ctx, "image unavailable", "span", "tui", "host", hostOf(e.src.URL), "file", e.src.file(), "err", m.err.Error())
		}
		if e.fit {
			return nil, RedrawSoon, true
		}
		return nil, RedrawNone, true
	}
	e.state, e.pic = imageReady, m.pic
	if !a.drawing() || a.closed {
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
// its cells, giving it an ID if it has none. When the terminal holds as
// much as it may, it takes the room of the images drawn least recently,
// deleting them and forgetting them, if those were drawn before e;
// otherwise e isn't sent, and waits until it is drawn again. took reports
// room taken, whose old cells must be drawn again at once.
func (a *Images) send(key string, e *entry) (seq string, sent, took bool) {
	if !e.sent {
		victims, ok := a.victims(e)
		if !ok {
			return "", false, false
		}
		var b strings.Builder
		for _, vk := range victims {
			b.WriteString(termimg.Delete(a.byKey[vk].id))
			a.pool.Release(vk)
			delete(a.byKey, vk)
		}
		seq, took = b.String(), len(victims) > 0
	}
	id, _, _ := a.pool.Get(key)
	e.id, e.sent = id, true
	if a.hidden {
		a.sentHidden[key] = true
	}
	return seq + e.sendSeq(), true, took
}

// victims returns the keys of the images held that must go for e to be
// sent, the least recently drawn first, and false if those would include
// one drawn no earlier than e.
func (a *Images) victims(e *entry) ([]string, bool) {
	n, size := 0, 0
	var held []string
	for k, v := range a.byKey {
		if v.sent {
			n, size = n+1, size+len(v.pic.PNG)
			held = append(held, k)
		}
	}
	fits := func() bool { return n < maxImages && size+len(e.pic.PNG) <= maxHeldBytes }
	if fits() {
		return nil, true
	}
	slices.SortFunc(held, func(x, y string) int {
		return cmp.Or(cmp.Compare(a.byKey[x].drawn, a.byKey[y].drawn), strings.Compare(x, y))
	})
	for i, k := range held {
		v := a.byKey[k]
		if v.drawn >= e.drawn {
			return nil, false
		}
		n, size = n-1, size-len(v.pic.PNG)
		if fits() {
			return held[:i+1], true
		}
	}
	return nil, false
}

// SetGraphics takes what the terminal shows, and reports whether that
// changed whether images are drawn. When the terminal began to show
// images, as one tmux was attached from anew does, the images that
// arrived are sent again at the end of the update.
func (a *Images) SetGraphics(g Graphics) (changed bool) {
	if a == nil {
		return false
	}
	was := a.drawing()
	a.g = g
	now := a.drawing()
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
		if e := a.byKey[key]; e.state == imageReady && !e.queued {
			e.queued = true
			a.wanted = append(a.wanted, key)
		}
	}
	return true
}

// Resend returns what sends every image the terminal holds to it again,
// as a terminal that tmux is attached from anew needs, since it never had
// them.
func (a *Images) Resend() tea.Cmd {
	if !a.drawing() || a.closed {
		return nil
	}
	var b strings.Builder
	for _, key := range slices.Sorted(maps.Keys(a.byKey)) {
		if e := a.byKey[key]; e.sent {
			b.WriteString(e.sendSeq())
		}
	}
	return a.raw(b.String())
}

// Online asks again, as they are next drawn, for the images that failed
// in a way that may mend, now that GitHub answers again, and reports
// whether there were any, which the views must draw again to ask.
func (a *Images) Online() bool {
	if a == nil {
		return false
	}
	failed := false
	for key, e := range a.byKey {
		if e.state == imageFailed && !e.gone {
			delete(a.byKey, key)
			failed = true
		}
	}
	return failed && a.drawing()
}

// Holding reports whether the terminal holds images the app sent.
func (a *Images) Holding() bool {
	return a != nil && !a.closed && a.held() > 0
}

// held counts the images the terminal holds.
func (a *Images) held() int {
	n := 0
	for _, e := range a.byKey {
		if e.sent {
			n++
		}
	}
	return n
}

// Close returns what deletes every image sent from the terminal, which
// keeps them after the app has gone, and sends no more. The app writes it
// as it quits. Called again, it returns the same deletes, for when they
// may not have reached the terminal; deleting twice does no harm.
func (a *Images) Close() string {
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
// view; the images sent until Show are sent again then.
func (a *Images) Hide() {
	if a != nil && !a.hidden {
		a.hidden, a.sentHidden = true, make(map[string]bool)
	}
}

// Show ends Hide, and returns what sends again the images sent while
// hidden that the terminal should still hold.
func (a *Images) Show() tea.Cmd {
	if a == nil || !a.hidden {
		return nil
	}
	sent := a.sentHidden
	a.hidden, a.sentHidden = false, nil
	if !a.drawing() || a.closed {
		return nil
	}
	var b strings.Builder
	for _, key := range slices.Sorted(maps.Keys(sent)) {
		if e, ok := a.byKey[key]; ok && e.sent {
			b.WriteString(e.sendSeq())
		}
	}
	return a.raw(b.String())
}

// raw returns the command that writes seq to the terminal.
func (a *Images) raw(seq string) tea.Cmd {
	if seq == "" {
		return nil
	}
	return tea.Raw(a.wrap(seq))
}

// wrap wraps seq in tmux's passthrough when the app runs inside tmux.
func (a *Images) wrap(seq string) string {
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

func imageKey(src ImageSource, size ImageSize, fit bool) string {
	mode := " "
	if fit {
		mode = " fit "
	}
	return strconv.Itoa(size.Cols) + "x" + strconv.Itoa(size.Rows) + mode + src.key()
}

// SizedAvatar returns raw, the address of an avatar as GitHub gave it,
// asking for the pixels a box of size wants, with the s parameter its
// avatar hosts take, or "" when raw is no address.
func SizedAvatar(raw string, size ImageSize) string {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return ""
	}
	q := u.Query()
	q.Set("s", strconv.Itoa(max(size.Rows, 1)*avatarPixelsPerRow))
	u.RawQuery = q.Encode()
	return u.String()
}
