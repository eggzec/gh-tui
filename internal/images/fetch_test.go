package images

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// web is a fake of every host on the web: handlers by host, and a record
// of each request made.
type web struct {
	t        *testing.T
	mu       sync.Mutex
	handlers map[string]http.HandlerFunc
	reqs     []*http.Request
}

func newWeb(t *testing.T) (*web, http.RoundTripper) {
	t.Helper()
	w := &web{t: t, handlers: make(map[string]http.HandlerFunc)}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		w.reqs = append(w.reqs, r)
		h, ok := w.handlers[r.Host]
		w.mu.Unlock()
		if !ok {
			t.Errorf("request to %s, which has no handler", r.Host)
			http.NotFound(rw, r)
			return
		}
		h(rw, r)
	}))
	t.Cleanup(srv.Close)
	tr := srv.Client().Transport.(*http.Transport).Clone()
	// Every host is the test server, whose certificate names none of them.
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // G402: the test server's own certificate.
	return w, tr
}

func (w *web) handle(host string, h http.HandlerFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[host] = h
}

// hits counts the requests to host.
func (w *web) hits(host string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, r := range w.reqs {
		if r.Host == host {
			n++
		}
	}
	return n
}

func (w *web) all() []*http.Request {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]*http.Request(nil), w.reqs...)
}

func serve(data []byte) http.HandlerFunc {
	return func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write(data) }
}

// mapStore is a Store in memory.
type mapStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *mapStore) Get(kind, key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[kind+"/"+key]
	return b, ok
}

func (s *mapStore) Put(kind, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string][]byte)
	}
	s.m[kind+"/"+key] = append([]byte(nil), data...)
	return nil
}

func (s *mapStore) Delete(kind, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, kind+"/"+key)
}

// clock is a clock the test sets.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

var box = Box{Cols: 10, Rows: 5}

const (
	avatar     = "https://avatars.githubusercontent.com/u/1?s=64"
	assetUUID  = "0f1e2d3c-aaaa-bbbb-cccc-0123456789ab"
	attachment = "https://github.com/user-attachments/assets/" + assetUUID
)

// The token of the environment, or any credential, never reaches an
// image host.
func TestNoCredentials(t *testing.T) {
	t.Setenv("GH_TOKEN", "ghp_secret")
	t.Setenv("GITHUB_TOKEN", "ghp_secret")
	w, tr := newWeb(t)
	w.handle("avatars.githubusercontent.com", serve(pngOf(t, 8, 8)))
	w.handle("github.com", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "https://github-production-user-asset-6210df.s3.amazonaws.com/1/x.png", http.StatusFound)
	})
	w.handle("github-production-user-asset-6210df.s3.amazonaws.com", serve(pngOf(t, 8, 8)))
	f := New("github.com", WithTransport(tr))
	for _, addr := range []string{avatar, attachment} {
		if _, err := f.Fetch(t.Context(), Source{URL: addr}, box); err != nil {
			t.Fatalf("Fetch(%s): %v", addr, err)
		}
	}
	reqs := w.all()
	if len(reqs) != 3 {
		t.Fatalf("%d requests, want 3", len(reqs))
	}
	for _, r := range reqs {
		for _, h := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
			if v := r.Header.Get(h); v != "" {
				t.Errorf("%s got %s: %q", r.Host, h, v)
			}
		}
		for k, vs := range r.Header {
			if strings.Contains(strings.Join(vs, ","), "ghp_") {
				t.Errorf("%s got the token in %s", r.Host, k)
			}
		}
	}

	// A request that carries credentials anyway is refused unsent.
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, avatar, http.NoBody)
	req.Header.Set("Authorization", "token ghp_secret")
	if resp, err := f.client.Do(req); err == nil {
		resp.Body.Close()
		t.Error("a request with Authorization was sent")
	}
	if n := w.hits("avatars.githubusercontent.com"); n != 1 {
		t.Errorf("%d requests to the avatar host, want 1", n)
	}
}

