package pulls

import (
	"context"
	"strconv"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// The finder step of the modal: finds one of the files the pull request
// changes by some letters of its path, and shows its diff on the Files tab.

// openFind shows the finder over the tab on view. It lists the files the
// diff has read, or reads them itself when the diff hasn't, and says that
// it loads until they are there. Without a head to read the files at, it
// does nothing.
func (m *detailModal) openFind() tea.Cmd {
	if m.find != nil || !m.canReadFiles() {
		return nil
	}
	// A merge that waits for the detail would ask over the finder.
	m.merging = nil
	m.findHead = m.detail.HeadSHA
	ctx, stop := context.WithCancel(m.ctx)
	f := finder.New(m.listChanged(),
		finder.WithContext(ctx),
		finder.WithKeyMap(m.keys.finder),
		finder.WithStyles(m.theme.Finder(m.icons)),
		finder.WithPlaceholder("Type to find a changed file"),
		finder.WithErrorText(m.findErr),
	)
	f.Focus()
	f.SetSize(m.width, m.height)
	m.find, m.stopFind = &f, stop
	return f.Init()
}

// dropFind ends the finder and its read, and shows the tab it covered.
func (m *detailModal) dropFind() {
	if m.find == nil {
		return
	}
	m.find.Close()
	m.stopFind()
	m.find, m.stopFind = nil, nil
}

// listChanged returns what the finder loads: the files in the diff when it
// has read them all, and otherwise each page of them, as the diff reads
// them, which the cache serves when the diff read it first.
func (m *detailModal) listChanged() finder.Load {
	var have []diff.File
	if f := m.files; f != nil && f.diff.Done() && f.diff.Err() == nil && f.set.len() == f.diff.Files() {
		have = f.set.list()
	}
	fetch := m.fetchFiles(m.detail.HeadSHA, new(atomic.Bool))
	var note string
	if n := m.detail.ChangedFiles; n > core.MaxPullFiles {
		note = "only the first " + strconv.Itoa(core.MaxPullFiles) + " of " + strconv.Itoa(n)
	}
	minus := m.icons.Minus
	return func(ctx context.Context) (finder.Listing, error) {
		files := have
		if files == nil {
			for cursor := ""; ; {
				page, next, err := fetch(ctx, cursor)
				if err != nil {
					return finder.Listing{}, err
				}
				files = append(files, page...)
				if next == "" || len(page) == 0 && next == cursor {
					break
				}
				cursor = next
			}
		}
		items := make([]finder.Item, len(files))
		for i, f := range files {
			items[i] = finder.Item{Path: f.Path, Detail: changes(f, minus), Value: f}
		}
		return finder.Listing{Items: items, Note: note}, nil
	}
}

// chooseFile closes the finder and puts the diff on the file at path, on
// the Files tab, which it shows first if another does, and focuses the
// diff. A file the diff hasn't read yet is sought as the pages arrive.
func (m *detailModal) chooseFile(path string) tea.Cmd {
	m.dropFind()
	if m.detail.HeadSHA != m.findHead {
		return nil
	}
	cmd := m.switchTo(filesTab)
	f := m.files
	if f == nil {
		return cmd
	}
	found := f.diff.SeekFile(path)
	f.setFocus(diffPane)
	m.layoutFiles()
	var warn tea.Cmd
	if !found && f.diff.Done() {
		// The files are those of another head than the finder listed.
		warn = ui.Notify(toast.Warning, path+" isn't changed in this pull request.")
	} else {
		f.settling = path
	}
	var seek tea.Cmd
	// Fetches what the seek shows, which the diff does on its next update.
	f.diff, seek = f.diff.Update(nil)
	// The tree follows the diff to the file, which it reveals.
	return tea.Batch(cmd, warn, seek, m.follow(), m.spinWhileLoading())
}
