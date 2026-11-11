package releases

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Downloader saves the files of a release. The app wires one in; without it
// the download keys do nothing.
type Downloader interface {
	// Dir is the folder the folder picker starts in.
	Dir() string
	// Path is where the file called name goes in dir, with the name made safe
	// to write.
	Path(dir, name string) (string, error)
	// Start starts the transfer of url to path, and returns without waiting
	// for it.
	Start(url, path string, size int64) error
}

// WithDownloader sets what saves the files. Without it, the download keys
// do nothing.
func WithDownloader(d Downloader) Option { return func(o *options) { o.downloader = d } }

// picking is the folder picker that a download key opens over the panes.
type picking struct {
	fp    filepicker.Model
	files []core.ReleaseAsset
}

// download opens the folder picker for the files as.
func (m *Modal) download(as []core.ReleaseAsset) tea.Cmd {
	if len(as) == 0 || m.pick != nil || m.dl == nil {
		return nil
	}
	fp := filepicker.New()
	fp.KeyMap = m.pickKeys.filepicker()
	fp.DirAllowed, fp.FileAllowed, fp.ShowHidden = true, false, false
	fp.CurrentDirectory = m.startDir()
	fp.SetHeight(m.pickHeight())
	m.pick = &picking{fp: fp, files: as}
	return m.pick.fp.Init()
}

// startDir is where the picker starts: the folder of downloads.
func (m *Modal) startDir() string {
	return m.dl.Dir()
}

// pickHeight is the rows the picker has under its title.
func (m *Modal) pickHeight() int { return max(m.height-3, 3) }

// pressPick handles a key while the picker shows: dismiss closes it, select
// picks the folder it shows, and any other key goes to it.
func (m *Modal) pressPick(msg tea.KeyPressMsg) tea.Cmd {
	if keymap.Matches(msg, m.keys.Dismiss) {
		m.pick = nil
		return nil
	}
	if keymap.Matches(msg, m.pickKeys.Select) {
		p := m.pick
		m.pick = nil
		return m.confirmDownload(p.files, p.fp.CurrentDirectory)
	}
	return m.updatePick(msg)
}

// updatePick passes msg to the picker.
func (m *Modal) updatePick(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.pick.fp, cmd = m.pick.fp.Update(msg)
	return cmd
}

// confirmDownload asks before the files go to dir.
func (m *Modal) confirmDownload(as []core.ReleaseAsset, dir string) tea.Cmd {
	what := fmt.Sprintf("%d files", len(as))
	if len(as) == 1 {
		what = ui.OneLine(as[0].Name)
	}
	ask := confirm.Ask{
		Question: fmt.Sprintf("Download %s to %s?", what, ui.ShortPath(dir)),
		Run:      func() tea.Cmd { return m.startDownloads(as, dir, what) },
	}
	return ui.OpenConfirm(ask, m.confirmKeys, m.icons)
}

// startDownloads tells where the files go and starts one transfer for each.
// A failure to start one is a toast of its own.
func (m *Modal) startDownloads(as []core.ReleaseAsset, dir, what string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(as)+1)
	cmds = append(cmds, ui.Notify(toast.Success, fmt.Sprintf("downloading %s to %s", what, ui.ShortPath(dir))))
	for _, a := range as {
		cmds = append(cmds, m.fetch(a, dir))
	}
	return tea.Batch(cmds...)
}

// fetch resolves the address of a and starts its transfer.
func (m *Modal) fetch(a core.ReleaseAsset, dir string) tea.Cmd {
	svc, dl, ctx, repo := m.svc, m.dl, m.ctx, m.repo
	return func() tea.Msg {
		fail := func(err error) tea.Msg {
			return ui.Notify(toast.Error, fmt.Sprintf("download %s: %v", ui.OneLine(a.Name), err))()
		}
		path, err := dl.Path(dir, a.Name)
		if err != nil {
			return fail(err)
		}
		url, err := svc.AssetURL(ctx, repo, a.ID)
		if err != nil {
			return fail(err)
		}
		if err := dl.Start(url, path, a.Size); err != nil {
			return fail(err)
		}
		return nil
	}
}

// selectedAsset is the file under the cursor of the Files pane.
func (m *Modal) selectedAsset() (core.ReleaseAsset, bool) {
	n, ok := m.files.Selected()
	if !ok {
		return core.ReleaseAsset{}, false
	}
	a, ok := n.Value.(core.ReleaseAsset)
	return a, ok
}

// pickView is the picker, with its title, in the size of the modal.
func (m *Modal) pickView() string {
	title := m.theme.Muted.Render("Download to " + ui.ShortPath(m.pick.fp.CurrentDirectory))
	lines := []string{title, ""}
	lines = append(lines, strings.Split(m.pick.fp.View(), "\n")...)
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i := range lines {
		lines[i] = ui.Seal(lines[i], m.width)
	}
	for len(lines) < m.height {
		lines = append(lines, ui.Seal("", m.width))
	}
	return strings.Join(lines, "\n")
}