func TestRedirects(t *testing.T) {
	w, tr := newWeb(t)
	w.handle("evil.test", serve(pngOf(t, 8, 8)))
	w.handle("github.com", func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/evil"):
			http.Redirect(rw, r, "https://evil.test/x.png", http.StatusFound)
		case strings.HasSuffix(r.URL.Path, "/http"):
			http.Redirect(rw, r, "http://avatars.githubusercontent.com/u/1", http.StatusFound)
		case strings.HasSuffix(r.URL.Path, "/api"):
			http.Redirect(rw, r, "https://api.github.com/user", http.StatusFound)
		default:
			// A loop of allowed addresses.
			http.Redirect(rw, r, r.URL.String()+"x", http.StatusFound)
		}
	})
	f := New("github.com", WithTransport(tr))
	for _, tail := range []string{"evil", "http", "api"} {
		_, err := f.Fetch(t.Context(), Source{URL: attachment + "/" + tail}, box)
		if !errors.Is(err, ErrNotAllowed) {
			t.Errorf("redirect %s: err = %v, want ErrNotAllowed", tail, err)
		}
	}
	if _, err := f.Fetch(t.Context(), Source{URL: attachment + "/loop"}, box); !errors.Is(err, errRedirects) {
		t.Errorf("redirect loop: err = %v, want too many redirects", err)
	}
	if n := w.hits("github.com"); n != 3+maxRedirects+1 {
		t.Errorf("%d requests to github.com, want %d", n, 3+maxRedirects+1)
	}
	if w.hits("evil.test") != 0 {
		t.Error("the redirect to evil.test was followed")
	}
}

func TestSizeLimits(t *testing.T) {
	w, tr := newWeb(t)
	big := make([]byte, maxBytes+1)
	w.handle("avatars.githubusercontent.com", func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/length":
			rw.Header().Set("Content-Length", strconv.Itoa(len(big)))
			_, _ = rw.Write(big)
		case "/chunked":
			// No length, so only reading it finds it too long.
			rw.(http.Flusher).Flush()
			_, _ = rw.Write(big)
		case "/bomb":
			_, _ = rw.Write(bombPNG(t, 5000, 5000))
		}
	})
	f := New("github.com", WithTransport(tr))
	for _, p := range []string{"/length", "/chunked", "/bomb"} {
		_, err := f.Fetch(t.Context(), Source{URL: "https://avatars.githubusercontent.com" + p}, box)
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: err = %v, want ErrTooLarge", p, err)
		}
	}
}

