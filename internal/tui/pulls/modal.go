package pulls

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
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

// detailModal shows a pull request with its comments in a modal, and
// changes it, or its checks in a step of its own. It is opened with
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

	// checksSvc reads the checks, and newChecks makes the Checks step,
	// which checks is while it is shown; newChecks is nil without checks.
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
	m := &detailModal{
		svc:         s.svc,
		keys:        s.keys,
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
	if onChecks {
		step = m.openChecks()
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
	if m.checks != nil {
		return m.checks.View()
	}
	if m.ask != nil {
		// The question lines up with the detail, behind its gutter.
		lines := m.ask.Lines(m.confirmSt, m.keys.confirm, max(m.width-len(gutter), 0), min(m.height, ui.ConfirmLines))
		for i, l := range lines {
			lines[i] = ansi.Truncate(gutter+l, m.width, "")
		}
		return ui.OverLastLines(m.thread.View(), lines)
	}
	return m.thread.View()
}

// openChecks shows the Checks step in place of the detail, and returns
// what loads it.
func (m *detailModal) openChecks() tea.Cmd {
	if m.newChecks == nil || m.checks != nil {
		return nil
	}
	m.checks = m.newChecks()
	m.checks.SetTheme(m.theme)
	m.checks.SetSize(m.width, m.height)
	return m.checks.Init()
}

// closeChecks steps back from the Checks step to the detail, whose header
// counts the checks as the step last read them.
func (m *detailModal) closeChecks() tea.Cmd {
	if m.checks == nil {
		return nil
	}
	m.checks.Close()
	m.checks = nil
	if !m.loaded {
		return nil
	}
	return m.show()
}

// Hide implements ui.Hider: the Checks step stops its polls while
// another modal is open in place of this one.
func (m *detailModal) Hide() {
	if m.checks != nil {
		m.checks.Hide()
	}
}

// updateChecks passes msg to the Checks step, and steps back to the
// detail when the step asks.
func (m *detailModal) updateChecks(msg tea.Msg) tea.Cmd {
	if c, ok := msg.(checks.CloseMsg); ok {
		if c.ID == m.checks.ID() {
			return m.closeChecks()
		}
		return nil
	}
	return m.checks.Update(msg)
}

// Update implements ui.Modal. After the modal closed, it ignores what
// arrives late.
func (m *detailModal) Update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	if m.checks != nil {
		if k, ok := msg.(tea.KeyPressMsg); ok {
			return m.updateChecks(k)
		}
		// The thread and the detail go on loading behind the step.
		cmd := m.updateChecks(msg)
		if m.checks == nil {
			return cmd
		}
		return tea.Batch(cmd, m.updateDetail(msg))
	}
	return m.updateDetail(msg)
}

