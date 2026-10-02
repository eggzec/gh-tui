package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/images"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// newImages returns the fetcher of the images of the GitHub whose web
// host is web. It keeps what it fetched in entries, the account's own
// directory of the disk cache, when there is one.
func newImages(web string, entries cache.Store) *images.Fetcher {
	var opts []images.Option
	if entries != nil {
		opts = append(opts, images.WithStore(entries))
	}
	return images.New(web, opts...)
}

// fetchImage adapts f to what the app's avatars fetch with.
func fetchImage(f *images.Fetcher) ui.ImageFetch {
	return func(ctx context.Context, url string, box ui.ImageBox) (ui.Picture, error) {
		img, err := f.Fetch(ctx, images.Source{URL: url}, images.Box{
			Cols: box.Cols, Rows: box.Rows, CellWidth: box.Cell.Width, CellHeight: box.Cell.Height,
		})
		switch {
		case errors.Is(err, images.ErrUnavailable), errors.Is(err, images.ErrNotAllowed),
			errors.Is(err, images.ErrTooLarge), errors.Is(err, images.ErrFormat):
			return ui.Picture{}, fmt.Errorf("%w: %w", ui.ErrImageGone, err)
		case err != nil:
			return ui.Picture{}, err
		}
		return ui.Picture{PNG: img.PNG, Width: img.Width, Height: img.Height}, nil
	}
}
