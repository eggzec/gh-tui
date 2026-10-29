package pulls

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// fakeBlobs serves blobs by their SHA and records what it was asked for.
type fakeBlobs struct {
	mu    sync.Mutex
	blobs map[string]core.Blob
	err   error
	reads []string
}

func (f *fakeBlobs) Blob(_ context.Context, q filesvc.BlobQuery) (core.Blob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, q.SHA)
	return f.blobs[q.SHA], f.err
}

func (f *fakeBlobs) read() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reads)
}

// fullPanes opens the diff of path over files that each have a blob, but
// docs/c.md, which is deleted.
func fullPanes(t *testing.T, blobs *fakeBlobs, path string) (*host, *diffOverlay) {
	t.Helper()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		fs := patched()
		for i := range fs {
			fs[i].SHA = "sha-" + fs[i].Path
			if fs[i].Path == "docs/c.md" {
				fs[i].Status = core.FileRemoved
			}
		}
		svc.changed = fs
	}, WithBlobs(blobs))
	drain(t, h, m.openDiff(path))
	o, ok := m.top().(*diffOverlay)
	if !ok {
		t.Fatalf("top overlay = %T, want the diff", m.top())
	}
	modalScreen(t, h)
	return h, o
}

func notified(msgs []tea.Msg, text string) bool {
	return slices.ContainsFunc(msgs, func(m tea.Msg) bool {
		n, ok := m.(ui.NotifyMsg)
		return ok && strings.Contains(n.Text, text)
	})
}

func TestDiffFullFileSwitchesTheMode(t *testing.T) {
	t.Parallel()
	blobs := &fakeBlobs{blobs: map[string]core.Blob{
		"sha-a.go": {Content: []byte("keep\nnew a.go\nmore\nafter the hunk\n")},
	}}
	h, o := fullPanes(t, blobs, "a.go")
	if got := o.title(); !strings.Contains(got, "hunks only [x]") {
		t.Fatalf("title %q, want the mode hunks only with its key", got)
	}
	press(t, h, "x")
	if got := o.title(); !strings.Contains(got, "full file [x]") {
		t.Errorf("title %q after x, want the mode full file", got)
	}
	if got := modalScreen(t, h); !strings.Contains(got, "after the hunk") {
		t.Errorf("screen doesn't show the lines after the hunk:\n%s", got)
	}
	press(t, h, "x")
	if got := o.title(); !strings.Contains(got, "hunks only [x]") {
		t.Errorf("title %q after x again, want hunks only", got)
	}
	if got := modalScreen(t, h); strings.Contains(got, "after the hunk") {
		t.Errorf("screen still shows the lines after the hunk:\n%s", got)
	}
	// Shown whole again from what was read.
	press(t, h, "x")
	if got := o.title(); !strings.Contains(got, "full file [x]") {
		t.Errorf("title %q after the third x, want full file", got)
	}
	if reads := blobs.read(); len(reads) != 1 || reads[0] != "sha-a.go" {
		t.Errorf("blobs read %v, want sha-a.go once", reads)
	}
}

func TestDiffFullFileSurvivesAStepToAnotherFile(t *testing.T) {
	t.Parallel()
	blobs := &fakeBlobs{blobs: map[string]core.Blob{
		"sha-a.go": {Content: []byte("keep\nnew a.go\nmore\nafter the hunk\n")},
	}}
	h, o := fullPanes(t, blobs, "a.go")
	press(t, h, "x")
	press(t, h, "J")
	press(t, h, "K")
	if got := o.title(); !strings.Contains(got, "full file [x]") {
		t.Errorf("title %q after coming back to the file, want full file", got)
	}
}

func TestDiffFullFileKeepsALineOfNoHunkAcrossTheWholePatch(t *testing.T) {
	t.Parallel()
	blobs := &fakeBlobs{blobs: map[string]core.Blob{
		"sha-a.go": {Content: []byte("keep\nnew a.go\nmore\nafter the hunk\n")},
	}}
	h, o := fullPanes(t, blobs, "a.go")
	press(t, h, "x")
	// The fourth line is in no hunk of the patch.
	want := diff.Pos{Path: "a.go", Side: diff.NewSide, Line: 4}
	if !o.d.Seek(want) {
		t.Fatal("the diff doesn't show line 4 of the file whole")
	}
	for _, step := range []string{"to the whole patch", "back to the file"} {
		press(t, h, "a")
		if got, ok := o.d.Position(); !ok || got != want {
			t.Errorf("position %+v, %v after a, %s; want %+v", got, ok, step, want)
		}
		if !o.d.Full("a.go") {
			t.Errorf("a.go is not whole after a, %s", step)
		}
	}
}

func TestDiffFullFileRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		path  string
		blobs map[string]core.Blob
		err   error
		toast string
	}{
		{name: "deleted", path: "docs/c.md", toast: "deleted file"},
		{name: "binary", path: "a.go", blobs: map[string]core.Blob{"sha-a.go": {Binary: true}}, toast: "binary file"},
		{name: "too large", path: "a.go", err: &core.TooLargeError{Size: 3 << 20, Limit: 1 << 20}, toast: "over the 1 MiB limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			blobs := &fakeBlobs{blobs: tt.blobs, err: tt.err}
			h, o := fullPanes(t, blobs, tt.path)
			msgs := press(t, h, "x")
			if !notified(msgs, tt.toast) {
				t.Errorf("no toast saying %q among %v", tt.toast, msgs)
			}
			if got := o.title(); !strings.Contains(got, "hunks only") {
				t.Errorf("title %q, want the mode to stay hunks only", got)
			}
			if tt.name == "deleted" && len(blobs.read()) != 0 {
				t.Errorf("blobs read %v for a deleted file, want none", blobs.read())
			}
		})
	}
}
