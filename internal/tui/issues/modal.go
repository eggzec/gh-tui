package issues

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// issueMsg carries the issue that Get read for the thread that asked.
type issueMsg struct {
	thread int64
	issue  core.Issue
	err    error
}

// changedMsg tells the section that a modal changed an issue of repo in the
// cache, so that the list behind it shows the change at once.
type changedMsg struct {
	repo core.RepoRef
}

// detailModal shows an issue with its comments in a modal, and changes it.
// It is opened with [Section.openDetail].
type detailModal struct {
	svc  Service
	keys keyMap
	now  func() time.Time
	// links confirms the references of the bodies, and drew counts the
	// answers it has read.
	links *ui.Autolinks
	drew  uint64
	// sendCtx bounds the changes, which go on after the modal closes.
	sendCtx context.Context

	repo   core.RepoRef
	number int
	// other reports whether repo isn't the one selected, which the title
	// then names.
	other bool
	// caps is what the viewer may do in repo, as far as it is known, and
	// viewer who they are, or empty.
	caps   core.RepoCaps
	viewer string
	// token is what the token may do.
	token *ui.Token
	// issue is what is known of the issue, and loaded says whether
	// anything is: a modal opened from the search starts with nothing.
	issue  core.Issue
	loaded bool
	// failed is why the last read of the issue failed, or nil.
	failed error

	// id names the modal to its reads, conv is the conversation under the
	// header, which convOn says is made, and head is the header drawn.
	id     int64
	conv   conv.Model
	convOn bool
	// peek is the conversation drawn as a preview, and peekVer counts the
	// changes of the conversation that draw it again.
	peek      drawnPeek
	peekVer   uint64
	peekStamp int
	head      string
	voice     ui.Voice
	md        *markdown.Renderer
	// pics draws the images of the bodies and comments, or is nil. picRows
	// is the tallest they were drawn at.
	pics    *ui.Images
	picRows int
	seen    *ui.Seen
	// side holds the Involved and Development panes, and refSvc reads the
	// links of the Development one.
	side   *sidePanes
	refSvc refs.Service
	// mu guards pending, which the conversation's reads take from a command's
	// goroutine: the comments sent that GitHub hasn't confirmed.
	mu      sync.Mutex
	pending []core.TimelineItem
	sent    int
	// pagesRead counts the timeline pages the conversation read, and pages is
	// the count the Involved pane shows.
	pagesRead atomic.Int64
	pages     int64
	// newRefs makes the References step, which refs is while it shows in
	// place of the thread; newRefs is nil without a service for the links.
	newRefs func() *refs.Step
	refs    *refs.Step
	// ctx bounds the reads of the modal and is cancelled when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	// resume lets the reads ahead of the list go on, once the issue has
	// loaded or the modal closed.
	resume func()

	// edit is the comment the open composer rewrites, or nil for a new one.
	edit *editing
	// drafts keeps the text of a comment between sessions, or is nil.
	drafts composer.Drafts
	// ask is the change waiting for the user to confirm it, on the last
	// line, in the styles of confirmSt.
	ask *confirm.Ask
	// over is the picker open over the panes, which takes every key.
	over reactOverlay
	// facets reads the labels of the repository for the labels picker, or is
	// nil when the section has no source for them.
	facets Facets
	// armed is set once the question is armed: until then enter answers
	// nothing.
	armed     bool
	confirmSt confirm.Styles

	width, height int
	theme         ui.Theme
	rows          rowStyles
	icons         ui.Icons
	dates         ui.Dates
	chips         ui.LabelChips

	// nav is the focused pane, zoom whether it fills the area, narrow whether
	// the app asks for one pane at a time, and frames draws the boxes.
	nav    panes.Nav
	zoom   panes.Zoom
	narrow bool
	frames panes.Styles
	// dividers are the dividers shared with the other views, or nil.
	dividers *ui.Dividers
}

// modalIDs numbers the modals.
var modalIDs atomic.Int64

