package pulls

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// panesIDs numbers the modals, so that each takes the detail it asked for
// and no other's.
var panesIDs atomic.Int64

// panesDetailMsg carries the detail of a pull request to the modal that
// asked for it.
type panesDetailMsg struct {
	id     int64
	detail core.PullRequestDetail
	err    error
}

// panesModal shows a pull request in a modal of six panes under a header.
// It is opened with [Section.openDetail].
type panesModal struct {
	svc  Service
	keys panesKeys

	repo   core.RepoRef
	number int
	// other reports whether repo isn't the one selected, which the title
	// then names.
	other bool

	// detail is what the modal shows, from the cache or the list's row until
	// the read answers, and loaded says there is one.
	detail core.PullRequestDetail
	loaded bool

	// focus is the focused pane, zoom whether it fills the area, and sub
	// which of its list and preview has the focus, per pane.
	focus panes.Pane
	zoom  bool
	sub   [panes.Count]panes.Role

	// slots are what the panes are made of. hover is the row each pane's
	// preview shows, and previews how far each row's preview is scrolled,
	// kept while the modal lives.
	slots    [panes.Count]slot
	hover    [panes.Count]hovered
	previews map[previewKey]int

	// conv is the Conversation pane's tree.
	conv convPane

	// overlays are the pickers, questions and viewers open over the panes, the
	// last on top. Only it takes keys.
	overlays []openOverlay

	// caps is what the viewer may do in repo, as far as it is known, and
	// token says what the token lacks. sending is set while GitHub has a
	// review, and sendCtx bounds the send, which outlives the modal's reads.
	caps    core.RepoCaps
	token   *ui.Token
	sending bool
	sendCtx context.Context

	// found is the path the finder chose, for the Files pane to select, and
	// findErr says why the changed files couldn't be listed.
	found   string
	findErr func(error) (text, hint string)

	// id names the modal to the messages of its reads.
	id int64
	// ctx bounds the reads, and resume lets the reads ahead go on once the
	// modal's have answered.
	ctx    context.Context
	cancel context.CancelFunc
	resume func()
	closed bool

	width, height int
	theme         ui.Theme
	icons         ui.Icons
	st            styles
	frames        panes.Styles
	chips         ui.LabelChips
	// dim styles the panes behind an overlay, and cst the question of a
	// confirm.
	dim lipgloss.Style
	cst ui.ConfirmStyles
	// gen counts the changes of theme and icons, which the drawn panes
	// depend on.
	gen   int
	drawn drawnPanes
}

// openPanes opens a modal on pull request number of repo, on its Merge pane
// if onChecks is set. pr is the list item, shown until the detail arrives, or
// nil when there is none. The title names repo if showRepo is set or it isn't
// the one selected. back is the modal it replaces, which the back key returns
// to, or nil.
func (s *Section) openPanes(repo core.RepoRef, number int, pr *core.PullRequest, onChecks, showRepo bool, from ui.Pauser, back ui.Modal) tea.Cmd {
	// The reads of the modal are one trace.
	ctx, cancel := context.WithCancel(obs.WithTrace(s.ctx, "open.pull"))
	s.ahead.Opened(detailKey(repo, number))
	// The reads ahead wait, so that the detail's requests go first: the
	// list's, and those of the list it was opened from.
	resume := ui.PauseAll(s.ahead, from)
	cachedDetail, cached := s.svc.CachedGet(repo, number)
	slog.InfoContext(ctx, "open", "span", "tui", "kind", "pull", "repo", repo.String(), "number", number, "cached", cached)
	m := &panesModal{
		svc:    s.svc,
		keys:   newPanesKeys(s.rawKeys),
		repo:   repo,
		number: number,
		other:  showRepo || !s.hasRepo || !repo.Same(s.repo),
		focus:  panes.Conversation,
		slots:  slots,
		id:     panesIDs.Add(1),

		previews: map[previewKey]int{},
		ctx:      ctx,
		cancel:   cancel,
		resume:   resume,
		icons:    s.icons,
		conv:     convPane{voice: s.voice, dates: s.dates, now: s.now},

		caps:    s.capsOf(repo),
		token:   s.voice.Token,
		sendCtx: s.ctx,
	}
	if onChecks {
		m.focus = panes.Merge
	}
	findVoice := s.voice
	findVoice.Retry, findVoice.Open = key.Binding{}, key.Binding{}
	m.findErr = ui.ErrorText("list the changed files", core.Target{Repo: repo, Number: number}.String(), findVoice)
	m.SetTheme(s.theme)
	switch {
	case cached:
		m.detail, m.loaded = cachedDetail, true
	case pr != nil:
		m.detail, m.loaded = core.PullRequestDetail{PullRequest: *pr}, true
	}
	// The read starts once the modal is open, so that the app has it to pass
	// the result to.
	reads := []tea.Cmd{m.revalidate()}
	if !m.caps.Known {
		reads = append(reads, ui.LoadCaps(ctx, s.repos, repo))
	}
	if s.readViewer != nil {
		reads = append(reads, m.readViewer(s.readViewer))
	}
	return tea.Sequence(ui.OpenModalOver(m, back), tea.Batch(reads...))
}

