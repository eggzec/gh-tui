package ui

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func plusN(n int) string { return "+" + strconv.Itoa(n) }

func TestAvatarStripImages(t *testing.T) {
	f := &fakeFetch{}
	a := newTestAvatars(f, Graphics{Images: true})
	size := ImageSize{Cols: 2, Rows: 1, Round: true}
	users := []Person{
		{Login: "mona", AvatarURL: avatarOf("mona")},
		{Login: "hubot", AvatarURL: avatarOf("hubot")},
		{Login: "octo", AvatarURL: avatarOf("octo")},
	}
	if got := AvatarStrip(a, users, size, 20, plusN); got != strings.Repeat(" ", 8) {
		t.Fatalf("strip before the avatars arrived = %q, want 8 blank cells", got)
	}
	load(t, a)
	for _, b := range f.boxes {
		if !b.Round {
			t.Errorf("fetched %+v, want a round box", b)
		}
	}

	rows := func(u Person) string { return a.Box(SizedAvatar(u.AvatarURL, size), size)[0] }
	want := rows(users[0]) + " " + rows(users[1]) + " " + rows(users[2])
	if got := AvatarStrip(a, users, size, 20, plusN); got != want {
		t.Errorf("wide strip = %q, want %q", got, want)
	}
	if strings.Count(want, " ") < 2 || strings.Contains(want, "+") {
		t.Fatalf("want has no two avatars on one row: %q", want)
	}

	// Of four, two fit with room for +2: 2 + 1 + 2 + 1 + 2, but three do
	// not: 2 + 1 + 2 + 1 + 2 + 1 + 2.
	four := slices.Concat(users, []Person{{Login: "ghost", AvatarURL: avatarOf("ghost")}})
	got := AvatarStrip(a, four, size, 10, plusN)
	if w := ansi.StringWidth(got); w > 10 || !strings.HasSuffix(got, " +2") {
		t.Errorf("narrow strip = %q (%d cells), want two avatars and +2 in 10", got, w)
	}
	// Room for no avatar but the count.
	if got := AvatarStrip(a, users, size, 2, plusN); got != "+3" {
		t.Errorf("tiny strip = %q, want +3", got)
	}
	if got := AvatarStrip(a, users, size, 1, plusN); ansi.StringWidth(got) > 1 {
		t.Errorf("strip in 1 = %q, wider than 1", got)
	}
	if got := AvatarStrip(a, users, size, 0, plusN); got != "" {
		t.Errorf("strip in 0 = %q, want empty", got)
	}
}

func TestAvatarStripFailed(t *testing.T) {
	f := &fakeFetch{fail: map[string]error{}}
	a := newTestAvatars(f, Graphics{Images: true})
	size := ImageSize{Cols: 2, Rows: 1}
	u := Person{Login: "mona", AvatarURL: avatarOf("mona")}
	f.fail[SizedAvatar(u.AvatarURL, size)] = errors.New("connection reset")
	AvatarStrip(a, []Person{u}, size, 10, plusN)
	load(t, a)
	got := AvatarStrip(a, []Person{u}, size, 10, plusN)
	if strings.Contains(got, "@") || ansi.StringWidth(got) != 2 {
		t.Errorf("failed avatar = %q, want 2 blank cells and no handle", got)
	}
}

func TestAvatarStripText(t *testing.T) {
	a := newTestAvatars(&fakeFetch{}, Graphics{Cell: testCell})
	users := []Person{{Login: "mona"}, {Login: "hubot"}, {Login: "octo"}}
	size := ImageSize{Cols: 2, Rows: 1}
	for _, tt := range []struct {
		width int
		want  string
	}{
		{30, "@mona @hubot @octo"},
		{14, "@mona +2"},
		{12, "@mona +2"},
		{3, "+3"},
		{1, "+"},
		{0, ""},
	} {
		if got := AvatarStrip(a, users, size, tt.width, plusN); got != tt.want {
			t.Errorf("text strip in %d = %q, want %q", tt.width, got, tt.want)
		}
	}
}
