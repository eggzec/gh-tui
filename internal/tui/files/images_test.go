package files

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// pngBytes stands for the content of an image file: binary, as a PNG is.
const pngBytes = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"

// imagePreview returns a preview of a file named name with the content of
// a PNG, drawn with images, sized 40 by 12, once its content arrived.
func imagePreview(t *testing.T, images *ui.Images, name string) *preview {
	t.Helper()
	f := newFake()
	e := file(name, int64(len(pngBytes)))
	f.addTree(ghTUI, "", e)
	f.addBlob(e, pngBytes)
	p := newPreview(t.Context(), f, "", ghTUI, "", e, key.NewBinding(key.WithKeys("o")), ui.Voice{}, "", ui.NewIcons(""), images)
	p.SetSize(40, 12)
	b, err := f.Blob(t.Context(), filesvc.BlobQuery{Repo: ghTUI, SHA: e.SHA, Size: e.Size})
	if err != nil {
		t.Fatal(err)
	}
	p.Update(blobMsg{id: p.pager.ID(), blob: b})
	return p
}

// An image file opens as its image, fitted to the pane above the pager's
// status line, once it arrives: its blob read as the repository's other
// files are, and its cells all naming it. A new size fits it anew.
func TestPreviewImage(t *testing.T) {
	src := &uitest.ImageHost{}
	images := uitest.Avatars(src, true)
	p := imagePreview(t, images, "docs/logo.PNG")
	if got := ansi.Strip(p.View()); !strings.Contains(got, "Loading the image…") {
		t.Errorf("before the image arrived the preview = %q, want it loading", got)
	}
	raw, changed := uitest.LoadAvatars(t, images)
	if !changed || !strings.Contains(raw, "a=p,U=1,") {
		t.Fatalf("sent %q, changed %v, want the image placed", raw, changed)
	}
	if want := []string{"blob eggzec/gh-tui b-docs/logo.PNG"}; !slices.Equal(src.Asked(), want) {
		t.Errorf("fetched %q, want %q", src.Asked(), want)
	}
	if !strings.Contains(raw, "c=40,r=11,") {
		t.Errorf("placed %q, want the 40×11 cells above the status line", raw)
	}
	p.Update(ui.ImagesMsg{})
	v := p.View()
	assertFits(t, v, 40, 12)
	if n := uitest.Placeholders(t, v); n != 40*11 {
		t.Errorf("%d cells show the image, want %d", n, 40*11)
	}
	if last := ansi.Strip(v[strings.LastIndexByte(v, '\n')+1:]); !strings.Contains(last, "docs/logo.PNG") {
		t.Errorf("status line = %q, want the file's name", last)
	}

	p.SetSize(30, 8)
	if got := ansi.Strip(p.View()); !strings.Contains(got, "Loading the image…") {
		t.Errorf("after a resize the preview = %q, want the new fit loading", got)
	}
	uitest.LoadAvatars(t, images)
	p.Update(ui.ImagesMsg{})
	v = p.View()
	assertFits(t, v, 30, 8)
	if n := uitest.Placeholders(t, v); n != 30*7 {
		t.Errorf("after a resize %d cells show the image, want %d", n, 30*7)
	}
}

// Where images aren't drawn, or for a file of another name, the preview
// shows what it showed before images, fetches no image and sends the
// terminal nothing.
func TestPreviewImageNotDrawn(t *testing.T) {
	before := imagePreview(t, nil, "logo.png").View()
	if !strings.Contains(ansi.Strip(before), "Binary file, not shown") {
		t.Fatalf("without images the preview = %q, want the binary file's notice", ansi.Strip(before))
	}
	for _, tt := range []struct {
		name   string
		file   string
		images bool
	}{
		{"images off", "logo.png", false},
		{"not an image's name", "logo.bin", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src := &uitest.ImageHost{}
			images := uitest.Avatars(src, tt.images)
			p := imagePreview(t, images, tt.file)
			p.Update(ui.ImagesMsg{})
			want := before
			if tt.file != "logo.png" {
				want = imagePreview(t, nil, tt.file).View()
			}
			if got := p.View(); got != want {
				t.Errorf("preview =\n%q\nwant as without images\n%q", got, want)
			}
			if raw, changed := uitest.LoadAvatars(t, images); raw != "" || changed || len(src.Asked()) != 0 {
				t.Errorf("sent %q, fetched %q", raw, src.Asked())
			}
		})
	}
}

// An image file that can't be shown, such as a lossless WebP, which the
// decoder refuses, shows as it did before images.
func TestPreviewImageFails(t *testing.T) {
	fail := func(context.Context, ui.ImageSource, ui.ImageBox) (ui.Picture, error) {
		return ui.Picture{}, ui.ErrImageGone
	}
	images := ui.NewImages(context.Background(), fail, true)
	images.SetGraphics(ui.Graphics{Images: true, Cell: uitest.TestCell})
	p := imagePreview(t, images, "logo.webp")
	if _, changed := uitest.LoadAvatars(t, images); !changed {
		t.Error("the failure didn't have the views draw again")
	}
	p.Update(ui.ImagesMsg{})
	if got := ansi.Strip(p.View()); !strings.Contains(got, "Binary file, not shown") {
		t.Errorf("preview = %q, want the binary file's notice", got)
	}
	// Drawn again, it isn't asked for again.
	p.Update(ui.ImagesMsg{})
	if cmd, _ := images.Load(); cmd != nil {
		t.Error("the failed image was asked for again")
	}
}

// An image file whose fetch failed in a way that may mend shows as it did
// before images, and is drawn once GitHub answers again.
func TestPreviewImageRetried(t *testing.T) {
	fail := true
	fetch := func(_ context.Context, _ ui.ImageSource, box ui.ImageBox) (ui.Picture, error) {
		if fail {
			return ui.Picture{}, errors.New("connection reset")
		}
		return ui.Picture{PNG: []byte("png"), Width: 10, Height: 20, Cols: box.Cols, Rows: box.Rows}, nil
	}
	images := ui.NewImages(context.Background(), fetch, true)
	images.SetGraphics(ui.Graphics{Images: true, Cell: uitest.TestCell})
	p := imagePreview(t, images, "logo.png")
	uitest.LoadAvatars(t, images)
	p.Update(ui.ImagesMsg{})
	if got := ansi.Strip(p.View()); !strings.Contains(got, "Binary file, not shown") {
		t.Fatalf("after the failure the preview = %q, want the binary file's notice", got)
	}
	fail = false
	if !images.Online() {
		t.Fatal("Online found nothing to ask again")
	}
	p.Update(ui.ImagesMsg{})
	uitest.LoadAvatars(t, images)
	p.Update(ui.ImagesMsg{})
	if n := uitest.Placeholders(t, p.View()); n == 0 {
		t.Errorf("once GitHub answered the image isn't drawn:\n%s", ansi.Strip(p.View()))
	}
}
