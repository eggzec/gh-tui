package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// avatarModal returns the modal of a pull request open on comments, 80
// wide, drawing avatars with a.
func avatarModal(tb testing.TB, a *ui.Avatars) (*host, *detailModal) {
	tb.Helper()
	svc := newFakeService()
	svc.thread = uitest.Thread(4, clock)
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
	src := &uitest.ImageSource{}
	_, plain := threadModal(t, uitest.Thread(4, clock), true, 80, 60)
	_, off := avatarModal(t, uitest.Avatars(src, false))
	if off.View() != plain.View() {
		t.Errorf("avatars off changed the view:\n%s\nwant\n%s", off.View(), plain.View())
	}
}

// Where it does, each comment's head shows its author's avatar once it
// arrives, in the box it kept from the start.
func TestCommentAvatarsArrive(t *testing.T) {
	src := &uitest.ImageSource{}
	a := uitest.Avatars(src, true)
	h, m := avatarModal(t, a)
	if !strings.Contains(ansi.Strip(m.View()), "   hubot · ") {
		t.Errorf("no blank box before the login:\n%s", ansi.Strip(m.View()))
	}
	if _, changed := uitest.LoadAvatars(t, a); !changed {
		t.Fatal("no avatar arrived")
	}
	drain(t, h, h.Update(ui.AvatarsMsg{}))
	if n := uitest.Placeholders(t, m.View()); n != 4*2 {
		t.Errorf("%d placeholder cells, want 2 for each of 4 comments", n)
	}
}
