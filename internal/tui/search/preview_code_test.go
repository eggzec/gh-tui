package search

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
)

// blobs is a CodePreviews that serves one blob per SHA.
type blobs struct {
	mu    sync.Mutex
	files map[string]core.Blob
	errs  map[string]error
	asked []string
}

func (b *blobs) Blob(_ context.Context, q filesvc.BlobQuery) (core.Blob, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.asked = append(b.asked, q.SHA)
	return b.files[q.SHA], b.errs[q.SHA]
}

func (b *blobs) reads() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.asked...)
}

func codeSection(t *testing.T, b *blobs) *Section {
	t.Helper()
	s := newSection(t, newFake(), 140, 40, WithCodeReader(NewCodeReader(b)))
	typeText(t, s, "tea")
	run(t, s, s.showKind(core.SearchCode))
	run(t, s, s.searchCode())
	return s
}

func TestFirstMatchLine(t *testing.T) {
	t.Parallel()
	text := "package x\n\nfunc Tea() {}\n"
	for _, c := range []struct {
		query string
		want  int
	}{{"tea", 3}, {"PACKAGE", 1}, {"absent", 1}, {"", 1}} {
		if got := firstMatchLine(text, c.query); got != c.want {
			t.Errorf("firstMatchLine(%q) = %d, want %d", c.query, got, c.want)
		}
	}
}

func TestCodePreviewShowsTheWholeFileAtTheFirstMatch(t *testing.T) {
	t.Parallel()
	b := &blobs{files: map[string]core.Blob{
		"b1": {Content: []byte("package tea\n\n// Program.\nfunc Run() {}\nvar teaTime = 1\n")},
		"b2": {Content: []byte("package tea\n")},
	}}
	s := codeSection(t, b)
	if len(b.reads()) == 0 {
		t.Fatal("the file of the hit under the cursor was not read")
	}
	if !strings.Contains(s.pvKey, " loaded") {
		t.Fatalf("pvKey = %q, want the loaded file", s.pvKey)
	}
	if got := s.pv.Query(); got != "tea" {
		t.Errorf("the pager search = %q, want the query", got)
	}
	if got := s.View(); !strings.Contains(ansi.Strip(got), "var teaTime = 1") {
		t.Errorf("the preview lacks the file's later lines:\n%s", got)
	}
}

func TestCodePreviewStartsAtTheFirstMatchLine(t *testing.T) {
	t.Parallel()
	b := &blobs{files: map[string]core.Blob{
		"b1": {Content: []byte("package x\n\n// intro\n\nvar a = 1\nvar b = 2\nvar c = 3\nvar d = 4\nvar e = 5\nvar f = 6\nfunc Tea() {}\n")},
		"b2": {Content: []byte("package x\n")},
	}}
	s := codeSection(t, b)
	if got := ansi.Strip(s.View()); !strings.Contains(got, "func Tea() {}") {
		t.Errorf("the preview does not start at the first match:\n%s", got)
	}
}

func TestCodePreviewSaysWhyAFileIsNotShown(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		blob core.Blob
		err  error
		want string
	}{
		"binary":    {blob: core.Blob{Binary: true}, want: "Binary file, not shown"},
		"too large": {err: &core.TooLargeError{Size: 5 << 20, Limit: 1 << 20}, want: "files.preview.max_size"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := &blobs{files: map[string]core.Blob{"b1": c.blob, "b2": c.blob}, errs: map[string]error{"b1": c.err, "b2": c.err}}
			s := codeSection(t, b)
			if got := s.View(); !strings.Contains(ansi.Strip(got), c.want) {
				t.Errorf("the preview lacks %q:\n%s", c.want, got)
			}
		})
	}
}

func TestCodePreviewWithoutAReaderKeepsTheFragments(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 140, 40)
	typeText(t, s, "tea")
	run(t, s, s.showKind(core.SearchCode))
	run(t, s, s.searchCode())
	if strings.Contains(s.pvKey, " loading") || !strings.Contains(ansi.Strip(s.View()), "Program is a terminal") {
		t.Errorf("without a reader the preview must be the fragments: %q\n%s", s.pvKey, s.View())
	}
}

func TestCodePreviewDropsAFileReadForAnOlderQuery(t *testing.T) {
	t.Parallel()
	b := &blobs{files: map[string]core.Blob{"b1": {Content: []byte("old\n")}}}
	s := codeSection(t, b)
	h, _ := s.code.feed.Selected()
	s.readPreviewDone(previewReadMsg{id: s.id, text: "older", key: codeFileKey(h), body: "stale"})
	if _, ok := s.extras[codeFileKey(h)]; ok && s.extras[codeFileKey(h)].text == "stale" {
		t.Error("a file read for an older query was kept")
	}
}
