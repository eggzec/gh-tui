package files

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var (
	errOffline = fmt.Errorf("github: GET: %w", core.ErrOffline)
	errRefused = fmt.Errorf("github: 403 Forbidden: %w", core.ErrForbidden)
)

// online tells the host that GitHub answers again, twice, as a flapping
// connection may.
func (h *host) online() {
	for range 2 {
		h.run(func() tea.Msg { return ui.OnlineMsg{} })
	}
}

// agentsReads counts the reads of the content of AGENTS.md.
func agentsReads(f *fake) int {
	sha := file("AGENTS.md", 0).SHA
	n := 0
	for _, s := range f.blobSHAs() {
		if s == sha {
			n++
		}
	}
	return n
}

// TestPreviewOnlineRetriesOnce checks that the preview of a file that
// failed for want of an answer reads it once GitHub answers again, and
// that one GitHub refused, or one that loaded, costs nothing.
func TestPreviewOnlineRetriesOnce(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"offline", errOffline, 1},
		{"refused", errRefused, 0},
		{"loaded", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := sampleFake()
			sha := file("AGENTS.md", 0).SHA
			if tt.err != nil {
				f.blobErrs[sha] = tt.err
			}
			h := openRow(t, f, rowAgents)
			f.mu.Lock()
			delete(f.blobErrs, sha)
			f.mu.Unlock()
			before := agentsReads(f)
			h.online()
			if got := agentsReads(f) - before; got != tt.want {
				t.Errorf("reads once online = %d, want %d", got, tt.want)
			}
			if tt.want > 0 && !strings.Contains(h.modal(), "Guidance for anyone.") {
				t.Errorf("preview = %q, want the file", h.modal())
			}
		})
	}
}

// TestFinderOnlineRetriesListing checks that the finder whose listing
// failed for want of an answer lists the files once GitHub answers again,
// once.
func TestFinderOnlineRetriesListing(t *testing.T) {
	fk := sampleFake()
	h := newHost(loaded(t, fk, 40, 12))
	fk.mu.Lock()
	fk.errs[treeKey(ghTUI, "")] = errOffline
	fk.mu.Unlock()
	f := findIn(t, h)
	if f.find.Err() == nil {
		t.Fatal("the listing should have failed")
	}
	fk.mu.Lock()
	clear(fk.errs)
	// The listing the section read is gone, so the finder asks GitHub.
	clear(fk.cachedAll)
	fk.mu.Unlock()
	before := fk.allCount()
	h.online()
	if got := fk.allCount() - before; got != 1 {
		t.Errorf("listings once online = %d, want 1", got)
	}
	if got := listed(f); !slices.Contains(got, "AGENTS.md") {
		t.Errorf("finder lists %q, want the files", got)
	}
}

// TestFinderOnlineRetriesPreview checks that the finder's preview reads
// again, once, the file that failed for want of an answer, and not while
// a read of it is under way.
func TestFinderOnlineRetriesPreview(t *testing.T) {
	for _, underWay := range []bool{false, true} {
		t.Run(fmt.Sprint("under way ", underWay), func(t *testing.T) {
			fk := sampleFake()
			sha := file("AGENTS.md", 0).SHA
			fk.blobErrs[sha] = errOffline
			h := newHost(loaded(t, fk, 40, 12, fast))
			h.width, h.height = 120, 16
			f := findIn(t, h)
			h.keys(strings.Split("agents", "")...)
			if f.failed == nil {
				t.Fatal("the preview should have failed")
			}
			fk.mu.Lock()
			delete(fk.blobErrs, sha)
			fk.mu.Unlock()
			var read tea.Cmd
			if underWay {
				// A read of the file starts, and is still under way as
				// GitHub answers again.
				read = f.read()
			}
			before := agentsReads(fk)
			h.online()
			h.run(read)
			if got := agentsReads(fk) - before; got != 1 {
				t.Errorf("reads once online = %d, want 1", got)
			}
			if v := ansi.Strip(f.View()); !strings.Contains(v, "Guidance for anyone.") {
				t.Errorf("view = %q, want the file", v)
			}
		})
	}
}
