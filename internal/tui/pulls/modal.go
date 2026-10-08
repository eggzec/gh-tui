package pulls

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
)

// detailMsg carries the detail of a pull request to the thread that asked
// for it.
type detailMsg struct {
	thread int64
	detail core.PullRequestDetail
	err    error
}

// changedMsg tells the section that a modal changed a pull request of repo
// in the cache, so that the list behind it shows the change at once.
type changedMsg struct {
	repo core.RepoRef
}

// detailModal shows a pull request in a modal, as tabs: its conversation,
// and its checks, and changes it. It is opened with
// [Section.openDetail].
type detailModal struct {
	svc         Service
	keys        keyMap
	now         func() time.Time
	mergeMethod core.MergeMethod
	// sendCtx bounds the changes, which go on after the modal closes.
	sendCtx context.Context

	repo   core.RepoRef
	number int
	// other reports whether repo isn't the one selected, which the title
	// then names.
	other bool
	// caps is what the viewer may do in repo, as far as it is known.
	caps core.RepoCaps
	// token is what the token may do.
	token *ui.Token
	// detail is what is known of the pull request, and loaded says whether
	// anything is: a modal opened from the search starts with nothing.
	detail core.PullRequestDetail
	loaded bool
	// failed is why the last read of the detail failed, or nil.
	failed error

	thread thread.Model[core.Comment]
	// ctx bounds the reads of the modal and is cancelled when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	// resume lets the reads ahead of the list go on, once the detail has
	// loaded or the modal closed.
	resume func()

	// tab is the tab shown. checksSvc reads the checks, and newChecks makes
	// the Checks step, which checks is once the tab has been shown;
	// newChecks is nil without checks.
	tab       modalTab
	checksSvc checks.Service
	newChecks func() *checks.Step
	checks    *checks.Step
	// ask is the change waiting for the user to confirm it, on the last
	// line, in the styles of confirmSt.
	ask       *ui.Confirm
	confirmSt ui.ConfirmStyles

	width, height int
	theme         ui.Theme
	st            styles
	runSt         ui.RunStyles
	icons         ui.Icons
	dates         ui.Dates
	avatars       *ui.Images
	// picRows is the tallest the thread draws the images of markdown,
	// or 0 while it draws none, and bodies those it draws them of.
	picRows int
	bodies  *ui.ImageBodies
}

