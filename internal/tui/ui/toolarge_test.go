package ui

import (
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestOpenToLoadText(t *testing.T) {
	t.Parallel()
	got, ok := OpenToLoadText("a.bin", &core.TooLargeError{Size: 3 << 20, Limit: 2 << 20, Warn: true})
	want := "a.bin is 3 MiB, over the 2 MiB warning limit (files.preview.warn_size). Open it to load it anyway."
	if !ok || got != want {
		t.Errorf("OpenToLoadText = %q, %v, want %q", got, ok, want)
	}
	if _, ok := OpenToLoadText("a.bin", &core.TooLargeError{Size: 3 << 20, Limit: 2 << 20}); ok {
		t.Error("a file over the hard limit has no way to load it")
	}
}

func TestTooLargeText(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("read: %w", &core.TooLargeError{Size: 3 << 20, Limit: 2 << 20}), "a.bin is 3 MiB, over the 2 MiB limit (files.preview.max_size)."},
		{&core.TooLargeError{Limit: 2 << 20}, "a.bin is over the 2 MiB limit (files.preview.max_size)."},
		{core.ErrTooLarge, "a.bin is too large."},
	} {
		if got := TooLargeText("a.bin", tt.err); got != tt.want {
			t.Errorf("TooLargeText(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}