// openDetail opens a modal on issue number of repo. it is the list item,
// shown until the issue arrives, or nil when there is none. back is the
// modal it replaces, which the back key returns to, or nil.
func (s *Section) openDetail(repo core.RepoRef, number int, it *core.Issue, showRepo bool, from ui.Pauser, back ui.Modal) tea.Cmd {
	// The reads of the modal are one trace, however many pages it reads.
	ctx, cancel := context.WithCancel(obs.WithTrace(s.ctx, "open.issue"))
	s.ahead.Opened(detailKey(repo, number))
	// The reads ahead wait, so that the issue's requests go first: the
	// list's, and those of the list it was opened from.
	resume := ui.PauseAll(s.ahead, from)
	_, cached := s.svc.CachedGet(repo, number)
	slog.InfoContext(ctx, "open", "span", "tui", "kind", "issue", "repo", repo.String(), "number", number, "cached", cached)
	m := &detailModal{
		svc:     s.svc,
		keys:    s.keys.forModal(s.rawKeys),
		now:     s.now,
		links:   s.autolinks,
		sendCtx: s.ctx,
		repo:    repo,
		number:  number,
		other:   showRepo || !s.hasRepo || !repo.Same(s.repo),
		caps:    s.capsOf(repo),
		viewer:  s.viewer,
		token:   s.voice.Token,
		ctx:     ctx,
		cancel:  cancel,
		resume:  resume,
		theme:   s.theme,
		rows:    s.rows,
		icons:   s.icons,
		dates:   s.dates,
		chips:   s.rows.newChips(),
		id:      modalIDs.Add(1),
		voice:   s.voice,
		facets:  s.facets,
		drafts:  s.drafts,

		dividers: s.dividers,
		pics:     s.avatars,
	}
	m.voice.Icons = &m.icons
	m.nav.SetFocus(paneConversation)
	m.side, m.refSvc = newSide(s.rawKeys), s.refs
	m.side.dev.SetRepo(repo)
	m.side.deps = m.newDeps(s.rawKeys)
	m.confirmSt = s.theme.Confirm(s.icons)
	m.keys.References.SetEnabled(m.keys.References.Enabled() && s.refs != nil)
	if s.refs != nil {
		svc, keys := s.refs, s.rawKeys
		opts := append(slices.Clone(s.refsOpts), refs.WithReturn(m), refs.WithIcons(s.icons), refs.WithClock(s.now),
			refs.WithItem(func() refs.Item { return refs.Item{Title: m.issue.Title, Updated: m.issue.UpdatedAt} }))
		m.newRefs = func() *refs.Step {
			// The icons the modal has now, which :set may have changed.
			return refs.New(m.ctx, svc, core.Target{Repo: repo, Number: number, Kind: core.KindIssue}, false, keys, append(slices.Clone(opts), refs.WithIcons(m.icons))...)
		}
	}
	svc := s.svc
	switch cached, ok := svc.CachedGet(repo, number); {
	case ok:
		m.issue, m.loaded = cached, true
	case it != nil:
		m.issue, m.loaded = *it, true
	}
	var caps tea.Cmd
	if !m.caps.Known {
		// The app reads those of the selected repository.
		caps = ui.LoadCaps(ctx, s.repos, repo)
	}
	if !m.loaded {
		// The loads start once the modal is open, so that the app has it
		// to pass their results to.
		return tea.Sequence(ui.OpenModalOver(m, back), tea.Batch(m.buildConv(), m.get(), caps, m.readLinks(false), m.side.deps.Init()))
	}
	// What is cached shows at once, and is read again behind it.
	loads := []tea.Cmd{m.show(), m.buildConv(), m.get(), caps, m.readLinks(false), m.side.deps.Init()}
	return tea.Sequence(ui.OpenModalOver(m, back), tea.Batch(loads...))
}

// capsOf returns what the viewer may do in repo, as far as it is known.
func (s *Section) capsOf(repo core.RepoRef) core.RepoCaps {
	if s.hasRepo && repo.Same(s.repo) {
		return s.caps
	}
	return ui.CachedCaps(s.repos, repo)
}

// detailKey names issue number of repo to the reads ahead.
func detailKey(repo core.RepoRef, number int) details.Key {
	return details.Key{Repo: repo, Number: number}
}

// Title implements ui.Modal. It is the number only, with the repository
// when it isn't the page's: the header below the frame holds the title.
func (m *detailModal) Title() string {
	n := "#" + strconv.Itoa(m.number)
	if m.other {
		n = m.repo.String() + n
	}
	return n
}

// Link implements ui.Linked.
func (m *detailModal) Link() string { return m.issue.URL }

// Commands implements ui.Commanded: the issue can be copied, and its links
// shown if the section reads them.
func (m *detailModal) Commands() []string {
	if m.newRefs == nil {
		return []string{ui.CommandCopy}
	}
	return []string{ui.CommandCopy, ui.CommandReferences}
}