// openDetail opens a modal on pull request number of repo, on its checks
// if onChecks is set and the section has them. pr is the list item, shown
// until the detail arrives, or nil when there is none. The title names
// repo if showRepo is set or it isn't the one selected.
func (s *Section) openDetail(repo core.RepoRef, number int, pr *core.PullRequest, onChecks, showRepo bool, from ui.Pauser) tea.Cmd {
	// The reads of the modal are one trace, however many pages it reads.
	ctx, cancel := context.WithCancel(obs.WithTrace(s.ctx, "open.pull"))
	s.ahead.Opened(detailKey(repo, number))
	// The reads ahead wait, so that the detail's requests go first: the
	// list's, and those of the list it was opened from.
	resume := ui.PauseAll(s.ahead, from)
	_, cached := s.svc.CachedGet(repo, number)
	slog.InfoContext(ctx, "open", "span", "tui", "kind", "pull", "repo", repo.String(), "number", number, "cached", cached)
	modalKeys := s.keys.forModal(s.rawKeys)
	modalKeys.Checks.SetEnabled(modalKeys.Checks.Enabled() && s.checks != nil)
	m := &detailModal{
		svc:         s.svc,
		keys:        modalKeys,
		now:         s.now,
		mergeMethod: s.mergeMethod,
		sendCtx:     s.ctx,
		repo:        repo,
		number:      number,
		other:       showRepo || !s.hasRepo || !repo.Same(s.repo),
		caps:        s.capsOf(repo),
		token:       s.voice.Token,
		ctx:         ctx,
		cancel:      cancel,
		resume:      resume,
		st:          s.st,
		icons:       s.icons,
		dates:       s.dates,
		avatars:     s.avatars,
		bodies:      ui.NewImageBodies(s.capsOf(repo).Private),
		checksSvc:   s.checks,
	}
	m.theme, m.runSt, m.confirmSt = s.theme, ui.NewRunStyles(s.theme, s.icons), s.theme.Confirm(s.icons)
	if s.checks != nil {
		svc, keys := s.checks, s.rawKeys
		opts := append(slices.Clone(s.checksOpts), checks.WithReturn(m), checks.WithIcons(s.icons), checks.WithClock(s.now))
		m.newChecks = func() *checks.Step {
			return checks.New(m.sendCtx, svc, repo, number, keys, append(opts, checks.WithCaps(m.caps))...)
		}
	}
	var step tea.Cmd
	if onChecks && m.newChecks != nil {
		m.tab = checksTab
		step = m.showChecks()
	}
	if !m.caps.Known {
		// The app reads those of the selected repository.
		step = tea.Batch(step, ui.LoadCaps(ctx, s.repos, repo))
	}
	svc, q := s.svc, commentsQuery(repo, number)
	fetch := func(ctx context.Context, cursor string) ([]core.Comment, string, error) {
		q := q
		q.Cursor = cursor
		p, err := svc.Comments(ctx, q)
		return p.Items, p.Next, err
	}
	// The thread fetches failed comments again with its own retry key.
	v := s.voice
	v.Retry = s.keys.thread.Retry
	m.thread = thread.New(fetch, m.renderComment,
		thread.WithContext(ctx),
		thread.WithStyles(s.theme.Thread(s.icons)),
		thread.WithKeyMap(s.keys.thread),
		thread.WithFocused(true),
		thread.WithErrorText(ui.ErrorText("load the comments", core.Target{Repo: repo, Number: number}.String(), v)),
	)
	m.thread.SetCutHint(ui.OpenHint(s.icons, s.keys.Open))
	m.drawPictures()
	switch d, ok := svc.CachedGet(repo, number); {
	case ok:
		m.detail, m.loaded = d, true
	case pr != nil:
		m.detail, m.loaded = core.PullRequestDetail{PullRequest: *pr}, true
	}
	if !m.loaded {
		// The loads start once the modal is open, so that the app has it
		// to pass their results to.
		return tea.Sequence(ui.OpenModal(m), tea.Batch(m.thread.Init(), m.get(), step))
	}
	// What is cached shows at once, and is read again behind it.
	cp, primed := svc.CachedComments(q)
	if primed {
		m.thread.SetFirst(cp.Items, cp.Next)
	}
	loads := []tea.Cmd{m.show(), m.get(), step}
	if primed {
		loads = append(loads, m.thread.Reload())
	}
	return tea.Sequence(ui.OpenModal(m), tea.Batch(loads...))
}

// capsOf returns what the viewer may do in repo, as far as it is known.
func (s *Section) capsOf(repo core.RepoRef) core.RepoCaps {
	if s.hasRepo && repo.Same(s.repo) {
		return s.caps
	}
	return ui.CachedCaps(s.repos, repo)
}

// detailKey names pull request number of repo to the reads ahead.
func detailKey(repo core.RepoRef, number int) details.Key {
	return details.Key{Pull: true, Repo: repo, Number: number}
}

