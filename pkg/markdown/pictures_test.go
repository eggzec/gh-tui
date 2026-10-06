package markdown

import (
	"strconv"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

// fakePictures draws the images in ready as rows of x, as wide as the room
// or 6 cells, whichever is less, and records what it was asked.
type fakePictures struct {
	ready map[string]int // rows by address
	asked []string
}

func (f *fakePictures) draw(url string, width int) []string {
	f.asked = append(f.asked, url+" "+strconv.Itoa(width))
	n, ok := f.ready[url]
	if !ok {
		return nil
	}
	rows := make([]string, n)
	for i := range rows {
		rows[i] = "\x1b[38;5;7m" + strings.Repeat("x", min(width, 6)) + strconv.Itoa(i) + "\x1b[39m"
	}
	return rows
}

const picturesSrc = "Before.\n\n![shot](https://github.com/user-attachments/assets/1)\n\nText with ![inline](https://x.test/i.png) image.\n\n[![badge](https://x.test/b.svg)](https://x.test)\n\n<img alt=\"logo\" src=\"https://user-images.githubusercontent.com/2.png\">\n\nAfter."

// Images that stand alone on their lines show as Pictures draws them, on
// lines of their own, put in as they are; those in running text or in a
// link stay text, and so does one not drawn yet.
func TestPictures(t *testing.T) {
	f := &fakePictures{ready: map[string]int{"https://github.com/user-attachments/assets/1": 2}}
	r := New(DefaultStyle(true))
	r.SetPictures(f.draw)
	got := r.Render(picturesSrc, 40)
	lines := strings.Split(got, "\n")
	var rows []int
	for i, l := range lines {
		if strings.Contains(l, "\x1b[38;5;7mxxxxxx") {
			rows = append(rows, i)
			if !strings.Contains(l, "\x1b[38;5;7mxxxxxx"+strconv.Itoa(len(rows)-1)+"\x1b[39m") {
				t.Errorf("row %d = %q, want the picture's row as it was drawn", len(rows)-1, l)
			}
		}
	}
	if len(rows) != 2 || rows[1] != rows[0]+1 {
		t.Fatalf("picture rows at lines %v, want two in a row:\n%s", rows, got)
	}
	plain := xansi.Strip(got)
	for _, want := range []string{"Before.", "inline", "badge", "🖼 logo (https://user-", "After."} {
		if !strings.Contains(plain, want) {
			t.Errorf("render lacks %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "🖼 shot") {
		t.Errorf("the drawn image shows its text too:\n%s", plain)
	}
	for _, l := range lines {
		if w := xansi.StringWidth(l); w > 40 {
			t.Errorf("line %q is %d cells wide, over 40", l, w)
		}
	}
	for _, a := range f.asked {
		if strings.Contains(a, "x.test") {
			t.Errorf("asked for %q, an image in running text or a link", a)
		}
	}
}

// With no picture drawn, or no Pictures at all, a render is the one
// without pictures, byte for byte.
func TestPicturesNoneDrawn(t *testing.T) {
	want := New(DefaultStyle(true)).Render(picturesSrc, 40)
	r := New(DefaultStyle(true))
	f := &fakePictures{}
	r.SetPictures(f.draw)
	if got := r.Render(picturesSrc, 40); got != want {
		t.Errorf("with no picture drawn\n%q\nwant\n%q", got, want)
	}
	if len(f.asked) != 2 {
		t.Errorf("asked %q, want the two images alone on their lines", f.asked)
	}
	r.SetPictures(nil)
	if got := r.Render(picturesSrc, 40); got != want {
		t.Errorf("without pictures\n%q\nwant\n%q", got, want)
	}
}

// What Pictures draws is part of what a render is kept under: a render is
// reused while the pictures are the same, and made again, once, when one
// arrives.
func TestPicturesCache(t *testing.T) {
	f := &fakePictures{ready: map[string]int{}}
	r := New(DefaultStyle(true))
	r.SetPictures(f.draw)
	before := r.Render(picturesSrc, 40)
	n := r.Renders()
	r.Render(picturesSrc, 40)
	if r.Renders() != n {
		t.Errorf("rendered again with nothing changed")
	}
	f.ready["https://user-images.githubusercontent.com/2.png"] = 1
	after := r.Render(picturesSrc, 40)
	if r.Renders() == n || after == before {
		t.Fatal("an image that arrived didn't render again")
	}
	n = r.Renders()
	if again := r.Render(picturesSrc, 40); r.Renders() != n || again != after {
		t.Error("rendered again with the picture unchanged")
	}
}

// An image alone on its line is one written alone, with an address on the
// web; one in a list item, or with a relative address, isn't.
func TestAlone(t *testing.T) {
	for _, tt := range []struct {
		line, alt, url string
	}{
		{"![a](https://x.test/a.png)", "a", "https://x.test/a.png"},
		{"  ![a \\] b](<https://x.test/a b.png>)", "a ] b", ""},
		{"![](https://x.test/a.png \"title\") ", "", "https://x.test/a.png"},
		{"![a](<https://x.test/a.png>)", "a", "https://x.test/a.png"},
		{"- ![a](https://x.test/a.png)", "", ""},
		{"![a](docs/a.png)", "", ""},
		{"    ![a](https://x.test/a.png)", "", ""},
		{"x ![a](https://x.test/a.png)", "", ""},
	} {
		alt, url, ok := alone(tt.line, false)
		if ok != (tt.url != "") || alt != tt.alt && ok || url != tt.url {
			t.Errorf("alone(%q) = %q, %q, %v, want %q, %q", tt.line, alt, url, ok, tt.alt, tt.url)
		}
	}
}

// With relative addresses, an image alone may have one, but never one of
// another scheme or host.
func TestAloneRelative(t *testing.T) {
	for _, tt := range []struct {
		line, url string
	}{
		{"![a](docs/a.png)", "docs/a.png"},
		{"![a](./a.png?raw=true)", "./a.png?raw=true"},
		{"![a](/assets/a.png \"title\")", "/assets/a.png"},
		{"![a](<../a b.png>)", ""},
		{"![a](<../a.png>)", "../a.png"},
		{"![a](https://x.test/a.png)", "https://x.test/a.png"},
		{"![a](//x.test/a.png)", ""},
		{"![a](data:image/png;base64,AAAA)", ""},
		{"![a](javascript:alert(1))", ""},
		{"![a](file:///etc/passwd)", ""},
		{"- ![a](docs/a.png)", ""},
	} {
		_, url, ok := alone(tt.line, true)
		if ok != (tt.url != "") || url != tt.url {
			t.Errorf("alone(%q, true) = %q, %v, want %q", tt.line, url, ok, tt.url)
		}
	}
}

// A renderer told of relative addresses passes them to Pictures; one
// that isn't leaves them text.
func TestRelativePictures(t *testing.T) {
	src := "# Title\n\n![logo](img/logo.png)\n\nText.\n"
	var asked []string
	draw := func(url string, width int) []string {
		asked = append(asked, url)
		return []string{strings.Repeat("#", width)}
	}
	r := New(DefaultStyle(true))
	r.SetPictures(draw)
	if out := r.Render(src, 20); len(asked) != 0 || strings.Contains(out, "####") {
		t.Fatalf("drew %q without relative pictures:\n%s", asked, out)
	}
	r.SetRelativePictures(true)
	out := r.Render(src, 20)
	if len(asked) == 0 || asked[0] != "img/logo.png" || !strings.Contains(out, "####") {
		t.Errorf("asked %q, rendered:\n%s", asked, out)
	}
}

// An image that continues a quote or a list item lazily, or that is the
// text of a heading, stays in it as text; one indented into a list item
// stays in the item as a picture.
func TestPicturesInBlocks(t *testing.T) {
	const u = "https://github.com/user-attachments/assets/1"
	f := &fakePictures{ready: map[string]int{u: 1}}
	r := New(DefaultStyle(true))
	r.SetPictures(f.draw)
	for _, tt := range []struct {
		name, src string
		drawn     bool
	}{
		{"alone", "![a](" + u + ")", true},
		{"in a list item", "- item\n\n  ![a](" + u + ")\n- next", true},
		{"lazy in a quote", "> quoted\n![a](" + u + ")", false},
		{"lazy in a list item", "- item\n![a](" + u + ")", false},
		{"lazy after a lazy line in a quote", "> quoted\nlazy\n![a](" + u + ")", false},
		{"lazy after a lazy line in a list item", "- item\nlazy\n![a](" + u + ")", false},
		{"after a heading", "> quoted\n# heading\n![a](" + u + ")", true},
		{"a heading", "![a](" + u + ")\n---", false},
		{"after a paragraph", "text\n![a](" + u + ")", true},
	} {
		got := r.Render(tt.src, 40)
		if drawn := strings.Contains(got, "xxxxxx0"); drawn != tt.drawn {
			t.Errorf("%s: drawn %v, want %v:\n%s", tt.name, drawn, tt.drawn, xansi.Strip(got))
		}
		if tt.name == "in a list item" && !strings.Contains(xansi.Strip(got), "next") {
			t.Errorf("%s: the list lost its next item:\n%s", tt.name, xansi.Strip(got))
		}
	}
}

// A body with an image renders once, with its picture, and a body whose
// image isn't drawn yet renders the plain way too; a body without images
// renders once whatever Pictures draws, and keeps its render when what
// draws them changes.
func TestPicturesRenders(t *testing.T) {
	const u = "https://github.com/user-attachments/assets/1"
	f := &fakePictures{ready: map[string]int{u: 1}}
	r := New(DefaultStyle(true))
	r.SetPictures(f.draw)
	n := r.Renders()
	r.Render("![a]("+u+")", 40)
	if r.Renders()-n != 1 {
		t.Errorf("a drawn image took %d renders, want 1", r.Renders()-n)
	}
	n = r.Renders()
	r.Render("no images here", 40)
	r.SetPictures((&fakePictures{}).draw)
	r.Render("no images here", 40)
	if r.Renders()-n != 1 {
		t.Errorf("a body without images took %d renders across a new Pictures, want 1", r.Renders()-n)
	}
}
