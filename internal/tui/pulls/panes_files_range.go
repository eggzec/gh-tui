package pulls

import (
	"context"
	"errors"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
)

// filesRead is the read of the files of the range of commits selected.
type filesRead struct {
	// key names the range that was asked for, and seq counts the reads, so that
	// an answer to an earlier one is dropped. cancel ends the read in flight.
	key    string
	seq    int
	cancel context.CancelFunc
	// done says that an answer came, with files and a note that they are cut
	// short, or with err: the words that say why they aren't there.
	done  bool
	files []core.CommitFile
	note  string
	err   string
	// version counts the answers, which the rows are built from.
	version int
}

// filesRangeMsg carries the files of a range to the modal that asked for them.
type filesRangeMsg struct {
	id    int64
	seq   int
	key   string
	files []core.CommitFile
	note  string
	err   error
}

// rangeEnds returns the commits that the files of the range are compared
// between, and whether the range is every commit. The base of the first
// commit is the base of the pull request. ok is false until the range is
// known.
func (m *panesModal) rangeEnds() (base, head string, all, ok bool) {
	f := &m.files
	n := len(f.commits.Commits)
	if !f.loaded || n == 0 || f.rng.First < 0 || f.rng.Last >= n || f.rng.First > f.rng.Last {
		return "", "", false, false
	}
	all = f.rng == files.All(n)
	if all {
		return m.detail.BaseSHA, f.head, true, f.head != ""
	}
	base = m.detail.BaseSHA
	if f.rng.First > 0 {
		base = f.commits.Commits[f.rng.First-1].OID
	}
	head = f.commits.Commits[f.rng.Last].OID
	return base, head, false, base != "" && head != ""
}

// readRange reads the files of the range selected, unless it is the range of
// the last read. A new range ends the read in flight, and its answer is
// dropped when it comes. The cache may answer at once.
func (m *panesModal) readRange() tea.Cmd {
	f := &m.files
	base, head, all, ok := m.rangeEnds()
	if !ok {
		return nil
	}
	key := base + "\x00" + head
	if all {
		key = "all|" + head
	}
	if key == f.read.key {
		return nil
	}
	if f.read.cancel != nil {
		f.read.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	f.read = filesRead{key: key, seq: f.read.seq + 1, cancel: cancel, version: f.read.version + 1}
	f.seq++
	svc, repo, number, id, seq := m.svc, m.repo, m.number, m.id, f.read.seq
	if all {
		note := m.changedNote()
		if fs, ok, fresh := cachedPullFiles(svc, repo, number, head); ok {
			m.takeRange(filesRangeMsg{seq: seq, key: key, files: fs, note: note})
			if fresh {
				return nil
			}
		}
		return func() tea.Msg {
			fs, err := pullFiles(ctx, svc, repo, number, head)
			return filesRangeMsg{id: id, seq: seq, key: key, files: fs, note: note, err: err}
		}
	}
	q := pulls.RangeQuery{Repo: repo, Base: base, Head: head}
	if d, ok := svc.CachedRange(q); ok {
		m.takeRange(filesRangeMsg{seq: seq, key: key, files: d.Files, note: rangeNote(d)})
		return nil
	}
	return func() tea.Msg {
		d, err := svc.RangeFiles(ctx, q)
		return filesRangeMsg{id: id, seq: seq, key: key, files: d.Files, note: rangeNote(d), err: err}
	}
}

// rangeNote says that the files of a compare are cut short.
func rangeNote(d core.CompareDetail) string {
	if !d.Truncated {
		return ""
	}
	return "first " + strconv.Itoa(len(d.Files)) + " files of this range"
}

// rangeRead takes the files that a read brought. The answer to a read that was
// replaced is dropped, and so is one that failed because it was cancelled.
func (m *panesModal) rangeRead(msg filesRangeMsg) {
	f := &m.files
	if msg.id != m.id || msg.seq != f.read.seq || msg.key != f.read.key {
		return
	}
	if msg.err != nil && (m.ctx.Err() != nil || errors.Is(msg.err, context.Canceled)) {
		return
	}
	m.takeRange(msg)
}

// takeRange keeps the answer for the read in flight.
func (m *panesModal) takeRange(msg filesRangeMsg) {
	f := &m.files
	r := &f.read
	r.done, r.version = true, r.version+1
	r.files, r.note, r.err = slices.Clone(msg.files), msg.note, ""
	if msg.err != nil {
		r.files, r.note, r.err = nil, "", rangeErrText(msg.err)
	}
	f.seq++
}

// rangeErrText says why the files of a range couldn't be read.
func rangeErrText(err error) string {
	if errors.Is(err, core.ErrRewritten) {
		return "these commits are no longer on the branch"
	}
	return "the files of this range couldn't be read"
}

// filesData is what the rows of the pane are built from.
func (m *panesModal) filesData() files.Data {
	f := &m.files
	d := files.Data{Commits: f.commits, Range: f.rng, Findings: f.finds.byPath}
	if f.vw.in {
		d.Viewed = f.vw.pv
	}
	if f.read.done {
		d.Files, d.FilesErr, d.Truncated = f.read.files, f.read.err, f.read.note
	}
	return d
}

// revealFound selects the file the finder chose once the files are listed,
// and forgets it, also when the range doesn't have it.
func (m *panesModal) revealFound() {
	f := &m.files
	if m.found == "" || !f.read.done || f.built.data != f.read.version {
		return
	}
	if f.read.err == "" {
		f.tree.Reveal(files.FileID(m.found))
	}
	m.found = ""
}

// filesRange returns what the files of the Files pane are compared between,
// base and head by their full SHAs, with the files the range changes. ok is
// false while they are being read, and when the read failed. The diff of a
// file is read between these two commits.
func (m *panesModal) filesRange() (base, head string, fs []core.CommitFile, ok bool) {
	f := &m.files
	base, head, _, known := m.rangeEnds()
	if !known || !f.read.done || f.read.err != "" {
		return "", "", nil, false
	}
	return base, head, slices.Clone(f.read.files), true
}

// hoveredFile returns the path of the file under the cursor of the Files pane,
// and false when the cursor is on another row.
func (m *panesModal) hoveredFile() (path string, ok bool) {
	n, ok := m.files.tree.Selected()
	if !ok {
		return "", false
	}
	return files.PathOf(n.ID)
}