// commentsQuery selects the first page of the comments on pull request
// number of repo, as the modal reads it, so that what is read ahead is what
// the modal finds in the cache.
func commentsQuery(repo core.RepoRef, number int) pulls.CommentsQuery {
	return pulls.CommentsQuery{Repo: repo, Number: number}
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
func (m *detailModal) Link() string { return m.detail.URL }

// SetSize implements ui.Modal.
func (m *detailModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.thread.SetSize(m.width, m.height)
	m.drawPictures()
	if m.checks != nil {
		m.checks.SetSize(m.width, m.height)
	}
	if m.loaded {
		// The next Update loads what the new size shows.
		_ = m.show()
	}
}

// SetTheme implements ui.Modal.
func (m *detailModal) SetTheme(t ui.Theme) {
	m.theme = t
	m.st = newStyles(t, m.icons)
	m.runSt = ui.NewRunStyles(t, m.icons)
	m.confirmSt = t.Confirm(m.icons)
	m.thread.SetStyles(t.Thread(m.icons))
	if m.checks != nil {
		m.checks.SetTheme(t)
	}
	if m.loaded {
		_ = m.show()
	}
}

// View implements ui.Modal.
func (m *detailModal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	base := m.thread.View()
	if m.onChecks() {
		base = m.checks.View()
	}
	if m.ask != nil {
		// The question lines up with the detail, behind its gutter.
		lines := m.ask.Lines(m.confirmSt, m.keys.confirm, max(m.width-len(gutter), 0), min(m.height, ui.ConfirmLines))
		for i, l := range lines {
			lines[i] = ansi.Truncate(gutter+l, m.width, "")
		}
		return ui.OverLastLines(base, lines)
	}
	return base
}

// onChecks reports whether the Checks tab shows.
func (m *detailModal) onChecks() bool {
	return m.tab == checksTab && m.checks != nil
}

// Hide implements ui.Hider: the Checks step stops its polls while
// another modal is open in place of this one.
func (m *detailModal) Hide() {
	if m.checks != nil {
		m.checks.Hide()
	}
}

// Update implements ui.Modal. After the modal closed, it ignores what
// arrives late.
func (m *detailModal) Update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.press(msg)
	case checks.CloseMsg:
		// The step has nothing left to dismiss: the modal closes, from
		// whichever level of the step.
		if m.checks != nil && msg.ID == m.checks.ID() {
			return m.close()
		}
		return nil
	case ui.ReopenedMsg:
		// What the step opened, such as the file of an annotation, closed:
		// the step goes on if it is on view, and otherwise when it shows.
		if m.onChecks() {
			return m.checks.Update(msg)
		}
		return nil
	}
	if m.checks == nil {
		return m.updateDetail(msg)
	}
	// The thread and the detail go on loading behind the step, and the
	// step on reading behind the conversation.
	return tea.Batch(m.checks.Update(msg), m.updateDetail(msg))
}

