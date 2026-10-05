package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// Where the terminal shows no images, the dashboard looks as it does
// without avatars.
func TestProfileAvatarOff(t *testing.T) {
	plain := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, 120, 40)
	src := &uitest.ImageHost{}
	off := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, 120, 40, WithAvatars(uitest.Avatars(src, false)))
	if off.View() != plain.View() {
		t.Errorf("avatars off changed the view:\n%s\nwant\n%s", screen(off), screen(plain))
	}
}

// Where it does, the profile is as tall as the avatar's box from the
// start, with the box blank until the viewer's avatar arrives, and the
// panes share what is left.
func TestProfileAvatar(t *testing.T) {
	for _, size := range []struct{ w, h int }{{120, 40}, {80, 24}} {
		src := &uitest.ImageHost{}
		a := uitest.Avatars(src, true)
		s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, size.w, size.h, WithAvatars(a))
		lines := strings.Split(screen(s), "\n")
		if len(lines) != size.h {
			t.Fatalf("%d lines, want %d", len(lines), size.h)
		}
		for i, want := range []string{"Mona Lisa Octocat @octocat", "@github · San Francisco", ""} {
			if !strings.HasPrefix(lines[i], "        "+want) {
				t.Errorf("profile line %d = %q, want a blank box and then %q", i, lines[i], want)
			}
		}
		if !strings.HasPrefix(strings.TrimSpace(lines[3]), "╭") {
			t.Errorf("line 3 = %q, want the panes to start below the profile", lines[3])
		}
		if !s.onePane() {
			b := s.boxes
			if got := s.profileHeight() + b[pinnedPane].h + b[reposPane].h + b[calendarPane].h; got != size.h {
				t.Errorf("the rows take %d lines of %d", got, size.h)
			}
		}

		if _, changed := uitest.LoadAvatars(t, a); !changed {
			t.Fatal("the avatar didn't arrive")
		}
		if got := src.Asked(); len(got) != 1 || got[0] != "https://avatars.githubusercontent.com/u/583231?s=120&v=4" {
			t.Errorf("fetched %q, want the profile's avatar at the box's size", got)
		}
		run(t, s, s.Update(ui.ImagesMsg{}))
		if n := uitest.Placeholders(t, s.View()); n != 6*3 {
			t.Errorf("%d placeholder cells, want the 6×3 of the box", n)
		}
		for i, l := range strings.Split(s.View(), "\n") {
			if w := ansi.StringWidth(l); w != size.w {
				t.Errorf("line %d is %d wide, want %d", i, w, size.w)
			}
		}
	}
}

// A dashboard too narrow for the avatar beside the profile shows the
// profile without it, and every line keeps the width.
func TestProfileAvatarNarrow(t *testing.T) {
	for w := 1; w <= ownerui.MinAvatarWidth+2; w++ {
		src := &uitest.ImageHost{}
		a := uitest.Avatars(src, true)
		s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, w, 24, WithAvatars(a))
		uitest.LoadAvatars(t, a)
		run(t, s, s.Update(ui.ImagesMsg{}))
		for i, l := range strings.Split(s.View(), "\n") {
			if got := ansi.StringWidth(l); got != w {
				t.Errorf("width %d: line %d is %d wide", w, i, got)
			}
		}
		if n := uitest.Placeholders(t, s.View()); (n > 0) != (w >= ownerui.MinAvatarWidth) {
			t.Errorf("width %d: %d placeholder cells", w, n)
		}
	}
}