// capsOf returns what the viewer may do in repo, as far as it is known.
func (s *Section) capsOf(repo core.RepoRef) core.RepoCaps {
	if s.hasRepo && repo.Same(s.repo) {
		return s.caps
	}
	return ui.CachedCaps(s.repos, repo)
}

// revalidate reads the detail though a cached one is fresh: opening the modal
// reads it again, since its merge state and threads change without the pull
// request's update time moving. What is cached shows meanwhile.
func (m *panesModal) revalidate() tea.Cmd {
	svc, ctx, repo, number, id, resume := m.svc, m.ctx, m.repo, m.number, m.id, m.resume
	return func() tea.Msg {
		start := time.Now()
		d, err := svc.Revalidate(ctx, repo, number)
		resume()
		obs.End(ctx, start, err, "span", "tui", "repo", repo.String(), "number", number)
		return panesDetailMsg{id: id, detail: d, err: err}
	}
}

// Title implements ui.Modal. It is the number only, with the repository
// when it isn't the page's: the header below the frame holds the title.
func (m *panesModal) Title() string {
	n := "#" + strconv.Itoa(m.number)
	if m.other {
		n = m.repo.String() + n
	}
	return n
}

// Link implements ui.Linked.
func (m *panesModal) Link() string { return m.detail.URL }

// Commands implements ui.Commanded: the pull request can be copied.
func (m *panesModal) Commands() []string { return []string{ui.CommandCopy} }

// Selected implements ui.Selector, for the copy command: the pull request
// the modal shows.
func (m *panesModal) Selected() (ui.Selection, bool) {
	return ui.Selection{What: "pull request", URL: m.detail.URL, Repo: m.repo, Number: m.number, Owner: ui.Author(m.detail.Author)}, true
}

// SetSize implements ui.Modal.
func (m *panesModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
}

// Update implements ui.Modal. After the modal closed, it ignores what
// arrives late.
func (m *panesModal) Update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	switch msg := msg.(type) {
	case panesDetailMsg:
		return m.receive(msg)
	case reviewedMsg:
		return m.reviewed(msg)
	case checktree.OpenJobMsg:
		// Whichever pane holds the check row sends it, so the modal answers.
		return func() tea.Msg { return actionsOn(m.repo, msg, m) }
	case checktree.OpenCheckMsg:
		// A check of an App other than GitHub Actions has no modal yet.
		return nil
	case convViewerMsg:
		return m.receiveViewer(msg)
	case conv.EnterMsg:
		// What acting on a row does comes with the changes that act on it.
		return nil
	case ui.CapsMsg:
		if msg.Repo.Same(m.repo) {
			m.caps = msg.Caps
		}
		return nil
	case ui.OnlineMsg:
		// What was served kept shows what GitHub has now.
		if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
			m.detail, m.loaded = d, true
		}
		return m.convUpdate(msg)
	case ui.SettingsMsg:
		msg.ReloadIcons(&m.icons)
		m.conv.dates = ui.NewDates(msg.Config.UI.DateFormat)
		return nil
	case tea.KeyPressMsg:
		if m.top() != nil {
			return m.pressOverlay(msg)
		}
		cmd := m.press(msg)
		m.syncConvFocus()
		m.follow()
		return cmd
	case tea.PasteMsg:
		if m.top() != nil {
			return m.pressOverlay(msg)
		}
	default:
		// What the overlay on top asked for: the messages of a composer.
		if m.top() != nil {
			return m.pressOverlay(msg)
		}
		// The conversation's own: its pages and its tree's.
		return m.convUpdate(msg)
	}
	return nil
}

// receive takes the detail that a read brought. A read that failed keeps what
// the modal shows.
func (m *panesModal) receive(msg panesDetailMsg) tea.Cmd {
	if msg.id != m.id {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		return ui.Fail("load #"+strconv.Itoa(m.number), core.About(core.Target{Repo: m.repo, Number: m.number}.String(), msg.err))
	}
	m.detail, m.loaded = msg.detail, true
	m.follow()
	return m.receiveHead()
}

