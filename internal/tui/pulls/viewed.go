package pulls

import (
	"maps"
	"strconv"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// viewing says whether the user viewed a file, which is kept on this machine
// only.
type viewing int

const (
	// notViewed is a file with no mark.
	notViewed viewing = iota
	// viewedFile is a file marked as viewed, as it is still.
	viewedFile
	// changedFile is a file that was marked as viewed and has changed since.
	changedFile
)

// viewingOf says whether file counts as viewed: it was marked, and holds the
// blob it had then. A file that GitHub names no blob of is taken by its path.
func (m *detailModal) viewingOf(file diff.File) viewing {
	sha, ok := m.visit.prev.Viewed[file.Path]
	switch {
	case !ok:
		return notViewed
	case sha == file.SHA || sha == "" || file.SHA == "":
		return viewedFile
	}
	return changedFile
}

// styleViewed renders the marks of the files in theme and icons.
func (f *filesState) styleViewed(theme ui.Theme, icons ui.Icons) {
	f.seen = theme.Success.Render(icons.Yes)
	f.changed = theme.Subtle.Render(icons.Changed)
}

// viewMark returns the mark that follows a file in the tree and its header in
// the diff, or empty for a file that wasn't viewed.
func (f *filesState) viewMark(file diff.File) string {
	if f.viewing == nil {
		return ""
	}
	switch f.viewing(file) {
	case viewedFile:
		return f.seen
	case changedFile:
		return f.changed
	case notViewed:
	}
	return ""
}

// viewedNote is the end of the status line of the diff: how many files are
// viewed.
func (f *filesState) viewedNote() string {
	if f.viewed == 0 {
		return ""
	}
	return strconv.Itoa(f.viewed) + " viewed"
}

// foldViewed folds the files that are viewed among files, or among all those
// read for nil, and counts them. Files not viewed are left as they are, so that
// one the user opened stays open.
func (m *detailModal) foldViewed(files []diff.File) {
	f := m.files
	if f == nil {
		return
	}
	if files == nil {
		files = f.set.list()
	}
	for _, file := range files {
		if m.viewingOf(file) == viewedFile {
			f.diff.SetCollapsed(file.Path, true)
		}
	}
	m.countViewed()
}

// countViewed counts the files read that are viewed, and draws the tree again
// with their marks.
func (m *detailModal) countViewed() {
	f := m.files
	n := 0
	for _, file := range f.set.list() {
		if m.viewingOf(file) == viewedFile {
			n++
		}
	}
	f.viewed = n
	f.tree.SetIcons(f.icon)
}

// toggleViewed marks file as viewed, which folds it, or takes the mark off,
// which opens it, and keeps the marks. With next set, a file that was marked
// moves the diff on to the next one that isn't viewed.
func (m *detailModal) toggleViewed(file diff.File, next bool) tea.Cmd {
	v, f := &m.visit, m.files
	if v.store != nil && !v.loaded {
		// What was kept isn't read yet, and saving now would replace it.
		return nil
	}
	marks := maps.Clone(v.prev.Viewed)
	if marks == nil {
		marks = map[string]string{}
	}
	marked := m.viewingOf(file) != viewedFile
	if marked {
		marks[file.Path] = file.SHA
	} else {
		delete(marks, file.Path)
	}
	v.prev.Viewed = marks
	f.diff.SetCollapsed(file.Path, marked)
	m.countViewed()
	cmds := []tea.Cmd{m.saveViewed()}
	if marked && next {
		if p := m.nextUnviewed(file.Path); p != "" {
			cmds = append(cmds, m.showFile(p))
		}
	}
	return tea.Batch(cmds...)
}

// nextUnviewed returns the path of the first file after the one at path that
// isn't viewed, or empty for none.
func (m *detailModal) nextUnviewed(path string) string {
	files := m.files.set.list()
	at, _ := m.files.set.index(path)
	for _, file := range files[min(at+1, len(files)):] {
		if m.viewingOf(file) != viewedFile {
			return file.Path
		}
	}
	return ""
}

// saveViewed keeps the marks, with the last visit as it was, in a command that
// warns if it can't. Nothing is kept over a state that could not be read.
func (m *detailModal) saveViewed() tea.Cmd {
	v := &m.visit
	if v.store == nil || v.done || v.readFailed {
		return nil
	}
	store, key, seq := v.store, m.visitKey(), &v.seq
	state := v.prev
	state.Viewed = maps.Clone(state.Viewed)
	n := seq.take()
	return func() tea.Msg {
		seq.mu.Lock()
		defer seq.mu.Unlock()
		if n <= seq.wrote {
			// A later save, or the write of the visit, kept something newer.
			return nil
		}
		err := store.Save(key, state)
		seq.wrote = n
		if err != nil {
			return ui.Notify(toast.Warning, "Couldn't save the files viewed: "+err.Error())()
		}
		return nil
	}
}

// saveSeq orders the saves of the files viewed, which run in commands that
// may start in any order, against each other and against the write of the
// visit: a save that is older than what was written is dropped.
type saveSeq struct {
	mu          sync.Mutex
	next, wrote int
}

// take returns the number of the next save.
func (s *saveSeq) take() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	return s.next
}
