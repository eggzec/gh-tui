package pulls

import (
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestDevelopmentPreviewReadOnce(t *testing.T) {
	t.Parallel()
	f := &fakeDev{links: devLinks(), body: 3}
	h := openedDev(t, f)
	press(t, h, "z")
	// The Closes header needs no read.
	if len(f.issueReads)+len(f.pullReads) != 0 {
		t.Fatalf("a header was read: %v %v", f.issueReads, f.pullReads)
	}
	press(t, h, "down")
	press(t, h, "down")
	press(t, h, "up")
	press(t, h, "up")
	press(t, h, "down")
	if want := []int{12, 13}; !slices.Equal(f.issueReads, want) {
		t.Fatalf("issues read = %v, want %v, each once", f.issueReads, want)
	}
	for range 3 {
		press(t, h, "down")
	}
	if want := []int{30}; !slices.Equal(f.pullReads, want) {
		t.Fatalf("pull requests read = %v, want %v", f.pullReads, want)
	}
	if s := modalScreen(t, h); !strings.Contains(s, "Pull title") {
		t.Fatalf("the preview doesn't show the pull request:\n%s", s)
	}
}

func TestDevelopmentPreviewReadWhenShown(t *testing.T) {
	t.Parallel()
	f := &fakeDev{links: devLinks(), body: 3}
	h := openedDev(t, f)
	// Unzoomed there is no preview box, so a hovered row costs no read.
	press(t, h, "down")
	press(t, h, "down")
	if len(f.issueReads)+len(f.pullReads) != 0 {
		t.Fatalf("a preview that isn't shown was read: %v %v", f.issueReads, f.pullReads)
	}
	press(t, h, "z")
	if want := []int{13}; !slices.Equal(f.issueReads, want) {
		t.Fatalf("issues read = %v after zooming, want %v", f.issueReads, want)
	}
}

func TestDevelopmentPreviewScrollKept(t *testing.T) {
	t.Parallel()
	f := &fakeDev{links: devLinks(), body: 80}
	h := openedDev(t, f)
	m := h.modal()
	press(t, h, "z")
	press(t, h, "down")
	key := previewKey{paneDevelopment, "issue:" + core.RefKey(devRef(core.KindIssue, 12, "").Target)}
	// Into the preview, scrolled, and back to the list.
	press(t, h, "tab")
	for range 5 {
		press(t, h, "down")
	}
	press(t, h, "tab")
	want := m.previews[key]
	if want == 0 {
		t.Fatal("the preview didn't scroll")
	}
	press(t, h, "down")
	press(t, h, "up")
	if got := m.previews[key]; got != want {
		t.Fatalf("the offset of the preview is %d after hovering away and back, want %d", got, want)
	}
}
