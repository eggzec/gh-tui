package dev

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// ZoomContext is the key context of the zoomed preview of a row.
const ZoomContext = "item_preview"

// maxCommentPages bounds how many pages of comments the zoomed preview reads
// by itself for one row.
const maxCommentPages = 10

var zoomIDs atomic.Int64

// commentsMsg is what a read of a page of comments brought.
type commentsMsg struct {
	id   int64
	key  string
	gen  int
	page core.Page[core.Comment]
	err  error
}

// thread is the comments read so far of one row.
type thread struct {
	items   []core.Comment
	next    string
	total   int
	pages   int
	done    bool
	loading bool
	err     error
	gen     int
	cancel  context.CancelFunc
}

func (t *thread) take(p core.Page[core.Comment]) {
	t.items = append(t.items, p.Items...)
	t.next, t.done = p.Next, p.Last()
	t.total = max(t.total, p.Total)
	t.pages++
}

// Zoom shows the preview of a row of the Development, Blockers or Involved
// panes in a pager when the split is zoomed: the whole description rendered
// as markdown and every comment, read only, with search and filter. It is
// used by pointer, since it holds a pager and the comments read.
type Zoom struct {
	p     *Previews
	id    int64
	pg    pager.Model
	theme ui.Theme
	icons ui.Icons
	md    func(src string, width int, open ...int) string

	key     string
	item    Item
	opts    PreviewOptions
	seen    int
	ver     int
	threads map[string]*thread
	// pos is where the pager was left on each row, which a row that comes back
	// takes up again once it has its size.
	pos  map[string]pager.Position
	want *pager.Position
}

// NewZoom returns a zoomed preview that reads through p, takes its keys from
// keys and draws with theme and icons.
func NewZoom(p *Previews, keys config.Keymap, theme ui.Theme, icons ui.Icons) *Zoom {
	look := ui.In(keys, ZoomContext)
	z := &Zoom{
		p: p, id: zoomIDs.Add(1),
		pg:      pager.New(pager.WithKeyMap(pager.NewKeyMap(look))),
		threads: map[string]*thread{},
		pos:     map[string]pager.Position{},
	}
	z.SetStyles(theme, icons)
	return z
}

// Zoomable reports whether the row has a description and comments to show
// in the pager.
func Zoomable(key string, it Item) bool {
	if key == "" || it.Ref.Problem != "" {
		return false
	}
	switch it.Kind {
	case Issue, Blocker, Pull:
		return true
	case Header, More, None:
	}
	return false
}

// Show makes the row under key the one the pager shows, and returns the read
// of its comments. The comments of the row shown before stop being read, and
// resume if it comes back. A row with nothing to show leaves the pager empty.
func (z *Zoom) Show(key string, it Item, o PreviewOptions) tea.Cmd {
	if !Zoomable(key, it) {
		z.leave()
		return nil
	}
	z.opts = o
	if key != z.key {
		z.ver++
		z.leave()
		z.key, z.item, z.seen = key, it, z.p.reads
		z.pg.SetRendered(key, "", z.render)
		z.pg.SetSource(z.source())
		p := z.pos[key]
		z.want = &p
	}
	return z.start()
}

// Key is the row the pager shows, or "".
func (z *Zoom) Key() string { return z.key }

// Shown reports whether the pager shows a row.
func (z *Zoom) Shown() bool { return z.key != "" }

// leave stops reading the comments of the row shown.
func (z *Zoom) leave() {
	if z.key != "" {
		z.pos[z.key] = z.pg.Position()
	}
	if t := z.threads[z.key]; t != nil && t.cancel != nil {
		t.cancel()
		t.cancel, t.loading = nil, false
	}
	z.key = ""
}

// Draw returns the pager at the size, ending with its status line.
func (z *Zoom) Draw(width, height int) string {
	z.pg.SetSize(width, height)
	if z.seen != z.p.reads {
		z.seen = z.p.reads
		z.refresh()
	}
	if z.want != nil {
		z.pg.SetPosition(*z.want)
		z.want = nil
	}
	return z.pg.View()
}

