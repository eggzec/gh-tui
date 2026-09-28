package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Conditional holds the validators of an earlier response. Pass them to
// revalidate a cached entry: a 304 does not count against the rate limit.
type Conditional struct {
	ETag         string
	LastModified string
}

// Response describes a REST response apart from its body.
type Response struct {
	StatusCode int
	// URL is the URL the response is for. Getting it again with the
	// validators revalidates what the response brought.
	URL          string
	ETag         string
	LastModified string
	// NotModified reports a 304: the cached entry is still current.
	NotModified bool
	// Next is the URL of the next page, or empty on the last page. Pass it
	// to Get as the path.
	Next string
	// Last is the URL of the last page, if GitHub reported one.
	Last string
	// PollInterval is the least time to wait before polling again, if
	// GitHub asked for one.
	PollInterval time.Duration
	// RateLimit is the zero value if the response reported none.
	RateLimit RateLimit
}

// Get fetches path and decodes the JSON body into v. If cond matches the
// current resource, Get returns a Response with NotModified set and leaves v
// untouched.
func (c *Client) Get(ctx context.Context, path string, cond Conditional, v any) (Response, error) {
	return c.rest(ctx, http.MethodGet, path, cond, nil, v)
}

// probeList asks whether the REST list at path changed since the response
// cond came from. It reads only the most recently updated item, whose
// update time changes whenever any item changes, so the ETag moves too.
// The body is read so the connection can be reused, and then dropped.
func (c *Client) probeList(ctx context.Context, path string, cond Conditional) (Response, error) {
	var latest []json.RawMessage
	return c.Get(ctx, path+"?state=all&sort=updated&direction=desc&per_page=1", cond, &latest)
}

// rawAccept asks for the raw content of a resource, such as a blob,
// rather than JSON.
const rawAccept = "application/vnd.github.raw+json"

// getRaw fetches path as the raw media type, which returns the resource's
// content instead of JSON, and reads at most limit bytes of it. A larger
// body is not read: it fails with a *core.TooLargeError.
func (c *Client) getRaw(ctx context.Context, path string, limit int64) ([]byte, error) {
	b, err := c.rawRoundTrip(ctx, path, limit)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", http.MethodGet, path, err)
	}
	return b, nil
}

func (c *Client) rawRoundTrip(ctx context.Context, path string, limit int64) ([]byte, error) {
	u, err := c.resolve(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", rawAccept)
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, c.httpError(resp)
	}
	if resp.ContentLength > limit {
		return nil, &core.TooLargeError{Size: resp.ContentLength, Limit: limit}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, readFailed(ctx, err)
	}
	if int64(len(b)) > limit {
		return nil, &core.TooLargeError{Limit: limit}
	}
	return b, nil
}

// Do sends body as JSON, unless it is nil, and decodes the response into v,
// unless it is nil. Use it for mutations.
func (c *Client) Do(ctx context.Context, method, path string, body, v any) (Response, error) {
	return c.rest(ctx, method, path, Conditional{}, body, v)
}

func (c *Client) rest(ctx context.Context, method, path string, cond Conditional, body, v any) (Response, error) {
	res, err := c.roundTrip(ctx, method, path, cond, body, v)
	if err != nil {
		return res, fmt.Errorf("%s %s: %w", method, path, err)
	}
	return res, nil
}

func (c *Client) roundTrip(ctx context.Context, method, path string, cond Conditional, body, v any) (Response, error) {
	u, err := c.resolve(path)
	if err != nil {
		return Response{}, err
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return Response{}, fmt.Errorf("encode body: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return Response{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cond.ETag != "" {
		req.Header.Set("If-None-Match", cond.ETag)
	}
	if cond.LastModified != "" {
		req.Header.Set("If-Modified-Since", cond.LastModified)
	}

	resp, err := c.send(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	res := newResponse(resp)
	switch {
	case resp.StatusCode == http.StatusNotModified:
		res.NotModified = true
		return res, nil
	case resp.StatusCode >= http.StatusMultipleChoices:
		return res, c.httpError(resp)
	}
	return res, decode(ctx, resp.Body, v)
}

func newResponse(resp *http.Response) Response {
	links := parseLinks(resp.Header.Get("Link"))
	poll, _ := strconv.Atoi(resp.Header.Get("X-Poll-Interval"))
	rl, _ := parseRateLimit(resp.Header)
	var u string
	if resp.Request != nil {
		u = resp.Request.URL.String()
	}
	return Response{
		StatusCode:   resp.StatusCode,
		URL:          u,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		Next:         links["next"],
		Last:         links["last"],
		PollInterval: time.Duration(poll) * time.Second,
		RateLimit:    rl,
	}
}

// decode reads JSON from r, the body of a response to a request with ctx,
// into v. An empty body, as in a 204, is not an error. A body that breaks
// off is an outage, as readFailed says, and one that isn't JSON is not.
func decode(ctx context.Context, r io.Reader, v any) error {
	if v == nil {
		return nil
	}
	br := &bodyReader{r: r}
	if err := json.NewDecoder(br).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		if br.err != nil {
			return readFailed(ctx, err)
		}
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// bodyReader reads r and keeps why reading it failed before its end, so
// that decode can tell a body that broke off from one that isn't JSON.
type bodyReader struct {
	r   io.Reader
	err error
}

func (b *bodyReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		b.err = err
	}
	return n, err
}

// readFailed is the error of a body of a response to a request with ctx
// that broke off while it was read, such as when the client's timeout
// struck or the connection dropped: an outage, tagged as offline tags a
// request that got no response, unless ctx is done.
func readFailed(ctx context.Context, err error) error {
	return offline(ctx, fmt.Errorf("read response: %w", err))
}
