package pulls

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The conversation tab of the modal: the pull request's head and its
// comments, in a thread.

// updateDetail takes what is not a key: the reads of the detail and the
// comments, which go on whichever tab shows, and what moves the thread.
func (m *detailModal) updateDetail(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
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

// get fetches the detail. A fresh cached detail costs no request. Once it
// returns, the reads ahead of the list go on. What failed before is
// forgotten, so that GitHub answering again doesn't read it once more
// while this read is under way.
func (m *detailModal) get() tea.Cmd {
	return m.read(m.svc.Get)
}

// revalidate fetches the detail though a cached one is fresh: opening the
// modal reads it again, since its merge state and threads change without
// the pull request's update time moving. What is cached shows meanwhile.
func (m *detailModal) revalidate() tea.Cmd {
	return m.read(m.svc.Revalidate)
}

// read fetches the detail with get.
func (m *detailModal) read(get func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error)) tea.Cmd {
	m.failed = nil
	ctx, repo, number, id, resume := m.ctx, m.repo, m.number, m.thread.ID(), m.resume
	return func() tea.Msg {
		start := time.Now()
		d, err := get(ctx, repo, number)
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
