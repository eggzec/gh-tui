package history

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// filesAhead is how close to the last file loaded the cursor gets before
// the next page of files is read.
const filesAhead = 20

// commit is the commit pane: the commit under the graph's cursor, what it
// changed, and the patch of one of its files.
type commit struct {
	c   core.Commit
	has bool
	// detail is what c changed, once loaded.
	detail core.CommitDetail
	loaded bool
	// loading shows that the detail is on its way, and requested that it
	// was asked for.
	loading   bool
	requested bool
	err       error

	files        []core.CommitFile
	next         string
	truncated    bool
	filesLoading bool
	filesErr     error
	cursor, top  int

	// patch reports that the pager fills the pane with the patch of the
	// file under the cursor, rather than previewing it below the files.
	patch bool
	pager pager.Model
	// shown is the file in the pager, or -1.
	shown int

	// header caches the rendered header for headerKey.
	header    []string
	headerKey headerKey
}

type headerKey struct {
	sha    string
	width  int
	loaded bool
}

// restMsg reports that the graph's cursor rested on a commit.
type restMsg struct {
	id  int64
	seq int
}

// detailMsg carries what the commit sha changed.
type detailMsg struct {
	id     int64
	sha    string
	detail core.CommitDetail
	err    error
}

// filesMsg carries a page of the files that the commit sha changed.
type filesMsg struct {
	id     int64
	sha    string
	cursor string
	page   core.Page[core.CommitFile]
	err    error
}

// newCommit returns an empty commit pane, whose pager opens a patch in
// editor, if set.
func newCommit(editor string) commit {
	// The numbers of a patch's lines aren't those of the file.
	return commit{pager: pager.New(pager.WithLineNumbers(false), pager.WithEditor(editor)), shown: -1}
}

// clear forgets the commit, for another branch.
func (c *commit) clear() {
	*c = commit{pager: c.pager, shown: -1}
	c.pager.Blur()
	c.pager.SetMessage("", "")
}

// set shows commit k, from the row of the graph, and its detail if it is
// cached.
func (c *commit) set(k core.Commit, d core.CommitDetail, cached bool) {
	c.clear()
	c.c, c.has = k, true
	if cached {
		c.setDetail(d)
	}
}

func (c *commit) setDetail(d core.CommitDetail) {
	c.detail, c.loaded, c.loading, c.requested, c.err = d, true, false, true, nil
	c.files = d.Files
	c.next = d.FilesNext
	c.truncated = d.FilesTruncated
	c.header = nil
}

// follow shows the commit under the graph's cursor. A cached detail shows
// at once; the rest waits for the cursor to rest.
func (m *Modal) follow(k core.Commit) tea.Cmd {
	d, cached := m.svc.CachedCommit(m.repo, k.SHA)
	m.commit.set(k, d, cached)
	m.ahead.Opened(k.SHA)
	m.seq++
	cmds := []tea.Cmd{m.showFile()}
	if !cached {
		m.commit.loading = true
		cmds = append(cmds, m.startSpinner())
	}
	msg := restMsg{id: m.id, seq: m.seq}
	cmds = append(cmds, tea.Tick(m.opts.cfg.Prefetch.HoverDelay, func(time.Time) tea.Msg { return msg }))
	return tea.Batch(cmds...)
}

// rested reads the commit the cursor rested on, unless it moved on, and
// the commits around it.
func (m *Modal) rested(msg restMsg) tea.Cmd {
	if msg.seq != m.seq {
		return nil
	}
	around := m.ahead.Around(m.graphSHA, m.graph.model.Index(), m.opts.cfg.Prefetch.Around)
	return tea.Batch(m.loadDetail(), around)
}

// graphSHA returns the SHA of the commit at index i of the graph.
func (m *Modal) graphSHA(i int) (string, bool) {
	c, ok := m.graph.model.At(i)
	return c.ID, ok
}

// loadDetail reads what the commit shown changed, unless it is loaded.
func (m *Modal) loadDetail() tea.Cmd {
	c := &m.commit
	if !c.has || c.loaded || c.requested {
		return nil
	}
	c.loading, c.requested = true, true
	svc, ctx, repo, id, sha := m.svc, m.graph.ctx, m.repo, m.id, c.c.SHA
	return tea.Batch(m.startSpinner(), func() tea.Msg {
		ctx, end := obs.Begin(ctx, "history.commit")
		d, err := svc.Commit(ctx, repo, sha)
		end(err, "span", "tui", "repo", repo.String(), "sha", short(sha), "files", len(d.Files))
		return detailMsg{id: id, sha: sha, detail: d, err: err}
	})
}

// receiveDetail shows the detail of the commit shown.
func (m *Modal) receiveDetail(msg detailMsg) tea.Cmd {
	c := &m.commit
	if !c.has || msg.sha != c.c.SHA {
		return nil
	}
	if msg.err != nil {
		c.loading, c.err = false, msg.err
		c.header = nil
		return nil
	}
	c.setDetail(msg.detail)
	m.layoutCommit()
	return m.showFile()
}

// readDetail reads the detail of sha ahead of its use, for m.ahead.
func (m *Modal) readDetail(ctx context.Context, sha string) error {
	_, err := m.svc.Commit(ctx, m.repo, sha)
	return err
}

// cachedDetail reports whether the detail of sha is in memory.
func (m *Modal) cachedDetail(sha string) bool {
	_, ok := m.svc.CachedCommit(m.repo, sha)
	return ok
}