func TestCache(t *testing.T) {
	w, tr := newWeb(t)
	etag := `"v1"`
	w.handle("avatars.githubusercontent.com", func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			rw.WriteHeader(http.StatusNotModified)
			return
		}
		rw.Header().Set("ETag", etag)
		_, _ = rw.Write(pngOf(t, 64, 64))
	})
	w.handle("github.com", serve(pngOf(t, 16, 16)))
	store := &mapStore{}
	clk := &clock{now: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	f := New("github.com", WithTransport(tr), WithStore(store), WithClock(clk.Now))

	img, err := f.Fetch(t.Context(), Source{URL: avatar}, Box{Cols: 2, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if img.Cols != 2 || img.Rows != 1 || img.Width != 16 || img.Height != 16 {
		t.Errorf("avatar fit %dx%d px in %dx%d cells, want 16x16 in 2x1", img.Width, img.Height, img.Cols, img.Rows)
	}
	// Memory, then the store, which a new session reads.
	if _, err := f.Fetch(t.Context(), Source{URL: avatar}, Box{Cols: 2, Rows: 1}); err != nil {
		t.Fatal(err)
	}
	f2 := New("github.com", WithTransport(tr), WithStore(store), WithClock(clk.Now))
	if _, err := f2.Fetch(t.Context(), Source{URL: avatar}, Box{Cols: 4, Rows: 2}); err != nil {
		t.Fatal(err)
	}
	if n := w.hits("avatars.githubusercontent.com"); n != 1 {
		t.Fatalf("%d requests for the avatar, want 1", n)
	}

	// A day later the avatar is checked, and a 304 keeps the copy.
	clk.Add(revalidateAfter)
	f3 := New("github.com", WithTransport(tr), WithStore(store), WithClock(clk.Now))
	if _, err := f3.Fetch(t.Context(), Source{URL: avatar}, box); err != nil {
		t.Fatal(err)
	}
	reqs := w.all()
	if last := reqs[len(reqs)-1]; len(reqs) != 2 || last.Header.Get("If-None-Match") != etag {
		t.Errorf("%d requests, the last with If-None-Match %q; want 2, with %q", len(reqs), last.Header.Get("If-None-Match"), etag)
	}
	// An attachment never changes, so it isn't checked.
	for range 2 {
		f4 := New("github.com", WithTransport(tr), WithStore(store), WithClock(clk.Now))
		if _, err := f4.Fetch(t.Context(), Source{URL: attachment}, box); err != nil {
			t.Fatal(err)
		}
		clk.Add(30 * 24 * time.Hour)
	}
	if n := w.hits("github.com"); n != 1 {
		t.Errorf("%d requests for the attachment, want 1", n)
	}
}

func TestFailuresAreRemembered(t *testing.T) {
	w, tr := newWeb(t)
	w.handle("avatars.githubusercontent.com", http.NotFound)
	clk := &clock{now: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	offline := false
	f := New("github.com", WithTransport(tr), WithClock(clk.Now), WithOffline(func() bool { return offline }))
	for range 2 {
		if _, err := f.Fetch(t.Context(), Source{URL: avatar}, box); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	}
	if n := w.hits("avatars.githubusercontent.com"); n != 1 {
		t.Errorf("%d requests, want 1 until the failure is old", n)
	}
	clk.Add(failedFor)
	offline = true
	for range 2 {
		if _, err := f.Fetch(t.Context(), Source{URL: avatar}, box); !errors.Is(err, ErrOffline) {
			t.Fatalf("err = %v, want ErrOffline", err)
		}
	}
	offline = false
	_, _ = f.Fetch(t.Context(), Source{URL: avatar}, box)
	if n := w.hits("avatars.githubusercontent.com"); n != 2 {
		t.Errorf("%d requests, want 2: none while offline, and being offline isn't remembered", n)
	}
}

// htmlOf returns an HTML func that serves the bodies it holds and counts
// its calls.
func htmlOf(bodies func() map[string]string, calls *int) HTML {
	var mu sync.Mutex
	return func(_ context.Context, ids []string) (map[string]string, error) {
		mu.Lock()
		defer mu.Unlock()
		*calls++
		all := bodies()
		out := make(map[string]string)
		for _, id := range ids {
			out[id] = all[id]
		}
		return out, nil
	}
}

// An image on another host loads only through camo, at the address the
// body's HTML gives it.
func TestExternalOnlyThroughCamo(t *testing.T) {
	w, tr := newWeb(t)
	w.handle("evil.test", serve(pngOf(t, 8, 8)))
	w.handle("camo.githubusercontent.com", serve(pngOf(t, 8, 8)))
	external := "https://evil.test/track.png"
	calls := 0
	html := htmlOf(func() map[string]string {
		return map[string]string{
			"IC_1": `<p><a href="` + external + `"><img src="https://camo.githubusercontent.com/4a1b/68747470" data-canonical-src="` + external + `" alt="x"></a></p>`,
			// HTML that names the other host itself isn't followed.
			"IC_2": `<p><img src="` + external + `" data-canonical-src="` + external + `"></p>`,
		}
	}, &calls)
	f := New("github.com", WithTransport(tr), WithHTML(html))

	if _, err := f.Fetch(t.Context(), Source{URL: external, Body: "IC_1"}, box); err != nil {
		t.Fatalf("through camo: %v", err)
	}
	if _, err := f.Fetch(t.Context(), Source{URL: external + "?b", Body: ""}, box); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("without a body: err = %v, want ErrNotAllowed", err)
	}
	if _, err := f.Fetch(t.Context(), Source{URL: external + "?c", Body: "IC_2"}, box); err == nil {
		t.Error("an image the HTML left on the other host was fetched")
	}
	if w.hits("evil.test") != 0 {
		t.Error("the other host was reached")
	}
	if n := w.hits("camo.githubusercontent.com"); n != 1 {
		t.Errorf("%d requests to camo, want 1", n)
	}
}

// What was read of a body's HTML is used for a few minutes at most, even
// where its addresses don't expire, as those of the proxy don't, so an
// edited body is read again.
func TestBodyReadAgain(t *testing.T) {
	w, tr := newWeb(t)
	w.handle("camo.githubusercontent.com", serve(pngOf(t, 8, 8)))
	clk := &clock{now: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	external := "https://elsewhere.test/"
	calls := 0
	html := htmlOf(func() map[string]string {
		var b strings.Builder
		for _, n := range []string{"a", "b", "c"} {
			b.WriteString(`<img src="https://camo.githubusercontent.com/` + n + `" data-canonical-src="` + external + n + `">`)
		}
		return map[string]string{"IC_1": b.String()}
	}, &calls)
	f := New("github.com", WithTransport(tr), WithHTML(html), WithClock(clk.Now))
	for _, step := range []struct {
		after time.Duration
		image string
		reads int
	}{{0, "a", 1}, {time.Minute, "b", 1}, {4 * time.Minute, "c", 2}} {
		clk.Add(step.after)
		if _, err := f.Fetch(t.Context(), Source{URL: external + step.image, Body: "IC_1", Index: -1}, box); err != nil {
			t.Fatalf("image %s: %v", step.image, err)
		}
		if calls != step.reads {
			t.Errorf("image %s: HTML read %d times, want %d", step.image, calls, step.reads)
		}
	}
}

// jwtURL returns a private image address signed until exp.
func jwtURL(n int, exp time.Time) string {
	payload := base64.RawURLEncoding.EncodeToString(fmt.Appendf(nil, `{"exp":%d}`, exp.Unix()))
	return fmt.Sprintf("https://private-user-images.githubusercontent.com/1/%d-%s.png?jwt=h.%s.s", n, assetUUID, payload)
}

// A private attachment loads from the address the body's HTML signs, and
// a new one is read when it expires or the host refuses it.
func TestSignedAddresses(t *testing.T) {
	w, tr := newWeb(t)
	clk := &clock{now: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	var (
		mu     sync.Mutex
		signed int // which signature the HTML hands out
		good   = map[string]bool{}
	)
	sign := func() string {
		mu.Lock()
		defer mu.Unlock()
		signed++
		addr := jwtURL(signed, clk.Now().Add(5*time.Minute))
		good[strings.SplitN(addr, "/", 5)[4]] = true
		return addr
	}
	w.handle("private-user-images.githubusercontent.com", func(rw http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ok := good[strings.TrimPrefix(r.URL.RequestURI(), "/1/")]
		mu.Unlock()
		if !ok {
			http.Error(rw, "expired", http.StatusForbidden)
			return
		}
		_, _ = rw.Write(pngOf(t, 8, 8))
	})
	w.handle("github.com", http.NotFound)
	calls := 0
	html := htmlOf(func() map[string]string {
		addr := sign()
		return map[string]string{"IC_1": `<p><a href="` + addr + `"><img src="` + addr + `" alt="image"></a></p>`}
	}, &calls)
	f := New("github.com", WithTransport(tr), WithHTML(html), WithClock(clk.Now))
	src := func(q string) Source {
		return Source{URL: attachment + q, Body: "IC_1", Private: true}
	}

	if _, err := f.Fetch(t.Context(), src(""), box); err != nil {
		t.Fatal(err)
	}
	if w.hits("github.com") != 0 {
		t.Error("a private attachment was asked of github.com")
	}
	// Within its lifetime the signed address is used again.
	clk.Add(time.Minute)
	if _, err := f.Fetch(t.Context(), src("?again"), box); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("HTML read %d times within the signature's lifetime, want 1", calls)
	}
	// Once it has expired, a new one is read first.
	clk.Add(5 * time.Minute)
	if _, err := f.Fetch(t.Context(), src("?later"), box); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("HTML read %d times after the signature expired, want 2", calls)
	}
	// One the host refuses early is read again, once.
	mu.Lock()
	clear(good)
	mu.Unlock()
	if _, err := f.Fetch(t.Context(), src("?refused"), box); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("HTML read %d times after a refusal, want 3", calls)
	}
	// A private attachment not said to be private: github.com refuses it,
	// and the signed address just read serves it.
	if _, err := f.Fetch(t.Context(), Source{URL: attachment + "?unsaid", Body: "IC_1"}, box); err != nil {
		t.Fatal(err)
	}
	if w.hits("github.com") != 1 || calls != 3 {
		t.Errorf("%d requests to github.com and %d reads of HTML, want 1 and 3", w.hits("github.com"), calls)
	}
}

func TestExpiry(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	exp := now.Add(5 * time.Minute)
	tests := []struct {
		addr string
		want time.Time
	}{
		{jwtURL(1, exp), exp.Add(-expiryMargin)},
		{"https://github-production-user-asset-6210df.s3.amazonaws.com/x?X-Amz-Date=20260928T120000Z&X-Amz-Expires=300", exp.Add(-expiryMargin)},
		{"https://private-user-images.githubusercontent.com/x?jwt=garbage", now.Add(unknownExpiry)},
		{"https://camo.githubusercontent.com/x", time.Time{}},
	}
	for _, tt := range tests {
		if got := expiry(tt.addr, now); !got.Equal(tt.want) {
			t.Errorf("expiry(%s) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestMatch(t *testing.T) {
	imgs := images(`<img class="emoji" title=":+1:" src="https://github.githubassets.com/images/icons/emoji/unicode/1f44d.png">`+
		`<img src="https://camo.githubusercontent.com/a" data-canonical-src="https://x.test/a.png">`+
		`<img src="https://private-user-images.githubusercontent.com/1/2-`+assetUUID+`.png?jwt=a.b.c">`+
		`<img src="https://user-images.githubusercontent.com/3.png">`, time.Now())
	if len(imgs) != 3 {
		t.Fatalf("%d images, want 3: an emoji isn't one", len(imgs))
	}
	tests := []struct {
		stable     string
		index      int
		want       string
		ok, placed bool
	}{
		{"https://x.test/a.png", 5, "https://camo.githubusercontent.com/a", true, false},
		{attachment, 0, imgs[1].src, true, false},
		{"https://github.com/user-attachments/assets/ffffffff-aaaa-bbbb-cccc-0123456789ab", 1, "", false, false},
		{"https://user-images.githubusercontent.com/3.png", 2, "https://user-images.githubusercontent.com/3.png", true, true},
		// A proxied image isn't taken for another by its place.
		{"https://y.test/b.png", 0, "", false, false},
	}
	for _, tt := range tests {
		got, ok, placed := match(tt.stable, tt.index, imgs)
		if ok != tt.ok || placed != tt.placed || got.src != tt.want {
			t.Errorf("match(%s, %d) = %q, %v, %v; want %q, %v, %v", tt.stable, tt.index, got.src, ok, placed, tt.want, tt.ok, tt.placed)
		}
	}
}

// Callers of one image share its fetch, which outlives a caller that
// gives up, and stops once all have.
func TestSharedFetch(t *testing.T) {
	w, tr := newWeb(t)
	arrived := make(chan struct{}, 4)
	release := make(chan struct{})
	gone := make(chan struct{}, 4)
	w.handle("avatars.githubusercontent.com", func(rw http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		hold := release
		if strings.HasSuffix(r.URL.RawQuery, "&x") {
			// Never released: only its callers giving up ends it.
			hold = nil
		}
		select {
		case <-hold:
			_, _ = rw.Write(pngOf(t, 8, 8))
		case <-r.Context().Done():
			gone <- struct{}{}
		}
	})
	f := New("github.com", WithTransport(tr))

	first, cancel := context.WithCancel(t.Context())
	errs := make(chan error, 2)
	go func() {
		_, err := f.Fetch(first, Source{URL: avatar}, box)
		errs <- err
	}()
	<-arrived
	go func() {
		_, err := f.Fetch(t.Context(), Source{URL: avatar}, box)
		errs <- err
	}()
	// The second caller joins before the first gives up.
	for {
		f.mu.Lock()
		n := 0
		for _, fl := range f.flights {
			n = fl.waiters
		}
		f.mu.Unlock()
		if n == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that gave up: err = %v, want context.Canceled", err)
	}
	close(release)
	if err := <-errs; err != nil {
		t.Fatalf("the caller that waited: %v", err)
	}
	if n := w.hits("avatars.githubusercontent.com"); n != 1 {
		t.Errorf("%d requests, want 1 shared", n)
	}

	// When the only caller gives up, the request is cancelled, and that
	// isn't remembered as a failure.
	only, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := f.Fetch(only, Source{URL: avatar + "&x"}, box)
		done <- err
	}()
	<-arrived
	cancel()
	<-done
	select {
	case <-gone:
	case <-time.After(5 * time.Second):
		t.Fatal("the request went on after every caller had gone")
	}
	f.mu.Lock()
	_, failed := f.failed[avatar+"&x"]
	f.mu.Unlock()
	if failed {
		t.Error("a fetch every caller gave up on is remembered as failed")
	}
}

// Anyone can create a bucket named like GitHub's, so one is fetched only
// when an attachment on github.com redirects there.
func TestBucketOnlyByRedirect(t *testing.T) {
	w, tr := newWeb(t)
	evil := "github-production-user-asset-evil1.s3.amazonaws.com"
	w.handle(evil, serve(pngOf(t, 8, 8)))
	w.handle("github-production-user-asset-6210df.s3.amazonaws.com", serve(pngOf(t, 8, 8)))
	w.handle("github.com", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "https://github-production-user-asset-6210df.s3.amazonaws.com/1/x.png", http.StatusFound)
	})
	w.handle("avatars.githubusercontent.com", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "https://"+evil+"/x.png", http.StatusFound)
	})
	calls := 0
	html := htmlOf(func() map[string]string {
		return map[string]string{"IC_1": `<img src="https://` + evil + `/y.png" data-canonical-src="https://x.test/y.png">`}
	}, &calls)
	f := New("github.com", WithTransport(tr), WithHTML(html))

	for _, src := range []Source{
		{URL: "https://" + evil + "/x.png"},
		{URL: "https://x.test/y.png", Body: "IC_1"},
		{URL: avatar},
	} {
		if _, err := f.Fetch(t.Context(), src, box); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s: err = %v, want ErrNotAllowed", src.URL, err)
		}
	}
	if n := w.hits(evil); n != 0 {
		t.Errorf("%d requests to a bucket anyone could own", n)
	}
	// Both forms of github.com's attachments still redirect there.
	for _, addr := range []string{attachment, "https://github.com/octo/repo/assets/1/" + assetUUID} {
		if _, err := f.Fetch(t.Context(), Source{URL: addr}, box); err != nil {
			t.Errorf("%s: %v", addr, err)
		}
	}
	if n := w.hits("github-production-user-asset-6210df.s3.amazonaws.com"); n != 2 {
		t.Errorf("%d requests to GitHub's bucket, want 2", n)
	}
}