// Selected implements ui.Selector, for the copy command: the issue the
// modal shows.
func (m *detailModal) Selected() (ui.Selection, bool) {
	return ui.Selection{What: "issue", URL: m.issue.URL, Repo: m.repo, Number: m.number, Owner: ui.Author(m.issue.Author)}, true
}

// SetSize implements ui.Modal.
func (m *detailModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.zoom.SetNarrow(m.narrow || m.width < wideMin)
	m.settleDividers()
	if !m.nav.InPreview(m.paneLayout()) {
		// A preview that a resize took away must not keep the focus.
		m.nav.Reset()
	}
	m.layout()
	if m.refs != nil {
		m.refs.SetSize(m.width, m.height)
	}
	if m.loaded {
		// The header wraps its title to the width.
		_ = m.show()
	}
}

// SetTheme implements ui.Modal. It builds every style the modal uses.
func (m *detailModal) SetTheme(t ui.Theme) {
	m.theme = t
	if m.refs != nil {
		m.refs.SetTheme(t)
	}
	m.confirmSt = t.Confirm(m.icons)
	if o := m.composerOpen(); o != nil {
		o.Comp.SetStyles(t.Prompt(m.icons))
	}
	m.frames = panes.DefaultStyles(t.Dark).WithAccent(t.Accent)
	m.frames.Ellipsis = m.icons.Ellipsis
	m.frames.Separator = m.icons.Separator
	m.rows = newRowStyles(t, m.icons)
	m.chips = m.rows.newChips()
	m.restyleConv()
	m.refreshSide()
	if m.loaded {
		// The header is styled too.
		_ = m.show()
	}
}

// convOptions returns how the rows are drawn.
func (m *detailModal) convOptions() conv.Options {
	// The images of the bodies draw as the app's images setting says: none
	// while it is off.
	// A pane with no height (another is zoomed) keeps its pictures.
	if h := m.convRect().H; h > 0 || m.picRows == 0 {
		m.picRows = m.pics.PictureRows(h)
	}
	if m.md != nil {
		m.md.SetPictures(m.pics.Pictures(m.picRows, nil))
	}
	return conv.Options{
		Styles: conv.NewStyles(m.theme, m.icons, m.icons.Set()),
		MD:     m.md,
		Dates:  m.dates,
		Now:    m.now(),
	}
}

// restyleConv draws the rows again with the theme and the icons the modal has.
func (m *detailModal) restyleConv() {
	if !m.convOn {
		return
	}
	m.md.SetStyle(m.theme.Thread(m.icons).Markdown)
	m.md.SetGlyphs(m.icons.Markdown)
	m.peekVer++
	o := m.convOptions()
	m.conv.SetOptions(o)
	m.conv.SetStyles(o.Styles, treeview.DefaultStyles(m.theme.Dark))
	m.peekStamp = m.conv.Version()
}

// source reads the timeline of the issue, with the comments being sent as the
// last rows.
func (m *detailModal) source() conv.Source {
	svc, repo, number := m.svc, m.repo, m.number
	query := func(cursor string) issuesvc.TimelineQuery {
		return issuesvc.TimelineQuery{Repo: repo, Number: number, Cursor: cursor}
	}
	return conv.Source{
		CachedTimeline: func(cursor string) (core.Page[core.TimelineItem], bool) {
			p, ok := svc.CachedTimeline(query(cursor))
			return m.decorate(p, cursor), ok
		},
		Timeline: func(ctx context.Context, cursor string) (core.Page[core.TimelineItem], error) {
			p, err := svc.Timeline(ctx, query(cursor))
			m.pagesRead.Add(1)
			return m.decorate(p, cursor), err
		},
	}
}

// decorate adds the comments being sent to the last page, on a copy.
func (m *detailModal) decorate(p core.Page[core.TimelineItem], _ string) core.Page[core.TimelineItem] {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := slices.Clone(p.Items)
	if p.Next == "" {
		items = append(items, m.pending...)
	}
	p.Items = items
	return p
}

// buildConv makes the conversation, closing the one it replaces, and returns
// its first reads.
func (m *detailModal) buildConv() tea.Cmd {
	if m.convOn {
		m.conv.Close()
	}
	m.md = ui.NewMarkdown(m.theme, m.icons)
	m.seen = m.links.Seen(m.repo.String())
	m.md.SetAutolink(m.seen.Func())
	m.conv = conv.New(m.source(), conv.HostOptions(m.convOptions(), m.keys.tree, m.theme.Dark, m.viewer, m.voice, m.convRect().W, m.convRect().H)...)
	m.convOn = true
	m.peekStamp = m.conv.Version()
	m.peekVer++
	m.syncConvFocus()
	return m.conv.Init()
}