// press takes a key: the answer to a question, whatever a step types,
// then the keys of the modal, which work on every tab, and last those of
// the tab.
func (m *detailModal) press(msg tea.KeyPressMsg) tea.Cmd {
	if m.ask != nil {
		return m.answer(msg)
	}
	if m.onChecks() && m.checks.TakesKeys() {
		return m.checks.Update(msg)
	}
	k := m.keys
	switch {
	case m.hasTabs() && key.Matches(msg, k.NextTab):
		return m.cycle(1)
	case m.hasTabs() && key.Matches(msg, k.PrevTab):
		return m.cycle(-1)
	case key.Matches(msg, k.owner) && ui.Author(m.detail.Author) != "":
		return m.author()
	case key.Matches(msg, k.Checks) && m.tab != checksTab && m.hasChecks():
		return m.switchTo(checksTab)
	case k.isChange(msg):
		return m.change(msg)
	}
	if m.onChecks() {
		return m.checks.Update(msg)
	}
	switch {
	case key.Matches(msg, k.Back):
		return m.close()
	case key.Matches(msg, k.Refresh):
		m.svc.Invalidate(m.repo)
		return tea.Batch(m.get(), m.thread.Reload())
	case key.Matches(msg, k.Open):
		if m.detail.URL != "" {
			return ui.Open(m.detail.URL)
		}
		return nil
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

// close closes the modal, and stops its reads and the step's polls.
func (m *detailModal) close() tea.Cmd {
	m.closed = true
	m.cancel()
	if m.checks != nil {
		m.checks.Close()
	}
	m.resume()
	return ui.CloseModal(m)
}

// change starts the change that msg asks of the pull request, once the
// user confirms it on the last line of the modal. The change shows at once
// in the modal and the list behind it, and then it is sent.
func (m *detailModal) change(msg tea.KeyPressMsg) tea.Cmd {
	if !m.loaded {
		return nil
	}
	c, ok, warn := m.keys.change(m.svc, m.gate(), m.mergeMethod, m.detail.PullRequest, msg)
	if !ok {
		return warn
	}
	repo, about := m.repo, m.subject()
	run := m.keys.confirmed(m.svc, m.mergeMethod, c, msg,
		func() (core.PullRequest, ui.Gate, bool) {
			return m.detail.PullRequest, m.gate(), m.loaded
		},
		func(op *optimistic.Op, what string) tea.Cmd {
			return tea.Batch(m.reload(),
				func() tea.Msg { return changedMsg{repo: repo} },
				ui.Do(m.sendCtx, ui.PullsTitle, ui.About(about, op), what))
		})
	m.ask = &ui.Confirm{Question: c.question, Run: run}
	return nil
}

// answer takes the answer to the question on the last line: yes makes the
// change, and no steps back to the detail. Other keys do nothing.
func (m *detailModal) answer(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := m.keys.confirm.Answer(*m.ask, msg)
	if done {
		m.ask = nil
	}
	return cmd
}

// subject names the pull request in what the user reads of a failure,
// such as "eggzec/gh-tui#5", since GitHub may not find it or refuse access
// to it while it finds its repository.
func (m *detailModal) subject() string {
	return core.Target{Repo: m.repo, Number: m.number}.String()
}

// KeyLayers implements ui.Keyed: the answer while a change waits for one,
// or the step's alone while it takes every key. Otherwise the modal's own
// keys come first, and then those of the tab: the Checks step's, or the
// thread's.
func (m *detailModal) KeyLayers() []keyhelp.Layer {
	switch {
	case m.ask != nil:
		return []keyhelp.Layer{m.keys.confirm.Layer()}
	case m.onChecks() && m.checks.TakesKeys():
		return m.checks.KeyLayers()
	}
	k := m.keys.withChanges(m.gate(), m.mergeMethod, m.detail.PullRequest, m.loaded)
	if m.onChecks() {
		return append([]keyhelp.Layer{m.modalLayer(k)}, m.checks.KeyLayers()...)
	}
	return []keyhelp.Layer{m.modalLayer(k), ui.ContextHelp("pull_conversation", m.thread, false)}
}

// modalLayer returns the layer of the keys of the modal, which work on any
// of its tabs, for k, as the modal takes them. While the Checks tab shows,
// the step has the keys to refresh and to open, and the key for the
// checks, which are shown, does nothing.
func (m *detailModal) modalLayer(k keyMap) keyhelp.Layer {
	tabs := m.hasTabs()
	k.NextTab.SetEnabled(k.NextTab.Enabled() && tabs)
	k.PrevTab.SetEnabled(k.PrevTab.Enabled() && tabs)
	k.Checks.SetEnabled(k.Checks.Enabled() && m.tab != checksTab && m.hasChecks())
	owner := m.keys.owner
	owner.SetEnabled(owner.Enabled() && ui.Author(m.detail.Author) != "")
	if m.onChecks() {
		l := ui.ContextLayer(ctxModal, []key.Binding{k.Merge, k.Close, k.Reopen, k.ToggleDraft, k.Checks, k.NextTab, k.PrevTab},
			[]key.Binding{k.Merge, k.Close, k.Reopen, k.NextTab})
		l.Bindings = append(l.Bindings, owner)
		return l
	}
	// The list's keys don't work here.
	for _, b := range []*key.Binding{&k.Select, &k.Filter, &k.Sort, &k.ClearFilter} {
		b.SetEnabled(false)
	}
	l := ui.ContextLayer(ctxModal, slices.Concat(k.FullHelp()...), k.ShortHelp())
	l.Bindings = append(l.Bindings, owner)
	return l
}

// gate decides what the viewer may do in the repository.
func (m *detailModal) gate() ui.Gate {
	return ui.Gate{Repo: m.repo, Caps: m.caps, Token: m.token, Icons: m.icons}
}
