package pulls

import (
	"errors"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// fullNote says what the diff shows of a file that is shown whole.
const fullNote = "full file"

// fullFiles is what the diff keeps of the files it shows whole. A view built
// again, for another file or the other mode, starts with hunks only, so the
// files wanted whole are shown so again as soon as the view has them.
type fullFiles struct {
	// want holds the paths shown whole, and content what was read of the files.
	want    map[string]bool
	content map[string]string
	// reading holds the paths whose blob is on its way.
	reading map[string]bool
	// seek is the line a switch to another view left the cursor on, to go back
	// to once the view has the file whole, since its hunks alone don't have it.
	seek *diff.Pos
}

// fullMsg carries the blob read for the file at path of the diff o.
type fullMsg struct {
	o    *diffOverlay
	path string
	blob core.Blob
	err  error
}

// viewLabel says what the diff shows of the file at path: its hunks only, or
// the file whole.
func (o *diffOverlay) viewLabel(path string) string {
	if o.d.Full(path) {
		return fullNote
	}
	return viewNote
}

// modeNote is viewLabel with the key that switches it.
func (o *diffOverlay) modeNote(path string) string {
	note := o.viewLabel(path)
	if keys := hintKeys(o.keys.full); keys != "" {
		note += " [" + keys + "]"
	}
	return note
}

// toggleFull shows the file the cursor is on whole, or takes it back to its
// hunks. Only that file changes, also in the diff of the whole patch.
func (o *diffOverlay) toggleFull() tea.Cmd {
	path := o.current()
	if path == "" {
		return nil
	}
	if o.d.Full(path) {
		delete(o.full.want, path)
		o.d.ClearFull(path)
		return nil
	}
	if o.full.reading[path] {
		return nil
	}
	if o.m.blobs == nil {
		return ui.Notify(toast.Info, "This view can't read whole files.")
	}
	f, ok := o.fileOf(path)
	switch {
	case !ok:
		return ui.Notify(toast.Info, "Still reading the files; try again in a moment.")
	case f.Status == core.FileRemoved:
		return ui.Notify(toast.Info, "A deleted file has nothing to show whole.")
	case f.SHA == "":
		return ui.Notify(toast.Info, "GitHub named no content for this file.")
	case f.Patch == "":
		return ui.Notify(toast.Info, "This file has no changed lines to show whole.")
	case f.PatchTruncated:
		return ui.Notify(toast.Info, "This file's diff is cut short, so it can't be shown whole.")
	}
	if content, ok := o.full.content[path]; ok {
		return o.showFull(path, content)
	}
	if o.full.reading == nil {
		o.full.reading = map[string]bool{}
	}
	o.full.reading[path] = true
	blobs, ctx, q := o.m.blobs, o.ctx, filesvc.BlobQuery{Repo: o.m.repo, SHA: f.SHA}
	return func() tea.Msg {
		blob, err := blobs.Blob(ctx, q)
		return fullMsg{o: o, path: path, blob: blob, err: err}
	}
}

// fileOf returns the file of the range at path, once the files are read.
func (o *diffOverlay) fileOf(path string) (core.CommitFile, bool) {
	fs, _ := o.src.kept()
	i := slices.IndexFunc(fs, func(f core.CommitFile) bool { return f.Path == path })
	if i < 0 {
		return core.CommitFile{}, false
	}
	return fs[i], true
}

// receiveFull shows the file whole once its blob has come, if the diff is
// still on it. A file the diff has moved off is kept for when it comes back.
func (o *diffOverlay) receiveFull(msg fullMsg) tea.Cmd {
	delete(o.full.reading, msg.path)
	if o.done {
		return nil
	}
	switch {
	case errors.Is(msg.err, core.ErrTooLarge):
		return ui.Notify(toast.Info, "This file is too large to show whole.")
	case msg.err != nil:
		return ui.Fail("read "+msg.path, msg.err)
	case msg.blob.Binary:
		return ui.Notify(toast.Info, "A binary file can't be shown whole.")
	}
	if o.full.content == nil {
		o.full.content = map[string]string{}
	}
	o.full.content[msg.path] = string(msg.blob.Content)
	if o.current() != msg.path {
		return nil
	}
	return o.showFull(msg.path, o.full.content[msg.path])
}

// showFull shows the file at path whole, and says so when the diff can't.
func (o *diffOverlay) showFull(path, content string) tea.Cmd {
	if !o.d.SetFull(path, content) {
		return ui.Notify(toast.Info, "The diff of this file can't be shown whole.")
	}
	if o.full.want == nil {
		o.full.want = map[string]bool{}
	}
	o.full.want[path] = true
	// An update starts the lexing of the rows that came with the content.
	var cmd tea.Cmd
	o.d, cmd = o.d.Update(nil)
	return cmd
}

// reapplyFull shows whole again the files of fs that were shown so, which a
// new view starts without.
func (o *diffOverlay) reapplyFull(fs []diff.File) tea.Cmd {
	var cmds []tea.Cmd
	for _, f := range fs {
		content, ok := o.full.content[f.Path]
		if !ok || !o.full.want[f.Path] {
			continue
		}
		if o.d.SetFull(f.Path, content) {
			if s := o.full.seek; s != nil && s.Path == f.Path {
				o.d.Seek(*s)
				o.full.seek = nil
			}
			var cmd tea.Cmd
			o.d, cmd = o.d.Update(nil)
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}
