package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

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
	fetch := fetchImage(newImages("github.com", nil), blobs)
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