// Top is the first line of the window of the pager.
func (z *Zoom) Top() int { return z.pg.Position().Top }

// Focus gives the pager the keys, or takes them back.
func (z *Zoom) Focus(on bool) {
	if on != z.pg.Focused() {
		z.ver++
	}
	if on {
		z.pg.Focus()
		return
	}
	z.pg.Blur()
}

// Capturing reports whether the pager takes every key, as it does while its
// prompt or option choice is open.
func (z *Zoom) Capturing() bool { return z.pg.Capturing() }

// Pager is the pager, for the help to list its keys.
func (z *Zoom) Pager() *pager.Model { return &z.pg }

// Wants reports whether msg is one of the pager's keys for moving, searching
// and filtering, which it takes before the keys of the modal around it.
func (z *Zoom) Wants(msg tea.KeyPressMsg) bool {
	k := z.pg.KeyMap()
	return keymap.Matches(msg, k.Up, k.Down, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown,
		k.Home, k.End, k.Left, k.Right, k.Option, k.Search, k.Filter, k.Next, k.Prev, k.Edit)
}

// Press hands a key to the pager.
func (z *Zoom) Press(msg tea.KeyPressMsg) tea.Cmd {
	z.ver++
	var cmd tea.Cmd
	z.pg, cmd = z.pg.Update(msg)
	return cmd
}

// Scroll moves the pager a line, as its down or up key does. The host uses it
// for the keys that scroll the other split.
func (z *Zoom) Scroll(down bool) {
	act := ".up"
	if down {
		act = ".down"
	}
	// The pager takes keys only while it has the focus, which the list has.
	was := z.pg.Focused()
	z.pg.Focus()
	z.Press(keymap.ActionPress(ZoomContext + act))
	if !was {
		z.pg.Blur()
	}
}

// Version changes when what the pager draws does: a key, a page of comments, a
// new row or the focus.
func (z *Zoom) Version() int { return z.ver }

// Handles reports whether msg is a read of comments of this pager.
func (z *Zoom) Handles(msg tea.Msg) bool {
	m, ok := msg.(commentsMsg)
	return ok && m.id == z.id
}

// Update takes a page of comments that a read brought, and returns the read of
// the next while its row is still the one shown. Other messages, such as a
// search that ran on, go to the pager.
func (z *Zoom) Update(msg tea.Msg) tea.Cmd {
	z.ver++
	m, ok := msg.(commentsMsg)
	if !ok {
		var cmd tea.Cmd
		z.pg, cmd = z.pg.Update(msg)
		return cmd
	}
	t := z.threads[m.key]
	if m.id != z.id || t == nil || m.gen != t.gen {
		return nil
	}
	if t.cancel != nil {
		t.cancel()
	}
	t.loading, t.cancel = false, nil
	switch {
	case errors.Is(m.err, context.Canceled):
		return nil
	case m.err != nil:
		t.err = m.err
	default:
		t.take(m.page)
	}
	if m.key != z.key {
		return nil
	}
	z.refresh()
	return z.start()
}

// refresh renders the pager again, and gives it the source that the editor
// opens, after what it shows changed.
func (z *Zoom) refresh() {
	z.pg.Rerender()
	z.pg.SetSource(z.source())
}

// source is the row as markdown, for the editor: the description, then each
// comment under a heading of its author and date.
func (z *Zoom) source() string {
	var body string
	switch z.item.Kind {
	case Pull:
		if v, ok := z.p.pull(z.key); ok {
			body = v.Body
		}
	case Issue, Blocker:
		if v, ok := z.p.issue(z.key); ok {
			body = v.Body
		}
	case Header, More, None:
	}
	parts := []string{body}
	if t := z.threads[z.key]; t != nil {
		for i := range t.items {
			c := &t.items[i]
			head := "## " + c.Author.Login
			if !c.CreatedAt.IsZero() {
				head += ", " + c.CreatedAt.Format("2006-01-02 15:04")
			}
			parts = append(parts, head+"\n\n"+c.Body)
		}
	}
	return strings.Join(parts, "\n\n")
}

