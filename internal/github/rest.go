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
)

// Conditional holds the validators of an earlier response. Pass them to
// revalidate a cached entry: a 304 does not count against the rate limit.
type Conditional struct {
	ETag         string
	LastModified string
}

// Response describes a REST response apart from its body.
type Response struct {
	StatusCode   int
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
	return res, decode(resp.Body, v)
}

func newResponse(resp *http.Response) Response {
	links := parseLinks(resp.Header.Get("Link"))
	poll, _ := strconv.Atoi(resp.Header.Get("X-Poll-Interval"))
	rl, _ := parseRateLimit(resp.Header)
	return Response{
		StatusCode:   resp.StatusCode,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		Next:         links["next"],
		Last:         links["last"],
		PollInterval: time.Duration(poll) * time.Second,
		RateLimit:    rl,
	}
}

// decode reads JSON from r into v. An empty body, as in a 204, is not an
// error.
func decode(r io.Reader, v any) error {
	if v == nil {
		return nil
	}
	if err := json.NewDecoder(r).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
