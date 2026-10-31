package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// zoomedFiles opens the Files pane zoomed, with the patches of the files in
// the fake, and waits for the first diff.
func zoomedFiles(t *testing.T, fs []core.CommitFile, setup func(*fakeService), opts ...Option) (*host, *panesModal, *fakeService) {
	t.Helper()
	h, m, svc := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		svc.changed = fs
		if setup != nil {
			setup(svc)
		}
	}, opts...)
	press(t, h, "z")
	if !m.zoom.Zoomed() {
		t.Fatal("z didn't zoom the pane")
	}
	return h, m, svc
}

// hoverFile moves the cursor of the tree down to the next file row.
func hoverFile(t *testing.T, h *host, m *panesModal) string {
	t.Helper()
	for range 12 {
		press(t, h, "down")
		if p, ok := m.hoveredFile(); ok {
			return p
		}
	}
	t.Fatal("no file row was reached")
	return ""
}

func TestFilesDiffFollowsTheCursor(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	if !m.files.pv.on || m.files.pv.path == "" {
		t.Fatalf("no diff beside the tree: %+v", m.files.pv.path)
	}
	for range 2 {
		p := hoverFile(t, h, m)
		if m.files.pv.path != p {
			t.Fatalf("diff of %q, want the hovered %q", m.files.pv.path, p)
		}
		if got := modalScreen(t, h); !strings.Contains(got, "new "+p) {
			t.Errorf("screen doesn't show the diff of %s:\n%s", p, got)
		}
	}
}

func TestFilesDiffKeepsTheLastFileOnOtherRows(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	p := hoverFile(t, h, m)
	for range 12 {
		press(t, h, "down")
		if _, ok := m.hoveredFile(); !ok {
			if m.files.pv.path != p {
				t.Fatalf("diff of %q on a row that is no file, want %q kept", m.files.pv.path, p)
			}
			return
		}
		p = m.files.pv.path
	}
	t.Skip("no row that is no file after the file")
}

func TestFilesDiffShowsTheFirstFileOfANewRange(t *testing.T) {
	t.Parallel()
	only := core.CommitFile{Path: "only.go", Additions: 2, Patch: "@@ -1 +1 @@\n-old\n+new only.go\n"}
	h, m, _ := zoomedFiles(t, patched(), func(svc *fakeService) {
		svc.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: []core.CommitFile{only}}}
	})
	if m.files.pv.path == "" || m.files.pv.path == "only.go" {
		t.Fatalf("diff of %q before the range changed", m.files.pv.path)
	}
	m.files.rng = files.Range{First: 1, Last: 2}
	press(t, h, "up")
	if m.files.pv.path != "only.go" {
		t.Errorf("diff of %q after the range changed, want the first file of the range", m.files.pv.path)
	}
}

func TestFilesDiffReadsThePatchesOnce(t *testing.T) {
	t.Parallel()
	h, m, svc := zoomedFiles(t, changedFiles(), nil)
	reads := len(svc.fileReads)
	for range 3 {
		hoverFile(t, h, m)
	}
	for range 6 {
		press(t, h, "up")
	}
	if got := len(svc.fileReads); got != reads {
		t.Errorf("%d reads of the files after moving, want the %d before", got, reads)
	}
}

func TestFilesDiffNotBuiltBelowTheSplit(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() })
	if m.files.pv.on {
		t.Fatal("a diff is built before the pane is zoomed")
	}
	press(t, h, "z")
	if !m.files.pv.on {
		t.Fatal("no diff once the pane is zoomed")
	}
	m.SetSize(90, 30)
	press(t, h, "down")
	if m.files.pv.on {
		t.Error("a diff is built below 100 columns")
	}
}

func TestFilesDiffTabFocusesIt(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	hoverFile(t, h, m)
	path := m.files.pv.path
	press(t, h, "tab")
	if m.nav.Sub(paneFiles) != panes.Preview || !m.files.pv.o.d.Focused() {
		t.Fatalf("tab left box %v, diff focused %v", m.nav.Sub(paneFiles), m.files.pv.o.d.Focused())
	}
	at := m.files.pv.o.d.Cursor()
	press(t, h, "j")
	if got := m.files.pv.o.d.Cursor(); got != at+1 {
		t.Errorf("j moved the cursor of the diff from %d to %d", at, got)
	}
	if m.files.pv.path != path {
		t.Errorf("j changed the file to %q", m.files.pv.path)
	}
	press(t, h, "tab")
	if m.nav.Sub(paneFiles) != panes.List || m.files.pv.o.d.Focused() {
		t.Errorf("tab left box %v, diff focused %v", m.nav.Sub(paneFiles), m.files.pv.o.d.Focused())
	}
}

func TestFilesDiffTakesANewTheme(t *testing.T) {
	t.Parallel()
	_, m, _ := zoomedFiles(t, patched(), nil)
	before, seq := m.files.pv.o.d.View(), m.files.seq
	p, err := config.Default().Palette(!m.theme.Dark)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(ui.NewTheme(p, !m.theme.Dark))
	if m.files.pv.o.d.View() == before {
		t.Error("the diff keeps the colours of the old theme")
	}
	if m.files.seq == seq {
		t.Error("the panes are not drawn again for the new theme")
	}
}

func TestFilesDiffIgnoresOtherMessages(t *testing.T) {
	t.Parallel()
	_, m, _ := zoomedFiles(t, patched(), nil)
	seq := m.files.seq
	for range 3 {
		m.Update(struct{}{})
	}
	if m.files.seq != seq {
		t.Errorf("a message of no diff moved the drawing key from %d to %d", seq, m.files.seq)
	}
}

// TestFilesDiffView holds the zoomed Files pane at 120 columns in both themes,
// with the diff shown and while it loads.
func TestFilesDiffView(t *testing.T) {
	t.Parallel()
	for name, dark := range map[string]bool{"dark": true, "light": false} {
		for _, loading := range []bool{false, true} {
			state := "diff"
			if loading {
				state = "loading"
			}
			t.Run(name+"/"+state, func(t *testing.T) {
				t.Parallel()
				var m *panesModal
				if loading {
					_, m, _ = rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() })
					m.zoom.SetZoom(true)
					m.syncFilesDiff()
				} else {
					_, m, _ = zoomedFiles(t, patched(), nil)
				}
				p, err := config.Default().Palette(dark)
				if err != nil {
					t.Fatal(err)
				}
				m.SetTheme(ui.NewTheme(p, dark))
				m.SetSize(120, 30)
				t.Log(ansi.Strip(m.View()))
				golden.RequireEqual(t, m.View())
			})
		}
	}
}
