package releases

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fetched is one transfer the fake downloader was asked to start.
type fetched struct{ url, path string }

// fakeDownloader records the transfers it is asked to start.
type fakeDownloader struct {
	dir  string
	mu   sync.Mutex
	runs []fetched
}

func (f *fakeDownloader) Dir() string { return f.dir }

func (f *fakeDownloader) Path(dir, name string) (string, error) { return filepath.Join(dir, name), nil }

func (f *fakeDownloader) Start(url, path string, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, fetched{url, path})
	return nil
}

func (f *fakeDownloader) started() []fetched {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fetched(nil), f.runs...)
}

// drain runs cmd and everything it batches or sequences, and returns the
// messages that are left.
func drain(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 && len(out) < 100 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			out = append(out, msg)
		}
	}
	return out
}

// downloadModal returns a loaded modal over a folder that holds one folder.
func downloadModal(t *testing.T) (*Modal, *fakeDownloader) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dest"), 0o755); err != nil {
		t.Fatal(err)
	}
	dl := &fakeDownloader{dir: root}
	svc := &fakeService{}
	m := New(context.Background(), svc, repo, v3.ID, "", config.Default().Keys, WithNow(func() time.Time { return clock }), WithLocation(time.UTC), WithDownloader(dl))
	m.SetSize(100, 30)
	run(t, m, m.Init())
	return m, dl
}

// pick opens the picker with key, and picks the folder it shows.
func pick(t *testing.T, m *Modal, key string) tea.Cmd {
	t.Helper()
	cmd := press(m, key)
	if m.pick == nil {
		t.Fatalf("%q opened no picker", key)
	}
	for _, msg := range drain(cmd) {
		m.Update(msg)
	}
	if !strings.Contains(m.View(), "dest") {
		t.Fatalf("picker doesn't list the folder:\n%s", m.View())
	}
	// Browse into dest, then pick the folder the picker shows.
	for _, msg := range drain(m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})) {
		m.Update(msg)
	}
	if !strings.HasSuffix(m.pick.fp.CurrentDirectory, "dest") {
		t.Fatalf("picker is in %q, want dest", m.pick.fp.CurrentDirectory)
	}
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestPickTheStartFolderWithNoSubfolders(t *testing.T) {
	root := t.TempDir()
	dl := &fakeDownloader{dir: root}
	m := New(context.Background(), &fakeService{}, repo, v3.ID, "", config.Default().Keys, WithNow(func() time.Time { return clock }), WithLocation(time.UTC), WithDownloader(dl))
	m.SetSize(100, 30)
	run(t, m, m.Init())
	for _, msg := range drain(press(m, "D")) {
		m.Update(msg)
	}
	answer(t, m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}))
	got := dl.started()
	if len(got) != len(v3.Assets) {
		t.Fatalf("started %v, want every file", got)
	}
	if filepath.Dir(got[0].path) != root {
		t.Errorf("file goes to %q, want %q", got[0].path, root)
	}
}

func TestPickerFollowsTheKeymap(t *testing.T) {
	keys := config.Default().Keys
	keys["release_modal"]["expand"] = []string{"p"}
	m := New(context.Background(), &fakeService{}, repo, v3.ID, "", keys, WithDownloader(&fakeDownloader{dir: t.TempDir()}))
	if got := m.pickKeys.Open.Keys(); len(got) != 1 || got[0] != "p" {
		t.Fatalf("picker Open keys = %v, want [p]", got)
	}
	for _, k := range m.pickKeys.filepicker().Open.Keys() {
		if k == "enter" {
			t.Error("the picker keeps the library's enter")
		}
	}
}

func TestSettingsMsgReachesTheModalWhilePicking(t *testing.T) {
	m, _ := downloadModal(t)
	press(m, "D")
	cfg := config.Default()
	cfg.UI.Icons = "ascii"
	want := ui.NewIcons("ascii").Repo
	if m.icons.Repo == want {
		t.Fatal("the test needs icons that differ from ASCII")
	}
	m.Update(ui.SettingsMsg{Config: cfg})
	if m.icons.Repo != want {
		t.Error("a setting changed over the picker never reached the modal")
	}
	if m.pick == nil {
		t.Fatal("the picker closed")
	}
}

