package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limits of the fetcher.
const (
	// maxFetches is how many images are fetched at once.
	maxFetches = 4
	// maxDecodes is how many are decoded at once: each may take a
	// maxPixels image's memory.
	maxDecodes = 2
	// memoryBytes is how many bytes of PNG are kept in memory.
	memoryBytes = 32 << 20
	// failedFor is how long an image that failed isn't tried again.
	failedFor = 10 * time.Minute
	// revalidateAfter is how old a kept avatar is before it is checked
	// again: a user may change theirs, while an attachment never changes.
	revalidateAfter = 24 * time.Hour
	// flightTimeout bounds a shared fetch: reading a body's HTML, two
	// requests of the image and its decoding.
	flightTimeout = 30 * time.Second
	// userAgent names the app to image hosts.
	userAgent = "gh-tui"
)

// Kinds of what the fetcher keeps in its store, named by a hash of the
// address the markdown or the API gave: the bytes fetched, and when and
// with what validator.
const (
	kindData = "image"
	kindMeta = "imagemeta"
)

// Source is an image to fetch.
type Source struct {
	// URL is the address the markdown or the API names the image by. It
	// names what is kept of it, so an address signed for a few minutes
	// is never kept.
	URL string
	// Body is the node ID of the body the image is in, such as a
	// comment, whose rendered HTML says where GitHub serves it, and
	// Index which of its images it is, in order. Body is "" for an image
	// outside a body, such as an avatar.
	Body  string
	Index int
	// Private says the body is of a private repository, whose
	// attachments load only from addresses GitHub signs.
	Private bool
}

// Fetcher fetches images and makes them fit the terminal. It is safe for
// concurrent use. Fetch blocks, so call it in a tea.Cmd.
type Fetcher struct {
	hosts     hosts
	client    *http.Client
	transport http.RoundTripper
	signer    signer
	store     Store
	offline   func() bool
	now       func() time.Time

	fetches, decodes chan struct{}

	mu      sync.Mutex
	mem     *memory
	failed  *failures
	flights map[string]*flight
}

// New returns a fetcher of the images of the GitHub whose web host is
// web, such as github.com or an Enterprise Server's host.
func New(web string, opts ...Option) *Fetcher {
	f := &Fetcher{
		hosts:   newHosts(web),
		now:     time.Now,
		fetches: make(chan struct{}, maxFetches),
		decodes: make(chan struct{}, maxDecodes),
		mem:     newMemory(memoryBytes),
		failed:  newFailures(),
		flights: make(map[string]*flight),
	}
	for _, opt := range opts {
		opt(f)
	}
	if f.transport == nil {
		f.transport = newTransport()
	}
	f.client = newClient(f.transport, f.hosts)
	f.signer.now, f.signer.hosts = f.now, f.hosts
	return f
}

// Fetch returns the image src names, made to fit box. It reads memory,
// then the store, then the network. An image that failed, but for being
// offline or cancelled, fails again at once for a while. Callers of the
// same image share one fetch, which stops only when all have gone.
func (f *Fetcher) Fetch(ctx context.Context, src Source, box Box) (Image, error) {
	key := src.URL + "\x00" + strconv.Itoa(box.Cols) + "x" + strconv.Itoa(box.Rows) +
		"@" + strconv.Itoa(box.CellWidth) + "x" + strconv.Itoa(box.CellHeight)
	if box.Animate {
		key += " animated"
	}
	f.mu.Lock()
	img, ok := f.mem.get(key)
	failed := f.failed.get(src.URL, f.now())
	f.mu.Unlock()
	if ok {
		return img, nil
	}
	if failed != nil {
		return Image{}, failed
	}
	return f.join(ctx, key, src, box)
}

// flight is a fetch that callers of the same image and box share. It runs
// on a context of its own, so it outlives any one caller, and is cancelled
// once every caller has gone.
type flight struct {
	done    chan struct{}
	img     Image
	err     error
	waiters int
	cancel  context.CancelFunc
}

// join waits for the fetch of key, starting it if none is running.
func (f *Fetcher) join(ctx context.Context, key string, src Source, box Box) (Image, error) {
	f.mu.Lock()
	fl, ok := f.flights[key]
	if !ok {
		// The caller's values, such as its trace, but not its deadline
		// or cancellation.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flightTimeout)
		fl = &flight{done: make(chan struct{}), cancel: cancel}
		f.flights[key] = fl
		go f.fly(fctx, fl, key, src, box)
	}
	fl.waiters++
	f.mu.Unlock()
	select {
	case <-fl.done:
		return fl.img, fl.err
	case <-ctx.Done():
		f.mu.Lock()
		defer f.mu.Unlock()
		fl.waiters--
		if fl.waiters == 0 {
			fl.cancel()
			// A caller that comes later starts afresh.
			if f.flights[key] == fl {
				delete(f.flights, key)
			}
		}
		return Image{}, ctx.Err()
	}
}