// moreFiles reads the next page of files once the cursor nears the end of
// those loaded.
func (m *Modal) moreFiles() tea.Cmd {
	c := &m.commit
	if c.next == "" || c.filesLoading || c.filesErr != nil || c.cursor+filesAhead < len(c.files) {
		return nil
	}
	c.filesLoading = true
	svc, ctx, repo, id, sha, cursor := m.svc, m.graph.ctx, m.repo, m.id, c.c.SHA, c.next
	return tea.Batch(m.startSpinner(), func() tea.Msg {
		ctx, end := obs.Begin(ctx, "history.files")
		p, err := svc.CommitFiles(ctx, historysvc.CommitFilesQuery{Repo: repo, SHA: sha, Cursor: cursor})
		end(err, "span", "tui", "repo", repo.String(), "sha", short(sha))
		return filesMsg{id: id, sha: sha, cursor: cursor, page: p, err: err}
	})
}

// retryCommit reads again the detail of the commit, or else the page of
// its files, if it failed as retry says it may be read again.
func (m *Modal) retryCommit(retry func(err error) bool) tea.Cmd {
	c := &m.commit
	switch {
	case c.err != nil:
		if !retry(c.err) {
			return nil
		}
		c.err, c.requested = nil, false
		return m.loadDetail()
	case c.filesErr != nil:
		if !retry(c.filesErr) {
			return nil
		}
		c.filesErr = nil
		return m.moreFiles()
	}
	return nil
}

// receiveFiles adds a page of files.
func (m *Modal) receiveFiles(msg filesMsg) tea.Cmd {
	c := &m.commit
	if !c.has || msg.sha != c.c.SHA || msg.cursor != c.next {
		return nil
	}
	c.filesLoading = false
	if msg.err != nil {
		c.filesErr = msg.err
		return nil
	}
	c.files = append(slices.Clip(c.files), msg.page.Items...)
	c.next = msg.page.Next
	m.layoutCommit()
	return m.moreFiles()
}

// pressCommit handles a key in the commit pane's list of files.
func (m *Modal) pressCommit(msg tea.KeyPressMsg) tea.Cmd {
	c := &m.commit
	k := m.keys.List
	page := max(m.filesHeight(), 1)
	before := c.cursor
	switch {
	case key.Matches(msg, m.keys.Select):
		if c.cursor >= len(c.files) {
			return nil
		}
		c.patch = true
		m.layoutCommit()
		c.pager.Focus()
		return m.showFile()
	case key.Matches(msg, m.keys.Retry):
		return m.retryCommit(func(error) bool { return true })
	case key.Matches(msg, k.Up):
		c.cursor--
	case key.Matches(msg, k.Down):
		c.cursor++
	case key.Matches(msg, k.PageUp):
		c.cursor -= page
	case key.Matches(msg, k.PageDown):
		c.cursor += page
	case key.Matches(msg, k.Home):
		c.cursor = 0
	case key.Matches(msg, k.End):
		c.cursor = len(c.files) - 1
	default:
		return nil
	}
	c.cursor = min(max(c.cursor, 0), max(len(c.files)-1, 0))
	m.scrollFiles()
	if c.cursor == before {
		return nil
	}
	return tea.Batch(m.showFile(), m.moreFiles())
}

// closePatch goes back from the patch to the list of files.
func (m *Modal) closePatch() {
	m.commit.patch = false
	m.commit.pager.Blur()
	m.layoutCommit()
}

// scrollFiles keeps the file under the cursor in view.
func (m *Modal) scrollFiles() {
	c := &m.commit
	h := max(m.filesHeight(), 1)
	c.top = min(c.top, c.cursor)
	if c.cursor >= c.top+h {
		c.top = c.cursor - h + 1
	}
	c.top = max(min(c.top, len(c.files)-h), 0)
}

// showFile puts the patch of the file under the cursor in the pager, if the
// pager is on view and shows another file.
func (m *Modal) showFile() tea.Cmd {
	c := &m.commit
	if !c.loaded || c.cursor >= len(c.files) || !m.pagerShown() {
		return nil
	}
	if c.shown == c.cursor {
		return nil
	}
	c.shown = c.cursor
	f := c.files[c.cursor]
	hint := ""
	if k := m.keys.Open.Help().Key; k != "" {
		hint = " Press " + k + " to see it on GitHub."
	}
	switch {
	case f.Patch != "":
		return c.pager.SetContentSyntax(f.Path, "diff", f.Patch)
	case f.PatchTruncated:
		c.pager.SetMessage(f.Path, "This diff is too large to show here."+hint)
	case (f.Status == core.FileRenamed || f.Status == core.FileCopied) && f.Additions+f.Deletions == 0:
		c.pager.SetMessage(f.Path, "Renamed from "+f.PreviousPath+", with no changes.")
	default:
		c.pager.SetMessage(f.Path, "No diff to show: the file is binary, or empty."+hint)
	}
	return nil
}

// commitURL is the page of the commit shown on GitHub.
func (m *Modal) commitURL(c core.Commit) string {
	return commitURL(m.opts.host, m.repo, c)
}

// commitURL is the page of commit c of repo on host.
func commitURL(host string, repo core.RepoRef, c core.Commit) string {
	if c.URL != "" {
		return c.URL
	}
	return ui.WebURL(host, repo.String()+"/commit/"+c.SHA)
}

// fileURL is the diff of file on the page of commit c: GitHub names the
// anchor of a file's diff by the SHA-256 of its path.
func (m *Modal) fileURL(c core.Commit, file string) string {
	sum := sha256.Sum256([]byte(file))
	return m.commitURL(c) + "#diff-" + hex.EncodeToString(sum[:])
}

// branchURL is the page of a branch on GitHub.
func (m *Modal) branchURL(name string) string {
	segs := strings.Split(name, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return ui.WebURL(m.opts.host, m.repo.String()+"/tree/"+strings.Join(segs, "/"))
}