// press takes a key: the modal's own keys, which move the focus and the
// zoom, and the close key.
func (m *panesModal) press(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	l := m.layout()
	switch p, isDigit := k.digit(msg); {
	case keymap.Matches(msg, k.next):
		m.step(l, 1)
	case keymap.Matches(msg, k.prev):
		m.step(l, -1)
	case isDigit:
		m.focus = p
		m.resetSub()
	case keymap.Matches(msg, k.zoom):
		if m.canZoom() {
			m.zoom = !m.zoom
			m.resetSub()
		}
	case keymap.Matches(msg, k.back):
		return m.close()
	case keymap.Matches(msg, k.review):
		return m.openReview()
	case keymap.Matches(msg, k.find):
		return m.openFind()
	case m.inPreview(l):
		m.scrollPreview(l, msg)
	default:
		if p := m.slots[m.focus-1].press; p != nil {
			cmd, _ := p(m, msg)
			return cmd
		}
	}
	return nil
}

// resetSub gives the list of every pane the focus, as a change of pane or of
// the zoom does.
func (m *panesModal) resetSub() {
	for i := range m.sub {
		m.sub[i] = panes.List
	}
}

// scrollPreview scrolls the focused pane's preview, which has the focus in l,
// as far as its text goes. The offset stays with the row shown.
func (m *panesModal) scrollPreview(l panes.Layout, msg tea.KeyPressMsg) {
	b, ok := l.Box(m.focus, panes.Preview)
	s := m.slots[m.focus-1]
	if !ok || s.preview == nil {
		return
	}
	rows := b.Rect.Inner().H
	last := max(0, strings.Count(s.preview(m, b), "\n")+1-rows)
	k := m.keys.scroll
	pk := previewKey{m.focus, m.hover[m.focus-1].id}
	off := m.previews[pk]
	switch {
	case keymap.Matches(msg, k.Up):
		off--
	case keymap.Matches(msg, k.Down):
		off++
	case keymap.Matches(msg, k.PageUp):
		off -= rows
	case keymap.Matches(msg, k.PageDown):
		off += rows
	case keymap.Matches(msg, k.HalfPageUp):
		off -= max(1, rows/2)
	case keymap.Matches(msg, k.HalfPageDown):
		off += max(1, rows/2)
	case keymap.Matches(msg, k.Home):
		off = 0
	case keymap.Matches(msg, k.End):
		off = last
	default:
		return
	}
	m.previews[pk] = min(max(off, 0), last)
}

// layout returns the layout of the panes.
func (m *panesModal) layout() panes.Layout {
	return panes.Compute(panes.Spec{Width: m.width, Height: max(m.height-2, 0), Focus: m.focus, Zoom: m.zoom})
}

// canZoom reports whether the panes are wide enough to zoom: below that
// they are zoomed anyway.
func (m *panesModal) canZoom() bool { return m.width >= panes.WideMin }

// inPreview reports whether the preview of the focused pane is shown in l
// and has the focus.
func (m *panesModal) inPreview(l panes.Layout) bool {
	if m.sub[m.focus-1] != panes.Preview {
		return false
	}
	_, ok := l.Box(m.focus, panes.Preview)
	return ok
}

// canStep reports whether the keys that move the focus do anything: they
// move between the panes, or between the list and the preview of a zoomed
// pane that has one.
func (m *panesModal) canStep(l panes.Layout) bool {
	if !l.Zoomed {
		return true
	}
	_, ok := l.Box(m.focus, panes.Preview)
	return ok
}

// step moves the focus by delta panes, wrapping, or between the list and the
// preview of a zoomed pane.
func (m *panesModal) step(l panes.Layout, delta int) {
	switch {
	case !l.Zoomed:
		m.focus = panes.Pane((int(m.focus)-1+delta+panes.Count)%panes.Count + 1)
		m.resetSub()
	case m.canStep(l):
		i := m.focus - 1
		if m.sub[i] == panes.Preview {
			m.sub[i] = panes.List
		} else {
			m.sub[i] = panes.Preview
		}
	}
}

// context returns the context of the keys of what has the focus in l.
func (m *panesModal) context(l panes.Layout) string {
	if m.inPreview(l) {
		return ctxPreview
	}
	return m.slots[m.focus-1].context
}

// close closes the modal, and stops its reads.
func (m *panesModal) close() tea.Cmd {
	m.Discard()
	return ui.CloseModal(m)
}

// Discard implements ui.Discarder, for a modal the app drops while it is
// hidden, such as the one a chain of modals leaves behind: the reads end, and
// those held back for the modal go on.
func (m *panesModal) Discard() {
	m.closed = true
	if m.conv.started {
		m.conv.model.Close()
	}
	m.cancel()
	m.resume()
}