// View implements ui.Modal. It renders exactly the modal's size.
func (m *detailModal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	v := m.composed()
	if m.refs != nil {
		v = m.refs.View()
	}
	if m.over != nil {
		v = m.covered(v)
	}
	if m.ask == nil {
		return v
	}
	// The question is centred over the modal, as in the pull request's.
	return ui.OverCentred(v, *m.ask, m.confirmSt, m.keys.confirm, m.width, m.height)
}

// Update implements ui.Modal. After the modal closed, it ignores what
// arrives late.
func (m *detailModal) Update(msg tea.Msg) tea.Cmd {
	cmd := m.update(msg)
	if m.pagesRead.Load() != m.pages {
		// A page of the timeline came in after the issue: its commenters.
		m.refreshInvolved()
	}
	if !m.convOn {
		return cmd
	}
	// What rendered since names references that may be links.
	return tea.Batch(cmd, m.seen.Ask(m.ctx))
}

// convUpdate hands msg to the conversation, which drops what isn't its own.
func (m *detailModal) convUpdate(msg tea.Msg) tea.Cmd {
	if !m.convOn {
		return nil
	}
	var cmd tea.Cmd
	m.conv, cmd = m.conv.Update(msg)
	m.convChanged()
	return cmd
}

// convChanged counts a change of what the conversation draws, which the
// preview then draws again. A message that leaves the view as it was counts
// for nothing.
func (m *detailModal) convChanged() {
	if !m.convOn {
		return
	}
	if stamp := m.conv.Version(); stamp != m.peekStamp {
		m.peekStamp = stamp
		m.peekVer++
	}
}

func (m *detailModal) update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	if ui.IsArm(msg) {
		m.armed = m.ask != nil
		return nil
	}
	if m.over != nil {
		switch msg.(type) {
		case picker.ChosenMsg, picker.CancelMsg, picker.LockedMsg, tea.PasteMsg, tea.MouseMsg:
			// A mouse event goes to the overlay too, so none reaches the
			// panes it covers.
			return m.pressOver(msg)
		}
	}
	switch msg := msg.(type) {
	case ui.SettingsMsg:
		msg.ReloadIcons(&m.icons)
		if m.refs != nil {
			m.refs.SetIcons(m.icons)
		}
		return nil
	case ui.ImagesMsg:
		// What the images are, or whether they draw, changed.
		if m.convOn {
			m.conv.SetOptions(m.convOptions())
			m.peekVer++
		}
		if m.side != nil {
			m.side.desc.pics++
		}
		return nil
	case tea.KeyPressMsg:
		return m.press(msg)
	case labelsReadMsg:
		return m.labelsRead(msg)
	case issueMsg:
		cmd := m.gotIssue(msg)
		m.refreshSide()
		return cmd
	case ui.SyncMsg:
		if m.links.Synced(msg, &m.drew) {
			if m.convOn {
				m.conv.SetOptions(m.convOptions())
				m.peekVer++
			}
			return nil
		}
		if msg.Err != nil || msg.Key != issuesvc.SyncKey(m.repo) {
			return nil
		}
		return tea.Batch(m.buildConv(), m.get())
	case refs.CloseMsg:
		// The step has nothing left to clear: the modal closes.
		if m.refs != nil && msg.ID == m.refs.ID() {
			return m.close()
		}
		return nil
	case ui.ReopenedMsg:
		// Back from the modal that replaced this one: what is cached
		// shows at once, and is read again behind it, with the links if
		// they show.
		if msg.Modal != m {
			return nil
		}
		var step tea.Cmd
		if m.refs != nil {
			step = m.refs.Update(msg)
		}
		return tea.Batch(m.buildConv(), m.get(), step)
	case ui.CapsMsg:
		if msg.Repo.Same(m.repo) {
			m.caps = msg.Caps
		}
		return nil
	case conv.EnterMsg:
		return nil
	case linksMsg:
		return m.receiveLinks(msg)
	case mentionsMsg:
		return m.receiveMentions(msg)
	case treeview.OpenMsg:
		if cmd := m.enterSide(msg); cmd != nil {
			return cmd
		}
	case viewerMsg:
		m.viewer = msg.login
		return nil
	case ui.OnlineMsg:
		return m.online()
	case ui.DoneMsg:
		// The conversation is read again too, since the change may be a
		// comment, which the service has not put in its timeline.
		if msg.From != ui.IssuesTitle {
			return nil
		}
		m.mu.Lock()
		sent := len(m.pending) > 0
		m.pending = nil
		m.mu.Unlock()
		if sent {
			m.svc.Invalidate(m.repo)
		}
		return tea.Batch(m.reload(), m.buildConv())
	case composer.DiscardedMsg:
		return nil
	}
	if _, ok := msg.(tea.MouseMsg); ok && m.refs != nil {
		// The links cover the thread, which would scroll under the mouse.
		return nil
	}
	var over tea.Cmd
	if m.composerOpen() != nil {
		// The composer's own messages (its submit, its cancel, a draft
		// read, the cursor) come back here and belong to it.
		over = m.pressOver(msg)
	}
	cmd := tea.Batch(over, m.convUpdate(msg))
	if m.side != nil {
		// The loads and the spinner of the Blockers tree.
		var deps tea.Cmd
		m.side.deps, deps = m.side.deps.Update(msg)
		cmd = tea.Batch(cmd, deps)
	}
	if m.refs != nil {
		cmd = tea.Batch(cmd, m.refs.Update(msg))
	}
	if m.over != nil {
		// Every other message reaches the open overlay: the answers of the search
		// of a picker, and the blink and editor results of the description's
		// fields, which were dropped before.
		cmd = tea.Batch(cmd, m.pressOver(msg))
	}
	return cmd
}

