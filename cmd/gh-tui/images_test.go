package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/imgcaps"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakeBlobs serves the content of files by their SHA, and records the
// queries.
type fakeBlobs struct {
	content map[string][]byte
	asked   []filesvc.BlobQuery
}

func (f *fakeBlobs) Blob(_ context.Context, q filesvc.BlobQuery) (core.Blob, error) {
	f.asked = append(f.asked, q)
	b, ok := f.content[q.SHA]
	if !ok {
		return core.Blob{}, core.ErrNotFound
	}
	return core.Blob{SHA: q.SHA, Size: int64(len(b)), Content: b, Binary: true}, nil
}

func pngOf(tb testing.TB, w, h int) []byte {
	tb.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		tb.Fatal(err)
	}
	return b.Bytes()
}

// An image file is read through the files service, never the image
// client, and decoded to fit its box, keeping its aspect; one that is no
// image the app shows, such as a lossless WebP, or that isn't there,
// won't load however often it is asked for.
func TestFetchImageFile(t *testing.T) {
	repo := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	blobs := &fakeBlobs{content: map[string][]byte{
		"wide": pngOf(t, 400, 100),
		// A lossless WebP, which the decoder refuses from its header.
		"vp8l": []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"),
	}}
	fetch := fetchImage(newImages("github.com", nil, nil), blobs)
	box := ui.ImageBox{Cols: 40, Rows: 20, Cell: imgcaps.Cell{Width: 10, Height: 20}}
	pic, err := fetch(t.Context(), ui.ImageSource{Repo: repo, SHA: "wide", Size: 9}, box)
	if err != nil {
		t.Fatal(err)
	}
	if pic.Cols != 40 || pic.Rows != 5 || pic.Width != 400 || pic.Height != 100 || len(pic.PNG) == 0 {
		t.Errorf("picture of %d×%d px in %d×%d cells, want 400×100 in 40×5", pic.Width, pic.Height, pic.Cols, pic.Rows)
	}
	if len(blobs.asked) != 1 || blobs.asked[0] != (filesvc.BlobQuery{Repo: repo, SHA: "wide", Size: 9}) {
		t.Errorf("read %+v, want the file's blob with its size", blobs.asked)
	}
	for _, sha := range []string{"vp8l", "missing"} {
		if _, err := fetch(t.Context(), ui.ImageSource{Repo: repo, SHA: sha}, box); !errors.Is(err, ui.ErrImageGone) {
			t.Errorf("%s: err = %v, want one that won't load", sha, err)
		}
	}
}

// An image of another host is looked for in the rendered HTML of the
// body the markdown said it is in, and only there: without a body it is
// never fetched, and no HTML is read.
func TestFetchImageOfBody(t *testing.T) {
	var asked [][]string
	html := func(_ context.Context, ids []string) (map[string]string, error) {
		asked = append(asked, ids)
		// GitHub's HTML of it holds no image.
		return map[string]string{"IC_1": "<p>moved</p>"}, nil
	}
	fetch := fetchImage(newImages("github.com", nil, html), &fakeBlobs{})
	box := ui.ImageBox{Cols: 40, Rows: 20, Cell: imgcaps.Cell{Width: 10, Height: 20}}
	external := "https://elsewhere.test/a.png"
	if _, err := fetch(t.Context(), ui.ImageSource{URL: external}, box); !errors.Is(err, ui.ErrImageGone) || len(asked) != 0 {
		t.Errorf("without a body: err = %v, HTML of %q read", err, asked)
	}
	if _, err := fetch(t.Context(), ui.ImageSource{URL: external + "?b", Body: "IC_1", Private: true}, box); !errors.Is(err, ui.ErrImageGone) {
		t.Errorf("with a body: err = %v, want one that won't load", err)
	}
	if len(asked) != 1 || len(asked[0]) != 1 || asked[0][0] != "IC_1" {
		t.Errorf("HTML of %q read, want that of IC_1 once", asked)
	}
}

// An animated GIF file asked for with its frames comes with them, their
// delays and its loops; asked for without, it is its first frame.
func TestFetchImageAnimation(t *testing.T) {
	r := image.Rect(0, 0, 20, 10)
	pal := color.Palette{color.Black, color.White}
	var b bytes.Buffer
	g := &gif.GIF{Image: []*image.Paletted{image.NewPaletted(r, pal), image.NewPaletted(r, pal)}, Delay: []int{5, 30}, LoopCount: -1}
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	repo := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	fetch := fetchImage(newImages("github.com", nil, nil), &fakeBlobs{content: map[string][]byte{"anim": b.Bytes()}})
	box := ui.ImageBox{Cols: 40, Rows: 20, Cell: imgcaps.Cell{Width: 10, Height: 20}, Animate: true}
	pic, err := fetch(t.Context(), ui.ImageSource{Repo: repo, SHA: "anim"}, box)
	if err != nil {
		t.Fatal(err)
	}
	if len(pic.Frames) != 2 || pic.Frames[0].Delay != 50*time.Millisecond || pic.Frames[1].Delay != 300*time.Millisecond || pic.Loops != 1 {
		t.Errorf("animation of %d frames, loops %d: %+v", len(pic.Frames), pic.Loops, pic.Frames)
	}
	box.Animate = false
	if pic, err := fetch(t.Context(), ui.ImageSource{Repo: repo, SHA: "anim"}, box); err != nil || len(pic.Frames) != 0 || len(pic.PNG) == 0 {
		t.Errorf("without animation: %d frames, err %v, want the first frame", len(pic.Frames), err)
	}
}
