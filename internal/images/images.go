// Package images fetches the images that comments, bodies and profiles
// show, such as attachments and avatars, and makes them small PNGs to
// send to the terminal.
//
// It fetches with a client of its own that holds no credentials, only
// from GitHub's image hosts, on the first request and on every redirect,
// so the token never reaches an image host and a markdown author can't
// learn who reads their image: an image on any other host loads only
// through GitHub's proxy, camo, at the address GitHub rendered for it.
// None of these hosts count against the API's rate limit; only reading
// the rendered HTML of a body, which a private attachment or an image on
// another host needs, does.
//
// On an Enterprise Server, images on other hosts stay links. Its image
// proxy, where it has one, isn't at an address this package can tell from
// the server's own pages, and fetching those images directly would tell
// their hosts who reads them. Only its own avatars and attachments load.
package images

import (
	"errors"
	"fmt"
)

// Errors that Fetch returns, wrapped. An image that fails with any but
// ErrOffline is not tried again for a while.
var (
	// ErrNotAllowed is an address, or a redirect, to a host that images
	// aren't fetched from.
	ErrNotAllowed = errors.New("image host not allowed")
	// ErrUnavailable is an image the host refused or doesn't have.
	ErrUnavailable = errors.New("image unavailable")
	// ErrTooLarge is an image of more bytes or pixels than allowed.
	ErrTooLarge = errors.New("image too large")
	// ErrFormat is data that isn't a PNG, JPEG, GIF or lossy WebP image.
	ErrFormat = errors.New("not an image this can show")
	// ErrOffline is a fetch skipped while GitHub can't be reached.
	ErrOffline = errors.New("offline")
)

// statusError is a response other than success, which wraps
// ErrUnavailable for the ones that say the image isn't there for us.
type statusError struct {
	code int
}

func (e *statusError) Error() string { return fmt.Sprintf("image: HTTP %d", e.code) }

func (e *statusError) Unwrap() error {
	if e.refused() {
		return ErrUnavailable
	}
	return nil
}

// refused reports whether the host said the image isn't there for us,
// which a fresh signed address may change.
func (e *statusError) refused() bool {
	switch e.code {
	case 401, 403, 404, 410:
		return true
	}
	return false
}