func (m *detailModal) press(msg tea.KeyPressMsg) tea.Cmd {
	if m.ask != nil {
		return m.answer(msg)
	}
	if m.over != nil {
		return m.pressOver(msg)
	}
	if m.refs != nil {
		// The links take every key; their own are the keys of the modal's
		// that apply to them.
		return m.refs.Update(msg)
	}
	k := m.keys
	if m.pressPane(msg) {
		return nil
	}
	switch {
	case keymap.Matches(msg, k.References) && m.newRefs != nil:
		return m.openRefs()
	case keymap.Matches(msg, k.Back):
		return m.close()
	case keymap.Matches(msg, k.owner) && ui.Author(m.issue.Author) != "":
		return m.author()
	case keymap.Matches(msg, k.Comment):
		return m.openComposer(nil)
	case keymap.Matches(msg, k.Label):
		return m.openLabels()
	case keymap.Matches(msg, k.Edit) && m.nav.Focus() == paneConversation:
		return m.editComment()
	case keymap.Matches(msg, k.Edit) && m.nav.Focus() == paneDescription:
		return m.openDescEdit()
	case keymap.Matches(msg, k.Delete) && m.nav.Focus() == paneConversation:
		return m.deleteComment()
	case keymap.Matches(msg, k.React) && m.nav.Focus() == paneConversation:
		return m.openReact()
	case keymap.Matches(msg, k.Close):
		return m.setState(core.StateClosed)
	case keymap.Matches(msg, k.Reopen):
		return m.setState(core.StateOpen)
	case keymap.Matches(msg, k.Refresh):
		m.svc.Invalidate(m.repo)
		return tea.Batch(m.buildConv(), m.get())
	}
	if cmd, ok := m.pressSide(msg); ok {
		return cmd
	}
	if m.nav.Focus() != paneConversation {
		return nil
	}
	return m.convUpdate(msg)
}

// close closes the modal, and stops its reads.
func (m *detailModal) close() tea.Cmd {
	m.Discard()
	return ui.CloseModal(m)
}

// Discard implements ui.Discarder, for a modal the app drops while it is
// hidden, such as the one a chain of modals leaves behind: its reads end,
// and those held back for it go on.
func (m *detailModal) Discard() {
	if m.refs != nil {
		m.refs.Close()
		m.refs = nil
	}
	m.over = nil
	m.closed = true
	m.cancel()
	m.resume()
}

// get reads the issue, since a list page may carry less than the issue
// itself, such as no body. What failed before is forgotten, so that GitHub
// answering again doesn't read it once more while this read is under way.
func (m *detailModal) get() tea.Cmd {
	m.failed = nil
	svc, repo, number, id, ctx, resume := m.svc, m.repo, m.number, m.id, m.ctx, m.resume
	return func() tea.Msg {
		start := time.Now()
		it, err := svc.Get(ctx, repo, number)
		resume()
		obs.End(ctx, start, err, "span", "tui", "repo", repo.String(), "number", number)
		return issueMsg{thread: id, issue: it, err: err}
	}
}

