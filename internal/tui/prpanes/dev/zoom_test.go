package dev

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func newZoom(f *fakeReader) *Zoom {
	p := NewPreviews(f)
	return NewZoom(p, config.Default().Keys, theme(), ui.NewIcons(config.IconsUnicode))
}

func zoomItem() Item {
	return Item{Kind: Issue, Ref: issue(self, 1, "x", core.StateOpen)}
}

// shown loads the preview of the row under key and shows it in the pager.
func shown(t *testing.T, z *Zoom, key string) {
	t.Helper()
	if cmd := z.p.Load(key, zoomItem()); cmd != nil {
		z.p.Update(cmd())
	}
}

func comment(body string) core.Comment {
	return core.Comment{Author: core.User{Login: "bob"}, Body: body}
}

func TestZoomShowsBodyAndComments(t *testing.T) {
	f := &fakeReader{cached: true, body: "The description text.", warmPages: true,
		pages: map[string]core.Page[core.Comment]{"": {Items: []core.Comment{comment("first remark")}}}}
	z := newZoom(f)
	shown(t, z, "a")
	if cmd := z.Show("a", zoomItem(), previewOpts("unicode")); cmd != nil {
		t.Fatal("cached comments needed a read")
	}
	got := ansi.Strip(z.Draw(60, 40))
	for _, want := range []string{"The description text.", "first remark", "bob"} {
		if !strings.Contains(got, want) {
			t.Errorf("the pager lacks %q:\n%s", want, got)
		}
	}
	if !z.Shown() || z.Key() != "a" {
		t.Errorf("Shown = %v, Key = %q", z.Shown(), z.Key())
	}
}

// The editor gets the description and the comments as markdown, kept current
// as the comments land.
func TestZoomSource(t *testing.T) {
	f := &fakeReader{cached: true, body: "The description text.", warmPages: true,
		pages: map[string]core.Page[core.Comment]{"": {Items: []core.Comment{comment("first remark")}}}}
	z := newZoom(f)
	shown(t, z, "a")
	z.Show("a", zoomItem(), previewOpts("unicode"))
	z.Draw(60, 20)
	got := z.Pager().Source()
	for _, want := range []string{"The description text.", "## bob", "first remark"} {
		if !strings.Contains(got, want) {
			t.Errorf("the source lacks %q:\n%s", want, got)
		}
	}
}

func TestZoomFrame(t *testing.T) {
	body := "## Plan\n\nThe description has **bold** text and a list:\n\n- first item\n- second item\n\n" +
		strings.Repeat("A long paragraph that must wrap at the pager width. ", 6)
	for _, w := range []int{80, 120} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			f := &fakeReader{cached: true, body: body, warmPages: true,
				pages: map[string]core.Page[core.Comment]{"": {Items: []core.Comment{
					comment("first remark"), comment("second remark with `code`"),
				}}}}
			z := newZoom(f)
			shown(t, z, "a")
			z.Show("a", zoomItem(), previewOpts("unicode"))
			golden.RequireEqual(t, z.Draw(w-2, 20))
		})
	}
}

func TestZoomEmpty(t *testing.T) {
	z := newZoom(&fakeReader{})
	if cmd := z.Show("", Item{}, previewOpts("unicode")); cmd != nil || z.Shown() {
		t.Fatal("a row with nothing to show was shown")
	}
	if cmd := z.Show("h", Item{Kind: Header}, previewOpts("unicode")); cmd != nil || z.Shown() {
		t.Fatal("a header was shown")
	}
	z.Draw(40, 10) // an empty pager draws without panicking
	bad := zoomItem()
	bad.Ref.Problem = "gone"
	if Zoomable("a", bad) {
		t.Error("a row with a problem is zoomable")
	}
}

func TestZoomCommentError(t *testing.T) {
	f := &fakeReader{cached: true, commentErr: errors.New("boom")}
	z := newZoom(f)
	shown(t, z, "a")
	cmd := z.Show("a", zoomItem(), previewOpts("unicode"))
	if cmd == nil {
		t.Fatal("no read of the comments")
	}
	if !z.Handles(cmd()) {
		t.Fatal("the read isn't this pager's")
	}
	if next := z.Update(cmd()); next != nil {
		t.Error("a failed read asked for another")
	}
	if got := ansi.Strip(z.Draw(60, 30)); !strings.Contains(got, "could not read more comments: boom") {
		t.Errorf("no error line:\n%s", got)
	}
}

func TestZoomPageCap(t *testing.T) {
	pages := map[string]core.Page[core.Comment]{}
	for i := range maxCommentPages + 5 {
		cursor := ""
		if i > 0 {
			cursor = fmt.Sprint("p", i)
		}
		pages[cursor] = core.Page[core.Comment]{Items: []core.Comment{comment(fmt.Sprint("remark", i))}, Next: fmt.Sprint("p", i+1)}
	}
	f := &fakeReader{cached: true, warmPages: true, pages: pages}
	z := newZoom(f)
	shown(t, z, "a")
	if cmd := z.Show("a", zoomItem(), previewOpts("unicode")); cmd != nil {
		t.Fatal("a read past the cap")
	}
	got := ansi.Strip(z.Draw(60, 200))
	if !strings.Contains(got, "more comments on GitHub") {
		t.Errorf("no note of the cap:\n%s", got)
	}
	if !strings.Contains(got, fmt.Sprint("remark", maxCommentPages-1)) || strings.Contains(got, fmt.Sprint("remark", maxCommentPages)) {
		t.Errorf("the pager doesn't stop at %d pages:\n%s", maxCommentPages, got)
	}
}

func TestZoomHoverChangeCancels(t *testing.T) {
	f := &fakeReader{cached: true, pages: map[string]core.Page[core.Comment]{"": {Items: []core.Comment{comment("kept")}}}}
	z := newZoom(f)
	shown(t, z, "a")
	shown(t, z, "b")
	cmdA := z.Show("a", zoomItem(), previewOpts("unicode"))
	if cmdA == nil {
		t.Fatal("no read for a")
	}
	if z.Show("b", zoomItem(), previewOpts("unicode")) == nil {
		t.Fatal("no read for b")
	}
	// The read of a was cancelled by leaving it, so what it brings is dropped.
	if next := z.Update(cmdA()); next != nil {
		t.Error("a cancelled read went on")
	}
	if z.threads["a"].loading || len(z.threads["a"].items) != 0 {
		t.Errorf("thread a = %+v after the cancel", z.threads["a"])
	}
	if z.Show("a", zoomItem(), previewOpts("unicode")) == nil {
		t.Error("coming back to a didn't resume its read")
	}
}

func TestZoomStaleGenerationIgnored(t *testing.T) {
	z := newZoom(&fakeReader{cached: true})
	shown(t, z, "a")
	if z.Show("a", zoomItem(), previewOpts("unicode")) == nil {
		t.Fatal("no read")
	}
	stale := commentsMsg{id: z.id, key: "a", gen: z.threads["a"].gen - 1,
		page: core.Page[core.Comment]{Items: []core.Comment{comment("old")}}}
	if next := z.Update(stale); next != nil {
		t.Error("a stale page asked for another")
	}
	if len(z.threads["a"].items) != 0 || !z.threads["a"].loading {
		t.Errorf("a stale page changed the thread: %+v", z.threads["a"])
	}
}
