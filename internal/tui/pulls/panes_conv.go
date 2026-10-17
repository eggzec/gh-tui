package pulls

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// convViewerMsg carries the login of the viewer to the modal that asked for
// it.
type convViewerMsg struct {
	id    int64
	login string
}

// convPane is what the Conversation pane shows: the timeline and the review
// threads of the pull request at head, which are read once the modal knows
// that head and again when it changes.
type convPane struct {
	model   conv.Model
	started bool
	// head is the commit the model reads the threads of.
	head string
	// version counts the changes of what the model draws, for the cache of the
	// panes, and stamp is what it drew last.
	version int
	stamp   string
	// focused says whether the model has the focus, and w and h are the cells
	// it was sized to.
	focused bool
	w, h    int

	// viewer is the login whose unsubmitted review is drawn as a draft, once
	// it is known.
	viewer string
	voice  ui.Voice
	// opt sets which kinds of events are shown when the model is made.
	opt   conv.Option
	dates ui.Dates
	now   func() time.Time
	md    *markdown.Renderer
}

// convLookup gives the actions the Conversation's context has. The tree has
// others, such as marking a row, that have no key here and no line in the
// config.
type convLookup struct {
	ui.Context
	keys config.Keymap
}

// Of implements keymap.Lookup. It asks Has, not Of, for what the config may
// lack, so that the actions the code reads stay those the config has.
func (l convLookup) Of(action string) []string {
	if !strings.Contains(action, ".") && !l.keys.Has(l.Scope()+"."+action) {
		return nil
	}
	return l.Context.Of(action)
}

// receiveHead starts the conversation when the detail has a head, and starts
// it again for a new one: the old reads end.
func (m *panesModal) receiveHead() tea.Cmd {
	if m.detail.HeadSHA == "" || (m.conv.started && m.conv.head == m.detail.HeadSHA) {
		return nil
	}
	return m.buildConv()
}

// buildConv makes the conversation of the head the detail has, and returns
// its first reads.
func (m *panesModal) buildConv() tea.Cmd {
	c := &m.conv
	if c.started {
		c.model.Close()
	}
	svc, repo, number, head := m.svc, m.repo, m.number, m.detail.HeadSHA
	timeline := func(cursor string) pulls.TimelineQuery {
		return pulls.TimelineQuery{Repo: repo, Number: number, Cursor: cursor}
	}
	threads := func(cursor string) pulls.ThreadsQuery {
		return pulls.ThreadsQuery{Repo: repo, Number: number, Head: head, Cursor: cursor}
	}
	src := conv.Source{
		CachedTimeline: func(cursor string) (core.Page[core.TimelineItem], bool) {
			return svc.CachedTimeline(timeline(cursor))
		},
		Timeline: func(ctx context.Context, cursor string) (core.Page[core.TimelineItem], error) {
			return svc.Timeline(ctx, timeline(cursor))
		},
		CachedThreads: func(cursor string) (core.ThreadPage, bool) {
			return svc.CachedThreads(threads(cursor))
		},
		Threads: func(ctx context.Context, cursor string) (core.ThreadPage, error) {
			return svc.Threads(ctx, threads(cursor))
		},
	}
	c.voice.Icons = &m.icons
	c.md = markdown.New(m.theme.Thread(m.icons).Markdown)
	c.md.SetGlyphs(m.icons.Markdown)
	opts := []conv.Option{
		conv.WithOptions(m.convOptions()),
		conv.WithTreeKeys(m.keys.tree),
		conv.WithTreeStyles(foldtree.DefaultStyles(m.theme.Dark)),
		conv.WithViewer(c.viewer),
		conv.WithVoice(c.voice),
		conv.WithSize(c.w, c.h),
	}
	if c.opt != nil {
		opts = append(opts, c.opt)
	}
	c.model = conv.New(src, opts...)
	c.started, c.head, c.focused = true, head, false
	m.syncConvFocus()
	m.refreshConv()
	return c.model.Init()
}