// fly runs the fetch of fl, keeps what it made or that it failed, and
// hands the result to its callers.
func (f *Fetcher) fly(ctx context.Context, fl *flight, key string, src Source, box Box) {
	defer fl.cancel()
	var img Image
	data, keep, err := f.data(ctx, src)
	if err == nil {
		img, err = f.decode(ctx, data, box)
		if keep && f.store != nil && (errors.Is(err, ErrFormat) || errors.Is(err, ErrTooLarge)) {
			// Kept bytes that don't decode would fail every session.
			name := storeKey(src.URL)
			f.store.Delete(kindData, name)
			f.store.Delete(kindMeta, name)
		}
	}
	if err != nil {
		err = scrubbed{err}
	}
	f.mu.Lock()
	switch {
	case err == nil && keep:
		f.mem.put(key, img)
		f.failed.forget(src.URL)
	case err == nil:
	case !errors.Is(err, ErrOffline) && ctx.Err() == nil:
		f.failed.add(src.URL, err, f.now())
	}
	if f.flights[key] == fl {
		delete(f.flights, key)
	}
	fl.img, fl.err = img, err
	f.mu.Unlock()
	close(fl.done)
}

// Decode makes data, an image read some other way, such as a file of a
// repository, fit box, within the same limits as an image fetched, and
// sharing the decodes that may run at once. It blocks, so call it in a
// tea.Cmd. Data that is no image this can show fails with ErrFormat, and
// one too large with ErrTooLarge.
func (f *Fetcher) Decode(ctx context.Context, data []byte, box Box) (Image, error) {
	return f.decode(ctx, data, box)
}

func (f *Fetcher) decode(ctx context.Context, data []byte, box Box) (Image, error) {
	select {
	case f.decodes <- struct{}{}:
		defer func() { <-f.decodes }()
	case <-ctx.Done():
		return Image{}, ctx.Err()
	}
	return decode(data, box)
}

// meta is what the store keeps of a fetch beside its bytes.
type meta struct {
	ETag    string    `json:"etag,omitempty"`
	Fetched time.Time `json:"fetched"`
}

// data returns the bytes of src, kept or fetched, and whether they may be
// kept under its address: not those of an image found in its body's HTML
// only by its place, which may be another image.
func (f *Fetcher) data(ctx context.Context, src Source) (data []byte, keep bool, err error) {
	name := storeKey(src.URL)
	var (
		kept []byte
		m    meta
	)
	if f.store != nil {
		if b, ok := f.store.Get(kindData, name); ok {
			kept = b
			if mb, ok := f.store.Get(kindMeta, name); ok {
				_ = json.Unmarshal(mb, &m)
			}
			if !f.stale(src, m) {
				return kept, true, nil
			}
		}
	}
	if f.offline != nil && f.offline() {
		if kept != nil {
			return kept, true, nil
		}
		return nil, false, ErrOffline
	}
	etag := ""
	if kept != nil {
		etag = m.ETag
	}
	data, newTag, placed, err := f.download(ctx, src, etag)
	switch {
	case errors.Is(err, errNotModified):
		data, newTag = kept, m.ETag
	case err != nil && kept != nil && !errors.Is(err, ErrUnavailable):
		// The kept copy does until the host answers.
		return kept, true, nil
	case err != nil:
		return nil, false, err
	case placed:
		return data, false, nil
	}
	if f.store != nil {
		// A cache only: a failure to keep costs a fetch next time. What
		// its header shows won't decode isn't kept.
		if !errors.Is(err, errNotModified) {
			if _, err := inspect(data); err != nil {
				return nil, false, err
			}
			_ = f.store.Put(kindData, name, data)
		}
		if mb, err := json.Marshal(meta{ETag: newTag, Fetched: f.now()}); err == nil {
			_ = f.store.Put(kindMeta, name, mb)
		}
	}
	return data, true, nil
}

// stale reports whether a kept image needs checking: an avatar a day
// after it was fetched. Other images never change under their address.
func (f *Fetcher) stale(src Source, m meta) bool {
	u, err := url.Parse(src.URL)
	if err != nil || !strings.HasPrefix(strings.ToLower(u.Hostname()), "avatars.") && !strings.Contains(u.Path, "/avatars/") {
		return false
	}
	return f.now().Sub(m.Fetched) >= revalidateAfter
}

var errNotModified = errors.New("not modified")

