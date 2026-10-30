package images

import (
	"errors"
	"testing"
)

// A refused host is named in the error, with the setting that allows it.
func TestFetchUntrustedHostSaysHow(t *testing.T) {
	t.Parallel()
	f := New("github.com")
	_, err := f.Fetch(t.Context(), Source{URL: "https://img.example.com:8443/a.png", Index: -1}, Box{Cols: 10, Rows: 5, CellWidth: 10, CellHeight: 20})
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v, want ErrNotAllowed", err)
	}
	const want = "image host img.example.com is not trusted; add it to images.trusted_hosts"
	if got := err.Error(); got != want {
		t.Errorf("err = %q, want %q", got, want)
	}
}
