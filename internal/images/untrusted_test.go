package images

import (
	"errors"
	"strings"
	"testing"
)

// A refused host is named in the error, with the setting that allows it, only
// when trusting the host is what would let the address load.
func TestFetchUntrustedHostSaysHow(t *testing.T) {
	t.Parallel()
	const advice = "images.trusted_hosts"
	box := Box{Cols: 10, Rows: 5, CellWidth: 10, CellHeight: 20}
	for _, tc := range []struct {
		name, url string
		advise    bool
	}{
		{"untrusted host", "https://img.example.com/a.png", true},
		{"port 8443", "https://img.example.com:8443/a.png", false},
		{"http", "http://img.example.com/a.png", false},
		{"builtin host", "https://github.com/o/r/blob/x.png", false},
		{"no host", "/a.png", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := New("github.com")
			_, err := f.Fetch(t.Context(), Source{URL: tc.url, Index: -1}, box)
			if !errors.Is(err, ErrNotAllowed) {
				t.Fatalf("err = %v, want ErrNotAllowed", err)
			}
			if got := strings.Contains(err.Error(), advice); got != tc.advise {
				t.Errorf("err = %q, mentions %s = %v, want %v", err, advice, got, tc.advise)
			}
			if tc.advise {
				const want = "image host img.example.com is not trusted; add it to images.trusted_hosts"
				if err.Error() != want {
					t.Errorf("err = %q, want %q", err, want)
				}
			}
		})
	}
}
