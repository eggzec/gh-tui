package pulls

import (
	"context"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// viewNote says what the diff leaves out of the files it shows.
const viewNote = "hunks only"

// diffOverlay shows the diff of a file of the range over the panes, or the
// diff of every file of it. The files are read once, by the source, and a
// switch between the two builds a new view over them.
type diffOverlay struct {
	m    *panesModal
	src  *diffSource
	d    diff.Model
	keys diffKeys
	mode diffview.Mode
	// path is the file shown while the diff is of one file.
	path string
	// w and h are the size of the last drawing, for a view built after it.
	w, h   int
	ctx    context.Context
	cancel context.CancelFunc
	done   bool
	// vw is what is known of the marks of the files, and seen and changed are
	// the marks drawn after the header of a file that is viewed, or that changed
	// since.
	vw            viewedState
	seen, changed string
	// full is what is known of the files shown whole.
	full fullFiles
}

// openDiff opens the diff of the file at path over the panes, and shows the
// file in the Files pane.
func (m *panesModal) openDiff(path string) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	o := &diffOverlay{m: m, src: m.diffSource(), keys: m.keys.diff, mode: diffview.ThisFile, path: path, ctx: ctx, cancel: cancel}
	o.showCachedViewed()
	cmd := tea.Batch(o.build(), o.readViewed())
	m.open(o)
	m.reveal(path)
	return cmd
}

// reveal has the Files pane select the file at path when it is drawn next.
func (m *panesModal) reveal(path string) {
	m.found = path
	m.files.seq++
}

// build makes the view of the mode, which reads the first page from the
// source, and returns its read.
func (o *diffOverlay) build() tea.Cmd {
	path := ""
	if o.mode == diffview.ThisFile {
		path = o.path
	}
	o.d = diff.New(o.src.fetch(path),
		diff.WithContext(o.ctx),
		diff.WithKeyMap(o.keys.diff),
		diff.WithStyles(o.m.theme.Diff(o.m.icons)),
		diff.WithFocused(true),
	)
	if o.w > 0 && o.h > 0 {
		o.d.SetSize(o.w, o.h)
	}
	o.seen = o.m.theme.Success.Render(o.m.icons.Yes)
	o.changed = o.m.theme.Subtle.Render(o.m.icons.Changed)
	o.d.SetHeaderMark(o.viewMark)
	o.d.SetStatusNote(func() string {
		return diffview.Note(o.d.Cursor()+1, o.d.Len(), o.hints())
	})
	return o.d.Init()
}

// hints name the keys of the diff that its status line points to.
func (o *diffOverlay) hints() []string {
	k := o.keys
	var hs []string
	if keys := hintKeys(k.diff.NextFile, k.diff.PrevFile); keys != "" {
		hs = append(hs, keys+" file")
	}
	if keys := hintKeys(k.whole); keys != "" {
		if o.mode == diffview.ThisFile {
			hs = append(hs, keys+" whole patch")
		} else {
			hs = append(hs, keys+" this file")
		}
	}
	if keys := hintKeys(k.full); keys != "" {
		if o.d.Full(o.current()) {
			hs = append(hs, keys+" hunks only")
		} else {
			hs = append(hs, keys+" full file")
		}
	}
	if keys := hintKeys(k.diff.Search); keys != "" {
		hs = append(hs, keys+" search")
	}
	return hs
}

// current is the path of the file the diff is on.
func (o *diffOverlay) current() string {
	if o.mode == diffview.ThisFile {
		return o.path
	}
	f, _ := o.d.CurrentFile()
	return f.Path
}

// label names the range the diff is of.
func (o *diffOverlay) label() string {
	f := &o.m.files
	cs := f.commits.Commits
	if r := f.rng; len(cs) > 0 && r.First >= 0 && r.Last < len(cs) && r.First <= r.Last {
		return diffview.RangeLabel(len(cs), r.First, r.Last, cs[r.First].OID, cs[r.Last].OID)
	}
	head := o.m.detail.HeadSHA
	return diffview.RangeLabel(1, 0, 0, head, head)
}

func (o *diffOverlay) title() string {
	path := o.current()
	i, n := diffview.Place(o.src.paths(), path)
	pct := o.d.Cursor() * 100 / max(o.d.Len()-1, 1)
	return diffview.Title(o.mode, o.label(), path, i, n, o.modeNote(path), pct, o.m.placed().W-4)
}