// An image found in its body's HTML only by its place may be another, so
// it isn't kept under the address the markdown named.
func TestPlacedNotKept(t *testing.T) {
	w, tr := newWeb(t)
	w.handle("private-user-images.githubusercontent.com", serve(pngOf(t, 8, 8)))
	calls := 0
	html := htmlOf(func() map[string]string {
		return map[string]string{"IC_1": `<img class="emoji" src="https://github.githubassets.com/e.png">` +
			`<img src="https://private-user-images.githubusercontent.com/1/2.png?jwt=x">`}
	}, &calls)
	store := &mapStore{}
	f := New("github.com", WithTransport(tr), WithHTML(html), WithStore(store))
	legacy := Source{URL: "https://user-images.githubusercontent.com/1/2.png", Body: "IC_1", Private: true}
	for range 2 {
		if _, err := f.Fetch(t.Context(), legacy, box); err != nil {
			t.Fatal(err)
		}
	}
	if n := w.hits("private-user-images.githubusercontent.com"); n != 2 {
		t.Errorf("%d requests, want 2: nothing kept in memory", n)
	}
	if _, ok := store.Get(kindData, storeKey(legacy.URL)); ok {
		t.Error("an image found by its place is kept on disk")
	}
}

func TestFailuresPruned(t *testing.T) {
	clk := &clock{now: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	f := New("github.com", WithClock(clk.Now))
	f.mu.Lock()
	defer f.mu.Unlock()
	f.remember("a", ErrUnavailable)
	clk.Add(failedFor / 2)
	f.remember("b", ErrUnavailable)
	clk.Add(failedFor / 2)
	f.remember("c", ErrUnavailable)
	if _, ok := f.failed["a"]; ok || len(f.failed) != 2 {
		t.Errorf("failures %v, want a pruned and b and c kept", f.failed)
	}
}

// Bytes that won't decode aren't kept: refused by their header before
// they are, or dropped when they fail to decode.
func TestUndecodableNotKept(t *testing.T) {
	corrupt := pngOf(t, 40, 40)
	// The header reads, the pixels don't.
	i := bytes.Index(corrupt, []byte("IDAT"))
	for j := i + 6; j < i+20; j++ {
		corrupt[j] ^= 0xff
	}
	for name, data := range map[string][]byte{"too large": bombPNG(t, 5000, 5000), "corrupt": corrupt} {
		w, tr := newWeb(t)
		w.handle("github.com", serve(data))
		store := &mapStore{}
		f := New("github.com", WithTransport(tr), WithStore(store))
		if _, err := f.Fetch(t.Context(), Source{URL: attachment}, box); err == nil {
			t.Errorf("%s: decoded", name)
		}
		if _, ok := store.Get(kindData, storeKey(attachment)); ok {
			t.Errorf("%s: kept", name)
		}
	}
	// What was kept before is dropped once it fails.
	store := &mapStore{}
	_ = store.Put(kindData, storeKey(attachment), corrupt)
	f := New("github.com", WithStore(store), WithOffline(func() bool { return true }))
	if _, err := f.Fetch(t.Context(), Source{URL: attachment}, box); !errors.Is(err, ErrFormat) {
		t.Errorf("kept corrupt: err = %v, want ErrFormat", err)
	}
	if _, ok := store.Get(kindData, storeKey(attachment)); ok {
		t.Error("kept corrupt bytes stay kept")
	}
}

// The images of one body share a read of its HTML, the first and the one
// after the host refuses the addresses it gave.
func TestOneReadPerBody(t *testing.T) {
	const n = 6
	w, tr := newWeb(t)
	var (
		mu     sync.Mutex
		signed int
		good   = map[string]bool{}
	)
	uuid := func(i int) string { return fmt.Sprintf("0f1e2d3c-aaaa-bbbb-cccc-%012d", i) }
	w.handle("private-user-images.githubusercontent.com", func(rw http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ok := good[r.URL.RequestURI()]
		mu.Unlock()
		if !ok {
			http.Error(rw, "expired", http.StatusForbidden)
			return
		}
		_, _ = rw.Write(pngOf(t, 8, 8))
	})
	calls := 0
	html := htmlOf(func() map[string]string {
		// Slow enough for every image to ask while it is read.
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		signed++
		var b strings.Builder
		for i := range n {
			path := fmt.Sprintf("/1/%d-%s.png?jwt=s%d", i, uuid(i), signed)
			// The first signatures are refused.
			good[path] = signed > 1
			b.WriteString(`<img src="https://private-user-images.githubusercontent.com` + path + `">`)
		}
		return map[string]string{"IC_1": b.String()}
	}, &calls)
	f := New("github.com", WithTransport(tr), WithHTML(html))
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			src := Source{URL: "https://github.com/user-attachments/assets/" + uuid(i), Body: "IC_1", Private: true}
			if _, err := f.Fetch(t.Context(), src, box); err != nil {
				t.Errorf("image %d: %v", i, err)
			}
		})
	}
	wg.Wait()
	if calls != 2 {
		t.Errorf("HTML read %d times for %d images, want 2: once, and once after the refusals", calls, n)
	}
}

// A decoder that panics fails every caller of the image, and what was
// kept of it is dropped.
func TestDecodePanicFailsFetch(t *testing.T) {
	panicking(t, false)
	store := &mapStore{}
	_ = store.Put(kindData, storeKey(attachment), pngOf(t, 4, 4))
	f := New("github.com", WithStore(store), WithOffline(func() bool { return true }))
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := f.Fetch(t.Context(), Source{URL: attachment}, box)
			errs <- err
		}()
	}
	for range 2 {
		select {
		case err := <-errs:
			if !errors.Is(err, ErrFormat) {
				t.Errorf("err = %v, want ErrFormat", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a caller is still waiting")
		}
	}
	if _, ok := store.Get(kindData, storeKey(attachment)); ok {
		t.Error("kept bytes that panicked stay kept")
	}
}
