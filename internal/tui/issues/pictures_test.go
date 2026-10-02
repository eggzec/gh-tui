package issues

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// shotURL is the address of an attachment, as a comment names it.
const shotURL = "https://github.com/user-attachments/assets/0d1c"

// pictureComments are comments of which the second holds an attachment
// alone on its line, and an image in running text.
func pictureComments() []core.Comment {
	cs := uitest.Thread(3, testNow)
	cs[1].Body = "Here is what I see:\n\n![shot](" + shotURL + ")\n\nAnd ![inline](https://x.test/i.png) too."
	return cs
}

// pictureModal returns the modal of issue #999 open on pictureComments,
// drawing images with a, if set.
func pictureModal(tb testing.TB, a *ui.Images) (*host, *detailModal) {
	tb.Helper()
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, pictureComments()...)
	var opts []Option
	if a != nil {
		opts = append(opts, WithAvatars(a))
	}
	h := started(tb, svc, 80, 60, opts...)
	h.SetTheme(theme(true))
	press(tb, h, "down", "enter")
	m := h.modal()
	if m == nil {
		tb.Fatal("enter didn't open the issue")
	}
	return h, m
}

// Where the terminal shows no images, a comment's images show as their
// text, exactly as without images, and none is fetched.
func TestCommentPicturesOff(t *testing.T) {
	_, plain := pictureModal(t, nil)
	src := &uitest.ImageHost{}
	a := uitest.Avatars(src, false)
	_, off := pictureModal(t, a)
	if off.View() != plain.View() {
		t.Errorf("images off changed the view:\n%s\nwant\n%s", off.View(), plain.View())
	}
	if !strings.Contains(ansi.Strip(off.View()), "🖼 shot") {
		t.Errorf("the attachment doesn't show as its text:\n%s", ansi.Strip(off.View()))
	}
	if raw, _ := uitest.LoadAvatars(t, a); raw != "" || len(src.Asked()) != 0 {
		t.Errorf("sent %q, fetched %q", raw, src.Asked())
	}
}

// Where it does, an attachment alone on its line shows as its text until
// it arrives, then as the picture, at most images.max_rows tall, with
// every cell naming it; one in running text stays text.
func TestCommentPicturesArrive(t *testing.T) {
	src := &uitest.ImageHost{}
	a := uitest.Avatars(src, true)
	a.SetMaxRows(4)
	h, m := pictureModal(t, a)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "🖼 shot") {
		t.Fatalf("before it arrived the attachment doesn't show as its text:\n%s", v)
	}
	uitest.LoadAvatars(t, a)
	if !slices.Contains(src.Asked(), shotURL) {
		t.Fatalf("fetched %q, want the attachment", src.Asked())
	}
	for _, u := range src.Asked() {
		if strings.Contains(u, "x.test") {
			t.Errorf("fetched %q, an image in running text", u)
		}
	}
	run(t, h, h.Update(ui.ImagesMsg{}))
	v := m.View()
	avatars := 2 * 3
	if n := uitest.Placeholders(t, v); n <= avatars {
		t.Fatalf("%d placeholder cells, want the avatars' %d and the picture's", n, avatars)
	}
	plain := ansi.Strip(v)
	if strings.Contains(plain, "🖼 shot") || !strings.Contains(plain, "🖼 inline") {
		t.Errorf("want the attachment drawn and the inline image as text:\n%s", plain)
	}
	rows := 0
	for l := range strings.SplitSeq(v, "\n") {
		if strings.Count(l, "\U0010EEEE") > 2 {
			rows++
		}
	}
	if rows != 4 {
		t.Errorf("the picture takes %d rows, want images.max_rows, 4", rows)
	}
}
