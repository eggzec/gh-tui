package issues

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// avatarModal returns the modal of issue #999 open on comments, width
// wide, drawing avatars with a.
func avatarModal(tb testing.TB, a *ui.Images, width int) (*host, *detailModal) {
	tb.Helper()
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, uitest.Thread(4, testNow)...)
	h := started(tb, svc, width, 60, WithAvatars(a))
	h.SetTheme(theme(true))
	press(tb, h, "down", "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter didn't open the issue")
	}
	return h, m
}

// commentHeads returns the lines of v, plain, that head a comment.
func commentHeads(v string) []string {
	var heads []string
	for l := range strings.SplitSeq(ansi.Strip(v), "\n") {
		if strings.Contains(l, " · ") && strings.HasSuffix(strings.TrimSpace(l), "ago") && !strings.Contains(l, "opened") {
			heads = append(heads, l)
		}
	}
	return heads
}

// Where the terminal shows no images, the comments look as they do
// without avatars, with no room kept for them.
func TestCommentAvatarsOff(t *testing.T) {
	src := &uitest.ImageHost{}
	_, plain := threadModal(t, uitest.Thread(4, testNow), true, 80, 60)
	_, off := avatarModal(t, uitest.Avatars(src, false), 80)
	if off.View() != plain.View() {
		t.Errorf("avatars off changed the view:\n%s\nwant\n%s", off.View(), plain.View())
	}
	if len(src.Asked()) != 0 {
		t.Errorf("fetched %q", src.Asked())
	}
}

// Where it does, each comment's head keeps a box for its author's avatar
// from the start, which shows the avatar once it arrives, and nothing
// else on the line moves.
func TestCommentAvatarsArrive(t *testing.T) {
	src := &uitest.ImageHost{}
	a := uitest.Avatars(src, true)
	h, m := avatarModal(t, a, 80)
	heads := commentHeads(m.View())
	if len(heads) != 4 {
		t.Fatalf("%d comment heads, want 4:\n%s", len(heads), ansi.Strip(m.View()))
	}
	if !strings.HasPrefix(heads[0], "     hubot · ") {
		t.Errorf("head before the avatar arrived = %q, want a blank box before the login", heads[0])
	}
	if n := uitest.Placeholders(t, m.View()); n != 0 {
		t.Errorf("%d placeholder cells before any avatar arrived", n)
	}

	raw, changed := uitest.LoadAvatars(t, a)
	if !changed || strings.Count(raw, "a=t,") != 4 {
		t.Fatalf("sent %d avatars, changed %v; want the 4 authors'", strings.Count(raw, "a=t,"), changed)
	}
	if got := src.Asked(); len(got) != 4 || got[0] != "https://avatars.githubusercontent.com/u/hubot?s=40&v=4" {
		t.Errorf("fetched %q", got)
	}
	run(t, h, h.Update(ui.ImagesMsg{}))
	if n := uitest.Placeholders(t, m.View()); n != 4*2 {
		t.Errorf("%d placeholder cells, want 2 for each of 4 comments", n)
	}
	after := commentHeads(m.View())
	for i := range heads {
		text := strings.TrimSpace(heads[i])
		at := func(l string) int { return ansi.StringWidth(l[:max(strings.Index(l, text), 0)]) }
		if !strings.Contains(after[i], text) || at(after[i]) != at(heads[i]) {
			t.Errorf("head %d moved: %q, was %q", i, after[i], heads[i])
		}
	}
}

// A narrow modal cuts the head of a comment, and the avatar's cells keep
// what names the image.
func TestCommentAvatarsCut(t *testing.T) {
	src := &uitest.ImageHost{}
	a := uitest.Avatars(src, true)
	h, m := avatarModal(t, a, 80)
	uitest.LoadAvatars(t, a)
	run(t, h, h.Update(ui.ImagesMsg{}))
	for _, w := range []int{20, 8, 4} {
		m.SetSize(w, 60)
		uitest.Placeholders(t, m.View())
	}
	// A comment without an avatar, as one still sending, keeps the box,
	// so its head lines up with the others.
	c := core.Comment{Author: core.User{Login: "hubot"}}
	if got := ansi.Strip(m.renderComment(c, 80)); !strings.HasPrefix(got, "     hubot") {
		t.Errorf("comment head = %q", got)
	}
}
