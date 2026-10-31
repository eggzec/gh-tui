package pulls

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// finderRows is how many rows the finder asks for when it is placed, which
// is as many as a picker takes.
const finderRows = 10

// canFind reports whether the changed files can be listed: the detail has to
// be there, with the head to read them at.
func (m *panesModal) canFind() bool { return m.loaded && m.detail.HeadSHA != "" }

// openFind shows the finder of the files the pull request changes over the
// panes. Without a head to read the files at, it does nothing.
func (m *panesModal) openFind() tea.Cmd {
	if !m.canFind() {
		return nil
	}
	ctx, stop := context.WithCancel(m.ctx)
	head := m.detail.HeadSHA
	f := finder.New(m.listChanged(head),
		finder.WithContext(ctx),
		finder.WithKeyMap(m.keys.finder),
		finder.WithStyles(m.theme.Finder(m.icons)),
		finder.WithPlaceholder("Type to find a changed file"),
		finder.WithErrorText(m.findErr),
	)
	f.Focus()
	o := &finderOverlay{m: m, f: f, head: head, stop: stop}
	m.open(o)
	return o.f.Init()
}

// listChanged returns what the finder loads: every page of the files changed
// at head. A page that was kept is read again, so that the finder never lists
// what the pull request no longer has.
func (m *panesModal) listChanged(head string) finder.Load {
	svc, repo, number := m.svc, m.repo, m.number
	note := m.changedNote()
	minus := m.icons.Minus
	return func(ctx context.Context) (finder.Listing, error) {
		fs, err := pullFiles(ctx, svc, repo, number, head)
		if err != nil {
			return finder.Listing{}, err
		}
		items := make([]finder.Item, 0, len(fs))
		for _, f := range fs {
			items = append(items, finder.Item{Path: f.Path, Detail: changeText(f, minus), Value: f})
		}
		return finder.Listing{Items: items, Note: note}, nil
	}
}

// changedNote says that the list of the files of the pull request is cut
// short, or is empty when it isn't.
func (m *panesModal) changedNote() string {
	if n := m.detail.ChangedFiles; n > core.MaxPullFiles {
		return "only the first " + strconv.Itoa(core.MaxPullFiles) + " of " + strconv.Itoa(n)
	}
	return ""
}

// pullFiles reads every page of the files changed at head. A page that was
// kept is read again, so that nothing lists what the pull request no longer has.
func pullFiles(ctx context.Context, svc Service, repo core.RepoRef, number int, head string) ([]core.CommitFile, error) {
	ctx, end := obs.Begin(ctx, "pull.files")
	var fs []core.CommitFile
	var err error
	for cursor := ""; ; {
		q := pulls.FilesQuery{Repo: repo, Number: number, Head: head, Cursor: cursor}
		var p core.Page[core.CommitFile]
		p, err = svc.Files(ctx, q)
		if err == nil && p.Stale {
			q.Again = true
			p, err = svc.Files(ctx, q)
		}
		if err != nil {
			break
		}
		fs = append(fs, p.Items...)
		if p.Next == "" || len(p.Items) == 0 && p.Next == cursor {
			break
		}
		cursor = p.Next
	}
	end(err, "span", "tui", "repo", repo.String(), "number", number, "files", len(fs))
	return fs, err
}

// cachedPullFiles returns the files changed at head from memory, and whether
// every page of them was there. fresh says that none was kept from before.
func cachedPullFiles(svc Service, repo core.RepoRef, number int, head string) (fs []core.CommitFile, ok, fresh bool) {
	fresh = true
	for cursor := ""; ; {
		p, hit := svc.CachedFiles(pulls.FilesQuery{Repo: repo, Number: number, Head: head, Cursor: cursor})
		if !hit {
			return nil, false, false
		}
		fresh = fresh && !p.Stale
		fs = append(fs, p.Items...)
		if p.Next == "" || len(p.Items) == 0 && p.Next == cursor {
			return fs, true, fresh
		}
		cursor = p.Next
	}
}

// changeText is how many lines f adds and deletes, as "+12 −3", leaving out a
// count of none, or empty for a file with no counted lines.
func changeText(f core.CommitFile, minus string) string {
	var parts []string
	if f.Additions > 0 {
		parts = append(parts, "+"+strconv.Itoa(f.Additions))
	}
	if f.Deletions > 0 {
		parts = append(parts, minus+strconv.Itoa(f.Deletions))
	}
	return strings.Join(parts, " ")
}

// chooseFile focuses the Files pane, zoomed or not as it was, and keeps path
// for the pane to select once it lists the files.
func (m *panesModal) chooseFile(path string) {
	m.found = path
	// The pane is drawn again to select it, even when it had the focus.
	m.files.seq++
	m.nav.SetFocus(paneFiles)
}

// openFound shows the file the finder chose in the Files pane, and its diff
// over it.
func (m *panesModal) openFound(path string) tea.Cmd {
	m.chooseFile(path)
	return m.openDiff(path)
}

// finderOverlay finds a changed file by some letters of its path. It is a
// picker in the way it is placed, but it types: it takes every key itself.
type finderOverlay struct {
	m    *panesModal
	f    finder.Model
	head string
	stop context.CancelFunc
	done bool
}

func (o *finderOverlay) title() string           { return "Changed files in #" + strconv.Itoa(o.m.number) }
func (o *finderOverlay) kind() panes.OverlayKind { return panes.Picker }
func (o *finderOverlay) items() int              { return finderRows }
func (o *finderOverlay) text() int               { return 0 }
func (o *finderOverlay) clear() bool             { return false }
func (o *finderOverlay) closed() bool            { return o.done }

// typing reports that the overlay takes the keys of the app's actions as text.
func (o *finderOverlay) typing() bool { return true }

func (o *finderOverlay) layers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("finder", &o.f, true)}
}

func (o *finderOverlay) view(w, h int) string {
	o.f.SetSize(w, h)
	return o.f.View()
}

// finish ends the finder and the read it has in flight.
func (o *finderOverlay) finish() {
	if o.done {
		return
	}
	o.done = true
	o.f.Close()
	o.stop()
}

func (o *finderOverlay) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case finder.ChosenMsg:
		if msg.ID != o.f.ID() {
			return nil
		}
		o.finish()
		if o.m.detail.HeadSHA != o.head {
			return ui.Notify(toast.Warning, "The pull request changed; find again.")
		}
		// The finder gives way to the diff, which opens above it.
		o.m.pop()
		return o.m.openFound(msg.Item.Path)
	case finder.CancelMsg:
		if msg.ID == o.f.ID() {
			o.finish()
		}
		return nil
	}
	var cmd tea.Cmd
	o.f, cmd = o.f.Update(msg)
	return cmd
}
