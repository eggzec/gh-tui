package tui

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// zoomedFiles is the keys app with the files tree zoomed at the size, over
// a service whose blob read fails when blobErr is set.
func zoomedFiles(t *testing.T, w, h int, blobErr error) *Model {
	t.Helper()
	m := wiredApp(t, brokenFiles{blobErr: blobErr}, brokenIssues{}, brokenOwners{})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	tap(t, m, "z")
	tap(t, m, "j")
	return m
}

// The zoomed tree shows the file under the cursor in a passive preview
// beside it.
func TestFilesSplitZoomed(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := zoomedFiles(t, 120, 40, nil)
		if s := screenText(m); !strings.Contains(s, "README.md") || !strings.Contains(s, "Keys") {
			t.Errorf("no preview of the file beside the zoomed tree:\n%s", s)
		}
		if top := ansi.Strip(m.prev.top); !strings.Contains(top, "README.md") {
			t.Errorf("the box is titled %q, want the file", top)
		}
		golden.RequireEqual(t, m.View().Content)
	})
}

// With the focus on the preview, the keys that scroll it do, and the
// pager's other keys stay unbound.
func TestFilesSplitPreviewScrolls(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := zoomedFiles(t, 120, 40, nil)
		before := m.prev.preview.PreviewView()
		tap(t, m, "tab")
		if !m.prevFocus {
			t.Fatal("tab did not focus the preview")
		}
		tap(t, m, "j")
		after := m.prev.preview.PreviewView()
		if after == before {
			t.Errorf("the preview did not scroll:\n%s", after)
		}
		tap(t, m, "/")
		tap(t, m, "q")
		if !m.splitShown() || m.prev.preview.PreviewView() != after {
			t.Error("a search or quit key reached the preview")
		}
		tap(t, m, "tab")
		tap(t, m, "j")
		if m.prev.preview.PreviewView() != after {
			t.Error("the preview scrolled without the focus")
		}
	})
}

// A file that is not cached shows a placeholder, and no read.
func TestFilesSplitMiss(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := zoomedFiles(t, 120, 40, errors.New("boom"))
		if s := screenText(m); !strings.Contains(s, "Not read yet.") || strings.Contains(s, "boom") {
			t.Errorf("want the placeholder, without a read:\n%s", s)
		}
		golden.RequireEqual(t, m.View().Content)
	})
}

// A narrow screen zooms the tree alone.
func TestFilesSplitNarrow(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := zoomedFiles(t, 80, 24, nil)
		if s := screenText(m); strings.Contains(s, "Keys") {
			t.Errorf("a narrow screen has a preview box:\n%s", s)
		}
		golden.RequireEqual(t, m.View().Content)
	})
}
