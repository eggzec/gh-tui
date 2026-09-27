package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// timeoutTransport bounds each attempt at a request, from the time it is
// sent until its body is closed, as http.Client.Timeout bounds a whole
// call. So a request sent again gets a time of its own, rather than what
// the attempts before it left. It takes the place of the client's timeout.
type timeoutTransport struct {
	base    http.RoundTripper
	timeout time.Duration
}

// RoundTrip sends req with a deadline timeout from now, unless timeout
// isn't positive.
func (t *timeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.timeout <= 0 {
		return t.base.RoundTrip(req)
	}
	parent := req.Context()
	ctx, cancel := context.WithTimeout(parent, t.timeout)
	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		err = timedOut(parent, ctx, err)
		cancel()
		return nil, err
	}
	if resp.Body == nil {
		cancel()
		return resp, nil
	}
	resp.Body = &timeoutBody{ReadCloser: resp.Body, parent: parent, ctx: ctx, cancel: cancel}
	return resp, nil
}

// timedOut returns err, the error of an attempt with ctx, a child of
// parent, as a *timeoutError if the attempt's own deadline passed while
// parent is live: then the attempt, not its caller, gave up.
func timedOut(parent, ctx context.Context, err error) error {
	if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &timeoutError{err: err}
	}
	return err
}

// timeoutBody is the body of an attempt with a deadline, which it ends
// when it is closed.
type timeoutBody struct {
	io.ReadCloser
	parent, ctx context.Context
	cancel      context.CancelFunc
}

func (b *timeoutBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = timedOut(b.parent, b.ctx, err)
	}
	return n, err
}

func (b *timeoutBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

// timeoutError is an attempt that ran out of time. Like the error of
// http.Client's own timeout, it matches context.DeadlineExceeded, and its
// Timeout reports true.
type timeoutError struct{ err error }

func (e *timeoutError) Error() string {
	return "attempt timed out: " + e.err.Error()
}

func (e *timeoutError) Unwrap() error { return e.err }

func (*timeoutError) Timeout() bool { return true }

func (*timeoutError) Is(target error) bool { return target == context.DeadlineExceeded }
