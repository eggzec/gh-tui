package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// avatarModal returns the modal of a pull request open on three comments,
// whose bodies hold no image, so only avatars draw, 80 wide, drawing
// avatars with a.
func avatarModal(tb testing.TB, a *ui.Images) (*host, *detailModal) {
	tb.Helper()
	svc := newFakeService()
	svc.thread = uitest.Thread(3, clock)
	h := started(tb, svc, 80, 60, WithAvatars(a))
	h.SetTheme(theme(true))
	press(tb, h, "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter didn't open the pull request")
	}
	return h, m
}

// Where the terminal shows no images, the comments look as they do
// without avatars.
func TestCommentAvatarsOff(t *testing.T) {
	src := &uitest.ImageHost{}
	_, plain := threadModal(t, uitest.Thread(3, clock), true, 80, 60)
	_, off := avatarModal(t, uitest.Avatars(src, false))
	if off.View() != plain.View() {
		t.Errorf("avatars off changed the view:\n%s\nwant\n%s", off.View(), plain.View())
	}
}

// Where it does, each comment's head shows its author's avatar once it
// arrives, in the box it kept from the start.
func TestCommentAvatarsArrive(t *testing.T) {
	src := &uitest.ImageHost{}
	a := uitest.Avatars(src, true)
	h, m := avatarModal(t, a)
	if !strings.Contains(ansi.Strip(m.View()), "   hubot · ") {
		t.Errorf("no blank box before the login:\n%s", ansi.Strip(m.View()))
	}
	if _, changed := uitest.LoadAvatars(t, a); !changed {
		t.Fatal("no avatar arrived")
	}
	drain(t, h, h.Update(ui.ImagesMsg{}))
	if n := uitest.Placeholders(t, m.View()); n != 3*2 {
		t.Errorf("%d placeholder cells, want 2 for each of 3 comments", n)
	}
}

// An attachment alone on its line in a comment shows as the picture once
// it arrives, every cell naming it, where the terminal shows images, and
// as its text where it doesn't, as without images.
func TestCommentPictures(t *testing.T) {
	const shot = "https://github.com/user-attachments/assets/0d1c"
	comments := uitest.Thread(2, clock)
	comments[1].Body = "Before:\n\n![shot](" + shot + ")"
	open := func(a *ui.Images) (*host, *detailModal) {
		svc := newFakeService()
		svc.thread = comments
		h := started(t, svc, 80, 60, WithAvatars(a))
		h.SetTheme(theme(true))
		press(t, h, "enter")
		m := h.modal()
		return h, m
	}
	_, plain := threadModal(t, comments, true, 80, 60)
	_, off := open(uitest.Avatars(&uitest.ImageHost{}, false))
	if off.View() != plain.View() || !strings.Contains(ansi.Strip(off.View()), "🖼 shot") {
		t.Errorf("images off changed the view:\n%s\nwant\n%s", off.View(), plain.View())
	}
	src := &uitest.ImageHost{}
	a := uitest.Avatars(src, true)
	a.SetMaxRows(3)
	h, m := open(a)
	uitest.LoadAvatars(t, a)
	drain(t, h, h.Update(ui.ImagesMsg{}))
	if n := uitest.Placeholders(t, m.View()); n <= 2*2 {
		t.Errorf("%d placeholder cells, want the avatars' and the picture's", n)
	}
	if strings.Contains(ansi.Strip(m.View()), "🖼 shot") {
		t.Errorf("the picture shows as its text:\n%s", ansi.Strip(m.View()))
	}
}