// online reads again, now that GitHub answers again, the issue and the
// comments that failed for want of an answer from it.
func (m *detailModal) online() tea.Cmd {
	var get, again tea.Cmd
	if ui.Unreached(m.failed) {
		get = m.get()
	}
	if m.convOn && ui.Unreached(m.conv.Status().Err) {
		again = m.buildConv()
	}
	return tea.Batch(get, again)
}

// gotIssue shows the issue that Get read for this modal.
func (m *detailModal) gotIssue(msg issueMsg) tea.Cmd {
	if msg.thread != m.id {
		return nil
	}
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			return nil
		}
		m.failed = msg.err
		return ui.Fail("load #"+strconv.Itoa(m.number), core.About(m.subject(), msg.err))
	}
	newer := m.loaded && msg.issue.UpdatedAt.After(m.issue.UpdatedAt)
	m.issue, m.loaded, m.failed = msg.issue, true, nil
	if newer {
		// The links are read again for what changed: the pane's, by the
		// update time in its query, and the step's if it is open.
		if m.refs != nil {
			return tea.Batch(m.show(), m.refs.Reread(), m.readLinks(false))
		}
		return tea.Batch(m.show(), m.readLinks(false))
	}
	return m.show()
}

// setState closes or reopens the issue, once the user confirms it on the
// last line of the modal. The service shows the change in its cache at
// once, so the modal and the list behind it show it from there, then the
// change is sent.
func (m *detailModal) setState(state core.State) tea.Cmd {
	if !m.loaded {
		return nil
	}
	c, ok, refusal := stateChange(m.svc, m.gate(), m.issue, state,
		func() (core.Issue, ui.Gate, bool) { return m.issue, m.gate(), m.loaded },
		func(op *optimistic.Op, what string) tea.Cmd {
			return tea.Batch(m.reload(), m.changed(), m.send(op, what))
		})
	if !ok {
		return refusal
	}
	return m.asks(c)
}

// asks puts c on the last line, to be armed once the app hands the modal the
// message that arms it.
func (m *detailModal) asks(c confirm.Ask) tea.Cmd {
	m.ask = &c
	m.armed = false
	return ui.Arm()
}

// answer takes the answer to the question on the last line: yes makes the
// change, and no steps back to the issue. Other keys do nothing.
func (m *detailModal) answer(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := m.keys.confirm.Answer(m.ask, msg, m.armed)
	if done {
		m.ask = nil
	}
	return cmd
}

// gate decides what the viewer may do in the repository.
func (m *detailModal) gate() ui.Gate {
	return ui.Gate{Repo: m.repo, Caps: m.caps, Viewer: m.viewer, Token: m.token, Icons: m.icons}
}

// changed tells the section that the issue changed in the cache.
func (m *detailModal) changed() tea.Cmd {
	repo := m.repo
	return func() tea.Msg { return changedMsg{repo: repo} }
}

// send sends op, the change what, such as "close #5", of the issue, whose
// failure the app tells of naming the issue.
func (m *detailModal) send(op ui.Op, what string) tea.Cmd {
	return ui.Do(m.sendCtx, ui.IssuesTitle, ui.About(m.subject(), op), what)
}

// subject names the issue in what the user reads of a failure, such as
// "eggzec/gh-tui#5", since GitHub may not find it or refuse access to it
// while it finds its repository.
func (m *detailModal) subject() string {
	return core.Target{Repo: m.repo, Number: m.number}.String()
}

// reload shows the issue from the cache again.
func (m *detailModal) reload() tea.Cmd {
	it, ok := m.svc.CachedGet(m.repo, m.number)
	if !ok {
		return nil
	}
	m.issue, m.loaded = it, true
	return m.show()
}

// show draws the header from the issue; the Description pane shows its text.
func (m *detailModal) show() tea.Cmd {
	m.head = m.header(m.issue)
	m.layout()
	return nil
}

// header renders the head of the detail: the strip of the labels, or nothing
// for an issue without any. The state, the number and the title are in the
// top edge of the frame.
func (m *detailModal) header(it core.Issue) string {
	if len(it.Labels) == 0 {
		return ""
	}
	return "  " + ui.LabelStrip(it.Labels, m.chips, max(m.width-4, 1), func(s string) string { return m.theme.Subtle.Render(s) })
}

// TrimTitle implements ui.Trimmed: the state and the title join the number in
// the top edge of the frame, once they are known.
func (m *detailModal) TrimTitle() ui.TrimTitle {
	t := ui.TrimTitle{Name: m.Title()}
	if m.loaded {
		t.Lead = m.rows.badges[ui.IssueState(m.issue)]
		t.Detail = m.issue.Title
	}
	return t
}