func (o *diffOverlay) kind() panes.OverlayKind { return panes.Viewer }
func (o *diffOverlay) items() int              { return 0 }
func (o *diffOverlay) text() int               { return 0 }
func (o *diffOverlay) clear() bool             { return o.d.ClearSearch() }
func (o *diffOverlay) closed() bool            { return o.done }

// typing reports that the overlay takes the keys of the app's actions as text,
// while its search prompt is open.
func (o *diffOverlay) typing() bool { return o.d.Capturing() }

func (o *diffOverlay) view(w, h int) string {
	o.w, o.h = w, h
	o.d.SetSize(w, h)
	return o.d.View()
}

// finish ends the diff and the read it has in flight.
func (o *diffOverlay) finish() {
	o.done = true
	o.cancel()
}

func (o *diffOverlay) layers() []keyhelp.Layer {
	if o.d.Capturing() {
		return []keyhelp.Layer{ui.ContextHelp("search_prompt", o.d, true)}
	}
	keys := o.d.KeyMap()
	// A retry has no key of its own to show, and the cancel of an empty search
	// is the dismiss key.
	keys.Retry.SetEnabled(false)
	l := ui.ContextHelp(ctxDiff, keys, false)
	if empty := keys.CancelEmpty.Keys(); len(empty) > 0 {
		l.Bindings = slices.DeleteFunc(l.Bindings, func(b key.Binding) bool { return slices.Equal(b.Keys(), empty) })
	}
	l.Bindings = append(l.Bindings, o.keys.whole, o.keys.full, o.keys.viewed, o.keys.readOn)
	l.Short = append(l.Short, o.keys.whole, o.keys.full, o.keys.viewed, o.keys.readOn)
	// The keys of the modal don't reach the diff, so its layer lists none; the
	// chain still has it, as that of every overlay of the panes.
	return []keyhelp.Layer{ui.ContextLayer(ctxModal, nil, nil), l}
}

func (o *diffOverlay) update(msg tea.Msg) tea.Cmd {
	if fm, ok := msg.(diff.FilesMsg); ok && fm.ID == o.d.ID() {
		for _, f := range fm.Files {
			o.foldViewed(f.Path)
		}
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && !o.d.Capturing() {
		switch {
		case keymap.Matches(press, o.keys.viewed):
			return o.toggleViewed(o.current(), true)
		case keymap.Matches(press, o.keys.readOn):
			return o.readOn()
		case keymap.Matches(press, o.keys.diff.NextFile):
			return o.step(1)
		case keymap.Matches(press, o.keys.diff.PrevFile):
			return o.step(-1)
		case keymap.Matches(press, o.keys.whole):
			return o.toggle()
		case keymap.Matches(press, o.keys.full):
			return o.toggleFull()
		}
	}
	var cmd tea.Cmd
	o.d, cmd = o.d.Update(msg)
	if fm, ok := msg.(diff.FilesMsg); ok && fm.ID == o.d.ID() {
		cmd = tea.Batch(cmd, o.reapplyFull(fm.Files))
	}
	return cmd
}

// step moves the diff d files along the order of the tree, and the Files pane
// with it. It does nothing at either end.
func (o *diffOverlay) step(d int) tea.Cmd {
	next, ok := diffview.Step(o.src.paths(), o.current(), d)
	if !ok {
		return nil
	}
	return o.goTo(next)
}

// goTo shows the file at path, and the Files pane with it.
func (o *diffOverlay) goTo(path string) tea.Cmd {
	o.m.reveal(path)
	if o.mode == diffview.ThisFile {
		o.path = path
		return o.build()
	}
	o.d.SeekFile(path)
	// A file not fetched yet leaves the seek pending, and an update starts the
	// read of the pages it waits for.
	var cmd tea.Cmd
	o.d, cmd = o.d.Update(nil)
	return cmd
}

// toggle switches between the diff of the file and that of the whole patch,
// and keeps the line the cursor is on.
func (o *diffOverlay) toggle() tea.Cmd {
	pos, onLine := o.d.Position()
	path := o.current()
	if onLine {
		path = pos.Path
	}
	if path == "" {
		return nil
	}
	o.full.seek = nil
	if o.mode == diffview.WholePatch {
		o.mode, o.path = diffview.ThisFile, path
	} else {
		o.mode = diffview.WholePatch
	}
	cmd := o.build()
	if onLine {
		o.d.Seek(pos)
		// A line of a file shown whole may be in no hunk, so the view of hunks
		// alone can't hold the cursor on it.
		if o.full.want[pos.Path] {
			o.full.seek = &pos
		}
	} else {
		o.d.SeekFile(path)
	}
	return cmd
}
