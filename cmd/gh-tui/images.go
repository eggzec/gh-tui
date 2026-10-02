package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/images"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// newImages returns the fetcher of the images of the GitHub whose web
// host is web. It keeps what it fetched in entries, the account's own
// directory of the disk cache, when there is one, and reads the rendered
// HTML of bodies with html, for the images that load only from where it
// says.
func newImages(web string, entries cache.Store, html images.HTML) *images.Fetcher {
	opts := []images.Option{images.WithHTML(html)}
	if entries != nil {
		opts = append(opts, images.WithStore(entries))
	}
	return images.New(web, opts...)
}

// blobReader reads the content of a file of a repository.
type blobReader interface {
	Blob(ctx context.Context, q filesvc.BlobQuery) (core.Blob, error)
}

// fetchImage adapts f to what the app's images fetch with. An image on the
// web is fetched by f, without credentials; a file of a repository is
// read through files, as the repository's other content is, and only
// decoded by f, within the same limits.
func fetchImage(f *images.Fetcher, files blobReader) ui.ImageFetch {
	return func(ctx context.Context, src ui.ImageSource, box ui.ImageBox) (ui.Picture, error) {
		b := images.Box{Cols: box.Cols, Rows: box.Rows, CellWidth: box.Cell.Width, CellHeight: box.Cell.Height, Animate: box.Animate}
		var (
			img images.Image
			err error
		)
		if src.SHA != "" {
			img, err = fetchFile(ctx, f, files, src, b)
		} else {
			// The images of markdown aren't counted as GitHub's HTML
			// counts them, among badges and inline images, so none is
			// found in it by its place.
			img, err = f.Fetch(ctx, images.Source{URL: src.URL, Body: src.Body, Index: -1, Private: src.Private}, b)
		}
		switch {
		case errors.Is(err, images.ErrUnavailable), errors.Is(err, images.ErrNotAllowed),
			errors.Is(err, images.ErrTooLarge), errors.Is(err, images.ErrFormat),
			errors.Is(err, core.ErrTooLarge), errors.Is(err, core.ErrNotFound):
			return ui.Picture{}, fmt.Errorf("%w: %w", ui.ErrImageGone, err)
		case err != nil:
			return ui.Picture{}, err
		}
		pic := ui.Picture{PNG: img.PNG, Width: img.Width, Height: img.Height, Cols: img.Cols, Rows: img.Rows, Loops: img.Loops}
		for _, fr := range img.Frames {
			pic.Frames = append(pic.Frames, ui.Frame{PNG: fr.PNG, Delay: fr.Delay})
		}
		return pic, nil
	}
}

// fetchFile reads the image file of src through files and decodes it to
// fit box.
func fetchFile(ctx context.Context, f *images.Fetcher, files blobReader, src ui.ImageSource, box images.Box) (images.Image, error) {
	blob, err := files.Blob(ctx, filesvc.BlobQuery{Repo: src.Repo, SHA: src.SHA, Size: src.Size})
	if err != nil {
		return images.Image{}, fmt.Errorf("read image file: %w", err)
	}
	return f.Decode(ctx, blob.Content, box)
}