// download fetches src from where it may be fetched: its own address, if
// allowed, or the one its body's rendered HTML gives, for an attachment of
// a private repository, or for an image on another host, which then loads
// through GitHub's proxy. A signed address the host refuses is read again
// once, as it may have expired.
func (f *Fetcher) download(ctx context.Context, src Source, etag string) (data []byte, newTag string, placed bool, err error) {
	u, err := url.Parse(src.URL)
	if err != nil {
		return nil, "", false, fmt.Errorf("%w: %w", ErrNotAllowed, bareError(err))
	}
	direct := f.hosts.allowed(u) && (!src.Private || !f.hosts.attachment(u))
	if direct {
		data, newTag, err = f.get(ctx, u.String(), etag)
		// A private attachment the caller didn't say was private.
		var se *statusError
		if err == nil || src.Body == "" || !f.hosts.attachment(u) || !errors.As(err, &se) || !se.refused() {
			return data, newTag, false, err
		}
	}
	if src.Body == "" {
		return nil, "", false, fmt.Errorf("%w: %s", ErrNotAllowed, u.Host)
	}
	data, addr, placed, err := f.signed(ctx, src, "")
	if se := (*statusError)(nil); errors.As(err, &se) && se.refused() {
		data, _, placed, err = f.signed(ctx, src, addr)
	}
	// What a signed address serves is kept under the stable one, without
	// its validators, which the next address won't match.
	return data, "", placed, err
}

// signed fetches src from the address its body's rendered HTML gives, a
// new one if the host refused the address refused, and returns the
// address and whether it was found there only by its place.
func (f *Fetcher) signed(ctx context.Context, src Source, refused string) (data []byte, addr string, placed bool, err error) {
	addr, placed, err = f.signer.url(ctx, src.Body, src.URL, src.Index, refused)
	if err != nil {
		return nil, "", false, err
	}
	data, _, err = f.get(ctx, addr, "")
	return data, addr, placed, err
}

// get fetches addr, conditionally on etag if it isn't "".
func (f *Fetcher) get(ctx context.Context, addr, etag string) (data []byte, newTag string, err error) {
	select {
	case f.fetches <- struct{}{}:
		defer func() { <-f.fetches }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrNotAllowed, bareError(err))
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/webp")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch image: %w", bareError(err))
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified && etag != "":
		return nil, etag, errNotModified
	case resp.StatusCode != http.StatusOK:
		return nil, "", &statusError{code: resp.StatusCode}
	case resp.ContentLength > maxBytes:
		return nil, "", fmt.Errorf("%w: %d bytes", ErrTooLarge, resp.ContentLength)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image: %w", err)
	}
	if len(data) > maxBytes {
		return nil, "", fmt.Errorf("%w: over %d bytes", ErrTooLarge, maxBytes)
	}
	return data, resp.Header.Get("ETag"), nil
}

// storeKey names what is kept of the image at addr.
func storeKey(addr string) string {
	h := sha256.Sum256([]byte(addr))
	return hex.EncodeToString(h[:])
}

// bareError returns err with the address of the *url.Error in it, such
// as the client returns, made bare. What fails is remembered, returned
// and may be logged, while a signed address's query lets anyone holding
// it read a private image until it expires.
func bareError(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		ue.URL = bare(ue.URL)
	}
	return err
}

// bare returns addr without what may hold a secret: its query, such as
// the jwt of a signed address, its fragment and its user and password.
// It works on the text, so an address that doesn't parse, such as a
// redirect's malformed Location, is made bare too.
func bare(addr string) string {
	if i := strings.IndexAny(addr, "?#"); i >= 0 {
		addr = addr[:i]
	}
	scheme, rest, ok := strings.Cut(addr, "://")
	if !ok {
		return addr
	}
	authority := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		authority = rest[:i]
	}
	if i := strings.LastIndexByte(authority, '@'); i >= 0 {
		rest = rest[i+1:]
	}
	return scheme + "://" + rest
}

// webAddr finds the addresses in an error's text, up to the space, quote
// or bracket that ends them. A character escaped with a backslash, as %q
// escapes a quote within the address it quotes, doesn't end one.
var webAddr = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://(?:[^\s"'<>\\]|\\.)*`)

// scrubbed is an error that a fetch failed with, whose text has every
// address in it made bare: errors from below, such as the client's on a
// redirect whose Location doesn't parse, may quote an address whole,
// wherever in their text.
type scrubbed struct{ err error }

func (e scrubbed) Error() string { return webAddr.ReplaceAllStringFunc(e.err.Error(), bare) }

func (e scrubbed) Unwrap() error { return e.err }