// SetStyles draws the pager and its markdown with theme and icons, after they
// changed.
func (z *Zoom) SetStyles(theme ui.Theme, icons ui.Icons) {
	z.theme, z.icons, z.md = theme, icons, nil
	z.pg.SetStyles(theme.Pager(icons))
	z.refresh()
	z.ver++
}

// Retry forgets the comment reads that failed, so that they are made again
// when the row is shown.
func (z *Zoom) Retry() {
	for _, t := range z.threads {
		t.err = nil
	}
	z.refresh()
	z.ver++
}

// start fills the comments of the row shown from what is cached, then returns
// the read of the next page, unless it has them all or a read is out.
func (z *Zoom) start() tea.Cmd {
	key := z.key
	t := z.threads[key]
	if t == nil {
		t = &thread{}
		z.threads[key] = t
	}
	r, target := z.p.r, z.item.Ref.Target
	grew := false
	for !t.done && t.pages < maxCommentPages && !t.loading && t.err == nil {
		page, ok := r.CachedComments(target, t.next)
		if !ok {
			break
		}
		t.take(page)
		grew = true
	}
	if grew {
		z.refresh()
	}
	if t.done || t.loading || t.err != nil || t.pages >= maxCommentPages {
		return nil
	}
	ctx, cancel := context.WithCancel(z.p.ctx)
	t.cancel, t.loading = cancel, true
	t.gen++
	id, gen, cursor := z.id, t.gen, t.next
	return func() tea.Msg {
		page, err := r.Comments(ctx, target, cursor)
		return commentsMsg{id: id, key: key, gen: gen, page: page, err: err}
	}
}

// render is the content of the pager at a width: the head of the preview, the
// description as markdown, then each comment.
func (z *Zoom) render(width int) string {
	o := z.opts
	o.Width = width
	var out []string
	var body string
	switch z.item.Kind {
	case Pull:
		v, ok := z.p.pull(z.key)
		if !ok {
			return z.waiting(o)
		}
		out, body = pullHeader(v, o), v.Body
	case Issue, Blocker:
		v, ok := z.p.issue(z.key)
		if !ok {
			return z.waiting(o)
		}
		out, body = issueHeader(v, o), v.Body
	case Header, More, None:
		return Nothing(o)
	}
	st := o.Styles
	if strings.TrimSpace(body) != "" {
		out = append(out, "", z.markdown(body, width))
	}
	rule := st.dim.Render(strings.Repeat(st.Glyphs.Rule, max(width, 0)))
	t := z.threads[z.key]
	if t == nil {
		return strings.Join(out, "\n")
	}
	switch {
	case len(t.items) > 0 || t.done:
		out = append(out, "", rule, st.bold.Render(plural(max(t.total, len(t.items)), "comment")))
	case t.loading:
		out = append(out, "", rule, st.dim.Render("loading comments"+st.Glyphs.Ellipsis))
	}
	for i := range t.items {
		c := &t.items[i]
		out = append(out, "", commentHead(c.Author.Login, c.CreatedAt, o), z.markdown(c.Body, width))
	}
	switch {
	case t.err != nil:
		out = append(out, "", st.warn.Render("could not read more comments: "+ui.OneLine(t.err.Error())))
	case len(t.items) > 0 && t.loading:
		out = append(out, "", st.dim.Render("loading more comments"+st.Glyphs.Ellipsis))
	case !t.done && t.pages >= maxCommentPages:
		out = append(out, "", st.dim.Render("more comments on GitHub"))
	}
	return strings.Join(out, "\n")
}

// waiting is what the pager shows before the preview of the row has landed.
func (z *Zoom) waiting(o PreviewOptions) string {
	if z.p.failed[z.key] {
		return Nothing(o)
	}
	return Loading(o)
}

// markdown renders src at width, without the blank lines around it.
func (z *Zoom) markdown(src string, width int) string {
	if z.md == nil {
		z.md = ui.NewMarkdown(z.theme, z.icons).Render
	}
	return strings.Trim(z.md(src, width), "\n")
}