func TestActBackClosesThePickerNotTheModal(t *testing.T) {
	m, _ := downloadModal(t)
	press(m, "D")
	if _, ok := m.Act(ui.ActBack); !ok || m.pick != nil || m.closed {
		t.Fatalf("back: pick=%v closed=%v", m.pick != nil, m.closed)
	}
	press(m, "D")
	if _, ok := m.Act(ui.ActDismiss); !ok || m.pick != nil || m.closed {
		t.Fatalf("dismiss: pick=%v closed=%v", m.pick != nil, m.closed)
	}
}

func TestPickerHelpShowsItsKeys(t *testing.T) {
	m, _ := downloadModal(t)
	press(m, "D")
	if len(m.KeyLayers()) != 1 {
		t.Fatalf("layers = %d", len(m.KeyLayers()))
	}
}

// answer confirms the question the cmd opens, running what answering starts.
func answer(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	for _, msg := range drain(cmd) {
		open, ok := msg.(ui.OpenModalMsg)
		if !ok {
			continue
		}
		cm, ok := open.Modal.(*ui.ConfirmModal)
		if !ok {
			t.Fatalf("opened %T, want a confirm", open.Modal)
		}
		for _, m := range drain(open.Load) {
			cm.Update(m)
		}
		drain(cm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"}))
		return
	}
	t.Fatal("no question was asked")
}

func TestDownloadAllAsksThenStartsEveryFile(t *testing.T) {
	m, dl := downloadModal(t)
	answer(t, pick(t, m, "D"))
	got := dl.started()
	if len(got) != len(v3.Assets) {
		t.Fatalf("started %d transfers, want %d: %v", len(got), len(v3.Assets), got)
	}
	for i, a := range v3.Assets {
		if !strings.HasSuffix(got[i].path, filepath.Join("dest", a.Name)) {
			t.Errorf("transfer %d goes to %q", i, got[i].path)
		}
		if !strings.HasPrefix(got[i].url, "https://assets.test/") {
			t.Errorf("transfer %d reads %q", i, got[i].url)
		}
	}
	if m.pick != nil {
		t.Error("the picker stayed open after the pick")
	}
}

func TestDownloadStartsNothingUntilConfirmed(t *testing.T) {
	m, dl := downloadModal(t)
	cmd := pick(t, m, "D")
	drain(cmd)
	if got := dl.started(); len(got) != 0 {
		t.Fatalf("started %v before the answer", got)
	}
}

func TestDownloadOneFile(t *testing.T) {
	m, dl := downloadModal(t)
	answer(t, pick(t, m, "d"))
	if got := dl.started(); len(got) != 1 {
		t.Fatalf("started %v, want the file under the cursor only", got)
	}
}

func TestSelectInFilesDownloadsTheFile(t *testing.T) {
	m, dl := downloadModal(t)
	m.nav.SetFocus(paneFiles)
	answer(t, pick(t, m, "enter"))
	if got := dl.started(); len(got) != 1 {
		t.Fatalf("started %v, want one transfer", got)
	}
}

func TestEscClosesThePickerBeforeTheModal(t *testing.T) {
	m, dl := downloadModal(t)
	press(m, "D")
	if m.pick == nil {
		t.Fatal("no picker")
	}
	press(m, "esc")
	if m.pick != nil || m.closed {
		t.Fatalf("esc left pick=%v closed=%v, want the picker closed and the modal open", m.pick != nil, m.closed)
	}
	if got := dl.started(); len(got) != 0 {
		t.Fatalf("started %v", got)
	}
	press(m, "esc")
	if !m.closed {
		t.Error("a second esc didn't close the modal")
	}
}

func TestBackspaceInPickerKeepsTheModal(t *testing.T) {
	m, _ := downloadModal(t)
	press(m, "D")
	press(m, "backspace")
	if m.closed {
		t.Error("backspace closed the modal")
	}
}

func TestNoDownloaderNoPicker(t *testing.T) {
	m := newModal(&fakeService{}, 100, 30)
	run(t, m, m.Init())
	press(m, "D")
	if m.pick != nil {
		t.Error("the picker opened with nothing to save the files")
	}
}
