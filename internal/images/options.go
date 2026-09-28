package images

import (
	"net/http"
	"time"
)

// Store keeps what was fetched between sessions. It must be the account's
// own, such as its directory of the disk cache: the images of private
// repositories are kept in it under the addresses bodies name them by,
// which say nothing of who may see them, so a store another account reads
// would show it them. *disk.Store is one.
type Store interface {
	Get(kind, key string) ([]byte, bool)
	Put(kind, key string, data []byte) error
	Delete(kind, key string)
}

// Option configures a Fetcher.
type Option func(*Fetcher)

// WithHTML sets how the rendered HTML of bodies is read, which images of
// private repositories and of other hosts need. Without it, those stay
// unfetched.
func WithHTML(h HTML) Option {
	return func(f *Fetcher) { f.signer.html = h }
}

// WithStore keeps the images fetched in s, which is the account's own.
func WithStore(s Store) Option {
	return func(f *Fetcher) { f.store = s }
}

// WithOffline sets what says GitHub can't be reached, while which nothing
// is fetched.
func WithOffline(offline func() bool) Option {
	return func(f *Fetcher) { f.offline = offline }
}

// WithTransport sets what sends the requests, such as one of a test
// server. It must send each request as given, as an *http.Transport does:
// the fetcher checks a request's host and headers before handing it on,
// so a transport that adds credentials or follows redirects itself goes
// around those checks.
func WithTransport(t http.RoundTripper) Option {
	return func(f *Fetcher) { f.transport = t }
}

// WithClock sets the clock, for tests.
func WithClock(now func() time.Time) Option {
	return func(f *Fetcher) { f.now = now }
}