// convOptions returns how the rows are drawn.
func (m *panesModal) convOptions() conv.Options {
	c := &m.conv
	return conv.Options{
		Styles: conv.NewStyles(m.theme, m.icons, m.icons.Set()),
		MD:     c.md,
		Dates:  c.dates,
		Now:    c.now(),
	}
}

// restyleConv draws the rows again with the theme and the icons the modal has.
func (m *panesModal) restyleConv() {
	c := &m.conv
	if !c.started {
		return
	}
	c.md.SetStyle(m.theme.Thread(m.icons).Markdown)
	c.md.SetGlyphs(m.icons.Markdown)
	o := m.convOptions()
	c.model.SetOptions(o)
	c.model.SetStyles(o.Styles, foldtree.DefaultStyles(m.theme.Dark))
	m.refreshConv()
}

// readViewer reads the login of the signed-in user.
func (m *panesModal) readViewer(read Viewer) tea.Cmd {
	ctx, id := m.ctx, m.id
	return func() tea.Msg {
		login, err := read(ctx)
		if err != nil || login == "" {
			return nil
		}
		return convViewerMsg{id: id, login: login}
	}
}

// receiveViewer takes the login of the viewer. A conversation that was built
// without it is built again, since it draws the viewer's drafts from the start.
func (m *panesModal) receiveViewer(msg convViewerMsg) tea.Cmd {
	c := &m.conv
	if msg.id != m.id || msg.login == c.viewer {
		return nil
	}
	c.viewer = msg.login
	if !c.started {
		return nil
	}
	return m.buildConv()
}

// convUpdate hands msg to the conversation, which drops what isn't its own.
func (m *panesModal) convUpdate(msg tea.Msg) tea.Cmd {
	c := &m.conv
	if !c.started {
		return nil
	}
	var cmd tea.Cmd
	c.model, cmd = c.model.Update(msg)
	m.refreshConv()
	return cmd
}

// refreshConv counts a change of what the conversation draws, which the
// cached panes then draw again.
func (m *panesModal) refreshConv() {
	c := &m.conv
	stamp := strings.Join(m.convInfo(), "\x00") + "\x00" + c.model.View()
	if stamp != c.stamp {
		c.stamp = stamp
		c.version++
	}
}

// syncConvFocus gives the conversation the focus while its pane has it.
func (m *panesModal) syncConvFocus() {
	c := &m.conv
	want := m.focus == panes.Conversation
	if !c.started || c.focused == want {
		return
	}
	c.focused = want
	if want {
		c.model.Focus()
	} else {
		c.model.Blur()
	}
}

// convPress takes a key of the tree for the conversation, and reports whether
// it was one.
func (m *panesModal) convPress(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	t := m.keys.tree
	var mine bool
	for _, row := range t.FullHelp() {
		for _, b := range row {
			mine = mine || keymap.Matches(msg, b)
		}
	}
	if !mine {
		return nil, false
	}
	return m.convUpdate(msg), true
}

// convInfo is what the pane's title says of the conversation, in the forms
// that fit, the longest first: how many of its items the filter shows, and
// which filter that is.
func (m *panesModal) convInfo() []string {
	c := &m.conv
	if !c.started {
		return nil
	}
	n, total := c.model.Shown()
	if st := c.model.Status(); st.Loading && total == 0 {
		return nil
	}
	name := c.model.FilterName()
	if name == "" {
		name = "all"
	}
	shown := strconv.Itoa(n) + " of " + strconv.Itoa(total) + " shown"
	return []string{shown + m.icons.Separator + "filter: " + name, shown}
}

// convContent is the Conversation pane: the tree of the conversation, sized to
// the box.
func (m *panesModal) convContent(b panes.Box) panes.Content {
	c := &m.conv
	if !c.started {
		return panes.Content{Body: m.theme.Subtle.Render("loading")}
	}
	if in := b.Rect.Inner(); in.W != c.w || in.H != c.h {
		c.w, c.h = in.W, in.H
		c.model.SetSize(c.w, c.h)
	}
	return panes.Content{Body: c.model.View(), Info: m.convInfo()}
}