// updateDetail is Update while the detail shows.
func (m *detailModal) updateDetail(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.press(msg)
	case detailMsg:
		return m.receive(msg)
	case ui.DoneMsg:
		// The change was confirmed or rolled back; either way the cache
		// has the outcome.
		if msg.From != ui.PullsTitle {
			return nil
		}
		return m.reload()
	case ui.SyncMsg:
		if msg.Err != nil || msg.Key != pulls.SyncKey(m.repo) {
			return nil
		}
		return tea.Batch(m.get(), m.thread.Reload())
	case ui.CapsMsg:
		if msg.Repo.Same(m.repo) {
			m.caps = msg.Caps
			m.bodies.SetPrivate(msg.Caps.Private)
		}
		return nil
	case ui.OnlineMsg:
		return m.online()
	case ui.ImagesMsg:
		if !m.drawPictures() {
			m.thread.Redraw()
		}
		return nil
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

// online reads again, now that GitHub answers again, the detail and the
// comments that failed for want of an answer from it.
func (m *detailModal) online() tea.Cmd {
	var get tea.Cmd
	if ui.Unreached(m.failed) {
		get = m.get()
	}
	return tea.Batch(get, ui.RetryUnreached(&m.thread))
}

func (m *detailModal) press(msg tea.KeyPressMsg) tea.Cmd {
	if m.ask != nil {
		return m.answer(msg)
	}
	k := m.keys
	switch {
	case key.Matches(msg, k.Back):
		m.closed = true
		m.cancel()
		m.resume()
		return ui.CloseModal(m)
	case key.Matches(msg, k.Checks):
		return m.openChecks()
	case key.Matches(msg, k.Refresh):
		m.svc.Invalidate(m.repo)
		return tea.Batch(m.get(), m.thread.Reload())
	case key.Matches(msg, k.Open):
		if m.detail.URL != "" {
			return ui.Open(m.detail.URL)
		}
		return nil
	case k.isChange(msg):
		return m.change(msg)
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
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

// get fetches the detail. A fresh cached detail costs no request. Once it
// returns, the reads ahead of the list go on. What failed before is
// forgotten, so that GitHub answering again doesn't read it once more
// while this read is under way.
func (m *detailModal) get() tea.Cmd {
	m.failed = nil
	svc, ctx, repo, number, id, resume := m.svc, m.ctx, m.repo, m.number, m.thread.ID(), m.resume
	return func() tea.Msg {
		start := time.Now()
		d, err := svc.Get(ctx, repo, number)
		resume()
		obs.End(ctx, start, err, "span", "tui", "repo", repo.String(), "number", number)
		return detailMsg{thread: id, detail: d, err: err}
	}
}

func (m *detailModal) receive(msg detailMsg) tea.Cmd {
	if msg.thread != m.thread.ID() {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		m.failed = msg.err
		return ui.Fail("load #"+strconv.Itoa(m.number), core.About(m.subject(), msg.err))
	}
	m.detail, m.loaded, m.failed = msg.detail, true, nil
	return m.show()
}

// subject names the pull request in what the user reads of a failure,
// such as "eggzec/gh-tui#5", since GitHub may not find it or refuse access
// to it while it finds its repository.
func (m *detailModal) subject() string {
	return core.Target{Repo: m.repo, Number: m.number}.String()
}

// reload shows the detail from the cache again, which a change has just
// updated or rolled back.
func (m *detailModal) reload() tea.Cmd {
	d, ok := m.svc.CachedGet(m.repo, m.number)
	if !ok {
		return nil
	}
	m.detail, m.loaded = d, true
	return m.show()
}

// show sets the document of the thread from the detail.
func (m *detailModal) show() tea.Cmd {
	m.bodies.SetDocument(m.detail.ID, m.detail.Body)
	return m.thread.SetDocument(m.detailHeader(m.width), m.detail.Body)
}

// KeyLayers implements ui.Keyed: those of the Checks step while it shows,
// the answer while a change waits for one, and otherwise the modal's own
// keys and then the thread's.
func (m *detailModal) KeyLayers() []keyhelp.Layer {
	switch {
	case m.checks != nil:
		return m.checks.KeyLayers()
	case m.ask != nil:
		return []keyhelp.Layer{m.keys.confirm.Layer()}
	}
	k := m.keys.withChanges(m.gate(), m.mergeMethod, m.detail.PullRequest, m.loaded)
	// The list's keys don't work here.
	for _, b := range []*key.Binding{&k.Select, &k.Filter, &k.Sort, &k.ClearFilter, &k.NextTab, &k.PrevTab} {
		b.SetEnabled(false)
	}
	return []keyhelp.Layer{keyhelp.FromHelp("pull request", k, false), keyhelp.FromHelp("thread", m.thread, false)}
}

// gate decides what the viewer may do in the repository.
func (m *detailModal) gate() ui.Gate {
	return ui.Gate{Repo: m.repo, Caps: m.caps, Token: m.token, Icons: m.icons}
}

// detailHeader renders the head of the pull request at width: the title,
// its state, author and age, the refs and stats, the checks and the labels,
// over a rule.
func (m *detailModal) detailHeader(width int) string {
	d, st := &m.detail, &m.st
	inner := max(width-len(gutter), 1)
	now := m.now()
	var lines []string
	line := func(parts ...string) {
		lines = append(lines, gutter+strings.Join(parts, ""))
	}

	// The title and the number link to the pull request's page.
	for l := range strings.SplitSeq(ansi.Wrap(ui.OneLine(d.Title), inner, ""), "\n") {
		line(termtext.Link(d.URL, st.selected.Render(l)))
	}
	lines = append(lines, "")

	dot := st.sep.Render(st.ic.Separator)
	line(st.badge(d.PullRequest), "  ",
		termtext.Link(d.URL, st.age.Render("#"+strconv.Itoa(d.Number))), dot,
		st.title.Render(ui.OneLine(d.Author.Login)), st.author.Render(" opened "+m.dates.Prose(d.CreatedAt, now)), dot,
		st.author.Render("updated "+m.dates.Prose(d.UpdatedAt, now)))

	stats := []string{
		st.title.Render(ui.OneLine(d.HeadRef)) + st.sep.Render(" "+st.ic.Arrow+" ") + st.title.Render(ui.OneLine(d.BaseRef)),
		st.added.Render("+"+strconv.Itoa(d.Additions)) + " " + st.deleted.Render(st.ic.Minus+strconv.Itoa(d.Deletions)),
		st.author.Render(plural(d.ChangedFiles, "file")),
	}
	if r := st.reviewText(d.ReviewDecision); r != "" {
		stats = append(stats, r)
	}
	line(strings.Join(stats, dot))

	if c := m.ciLine(); c != "" {
		line(c)
	}
	if len(d.Labels) > 0 {
		names := make([]string, 0, len(d.Labels))
		for _, l := range d.Labels {
			names = append(names, st.label.Render(ui.OneLine(l.Name)))
		}
		line(strings.Join(names, "  "))
	}
	line(st.rule.Render(strings.Repeat(st.ic.Border.Top, inner)))
	return strings.Join(lines, "\n")
}

// ciLine counts the checks of the pull request by how they stand, from the
// checks that the Checks step read if they are in memory, which count the
// commit statuses too, or else from the detail. It names the key that
// shows them.
func (m *detailModal) ciLine() string {
	var line string
	if c, ok := m.cachedChecks(); ok && c.Total > 0 {
		line = m.st.age.Render("CI  ") + checks.Summary(c, m.runSt)
	} else {
		line = m.st.checksSummary(&m.detail)
	}
	if line == "" {
		return ""
	}
	if k := m.keys.Checks; k.Enabled() && k.Help().Key != "" {
		line += m.st.sep.Render(" " + m.st.ic.Separator + m.st.ic.Key(k.Help().Key) + " for details")
	}
	return line
}

// cachedChecks returns the checks of the pull request from memory.
func (m *detailModal) cachedChecks() (core.Checks, bool) {
	if m.checksSvc == nil {
		return core.Checks{}, false
	}
	return m.checksSvc.CachedChecks(actions.ChecksQuery{Repo: m.repo, Number: m.number})
}

func plural(n int, noun string) string {
	s := strconv.Itoa(n) + " " + noun
	if n != 1 {
		s += "s"
	}
	return s
}

// renderComment renders a comment as its author and age over its body,
// markdown rendered like the pull request's, behind a bar.
func (m *detailModal) renderComment(c core.Comment, width int) string {
	st := &m.st
	var b strings.Builder
	// The avatar takes its box from the start, so the line doesn't move
	// when it arrives.
	b.WriteString(gutter + m.avatars.Line(c.AvatarURL) + st.commenter.Render(ui.OneLine(c.Author.Login)) + st.age.Render(st.ic.Separator+m.dates.Prose(c.CreatedAt, m.now())))
	bar := gutter + st.bar
	// The bar takes two cells, and as many stay free on the right.
	m.bodies.Add(c.ID, c.Body)
	body := m.thread.Markdown(c.Body, markdown.Room(width, 2*len(gutter)+2))
	if body != "" {
		b.WriteByte('\n')
		b.WriteString(markdown.Indent(body, bar))
	}
	return b.String()
}

// drawPictures has the thread draw the images of the body and comments
// that stand alone on their lines, where images are drawn, at most as
// tall as the modal's height allows, and reports whether that changed.
// Only the bodies whose pictures change render again, and where images
// aren't drawn the markdown is as without them.
func (m *detailModal) drawPictures() bool {
	rows := m.avatars.PictureRows(m.height)
	if rows == m.picRows {
		return false
	}
	m.picRows = rows
	m.thread.SetPictures(m.avatars.Pictures(rows, m.bodies))
	return true
}
