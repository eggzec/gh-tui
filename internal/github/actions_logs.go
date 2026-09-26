package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// DefaultLogLimit is how much of a job log a caller should read at most:
// logs of large builds run to hundreds of megabytes, and their end, where
// a failure shows, is what matters.
const DefaultLogLimit = 10 << 20

// jobLogRoute names the download of a job log in the log.
const jobLogRoute = "/actions/jobs/{job_id}/logs"

// JobLog returns the log of job jobID of repo, as plain text in the
// format core.ParseLog reads. A log larger than limit bytes is read from
// its end: JobLog returns its last limit bytes at most, from the start of
// a line, with truncated set.
//
// GitHub answers with a redirect to a signed URL of its storage, valid for
// a few minutes. JobLog follows it without the token, which the storage
// neither needs nor may see, and which makes it refuse the request.
//
// The log of a job that hasn't completed isn't there yet, and reading it
// fails with core.ErrLogPending; once the repository's retention period
// passed, it fails with core.ErrLogExpired. If the storage doesn't serve
// the end of a log larger than limit, JobLog fails with a
// *core.TooLargeError.
func (c *Client) JobLog(ctx context.Context, repo core.RepoRef, jobID, limit int64) (text []byte, truncated bool, err error) {
	text, truncated, err = c.jobLog(ctx, repo, jobID, limit)
	if err != nil {
		return nil, false, fmt.Errorf("read log of job %d of %s: %w", jobID, repo, err)
	}
	return text, truncated, nil
}

func (c *Client) jobLog(ctx context.Context, repo core.RepoRef, jobID, limit int64) (text []byte, truncated bool, err error) {
	u, err := c.resolve(jobPath(repo, jobID) + "/logs")
	if err != nil {
		return nil, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, false, err
	}
	// The redirect is followed below, by hand, so that nothing of the
	// request reaches the storage but its URL.
	hc := *c.http
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.sendWith(&hc, req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusGone:
		// Refused holds for this 410 too, but the log path doesn't ask it:
		// the service reads a kept log before GitHub, so there is nothing
		// to drop, and callers test ErrLogExpired instead.
		return nil, false, fmt.Errorf("%w: %w", core.ErrLogExpired, c.httpError(resp))
	case isRedirect(resp.StatusCode):
		loc, err := c.logLocation(resp)
		if err != nil {
			return nil, false, err
		}
		// The redirect has no body to wait for.
		resp.Body.Close()
		cl := &call{op: jobLogRoute, repo: repo.String(), external: true}
		return c.download(withCall(ctx, cl), loc, limit)
	case resp.StatusCode >= http.StatusMultipleChoices:
		return nil, false, c.httpError(resp)
	}
	// A server may serve the log itself, from the API host.
	b, err := readLimited(resp, limit)
	return b, false, err
}

func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

// logLocation returns where resp redirects to. It must be HTTPS, unless
// the API itself isn't, as in tests.
func (c *Client) logLocation(resp *http.Response) (*url.URL, error) {
	loc, err := resp.Location()
	if err != nil {
		return nil, fmt.Errorf("redirect: %w", err)
	}
	if loc.Scheme != "https" && loc.Scheme != c.restURL.Scheme {
		return nil, fmt.Errorf("redirect to %s URL refused", loc.Scheme)
	}
	return loc, nil
}

// download reads the log at loc with none of the headers the API takes.
// Its log record comes from the call in ctx, which leaves the URL out.
func (c *Client) download(ctx context.Context, loc *url.URL, limit int64) (text []byte, truncated bool, err error) {
	resp, err := c.get(ctx, loc, "")
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if err := downloadError(resp); err != nil {
		return nil, false, err
	}
	if resp.ContentLength <= limit {
		b, err := readLimited(resp, limit)
		return b, false, err
	}

	size := resp.ContentLength
	resp.Body.Close()
	tail, err := c.get(ctx, loc, "bytes="+strconv.FormatInt(size-limit, 10)+"-")
	if err != nil {
		return nil, false, err
	}
	defer tail.Body.Close()
	if err := downloadError(tail); err != nil {
		return nil, false, err
	}
	if tail.StatusCode != http.StatusPartialContent {
		return nil, false, &core.TooLargeError{Size: size, Limit: limit}
	}
	b, err := readLimited(tail, limit)
	if err != nil {
		return nil, false, err
	}
	// The range starts within a line most likely.
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[i+1:]
	}
	return b, true, nil
}

// get sends a plain GET for loc, of the byte range rng if it isn't empty.
func (c *Client) get(ctx context.Context, loc *url.URL, rng string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// The error names the URL, which holds the signature.
		if ue, ok := errors.AsType[*url.Error](err); ok {
			ue.URL = loc.Scheme + "://" + loc.Host
		}
		return nil, err
	}
	return resp, nil
}

// downloadError is the error of a response of the storage: a 404 means
// that the job hasn't completed, since GitHub stores its log only then.
func downloadError(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return core.ErrLogPending
	case resp.StatusCode >= http.StatusMultipleChoices:
		return fmt.Errorf("download: %s", resp.Status)
	default:
		return nil
	}
}

// readLimited reads the body of resp, which must be at most limit bytes.
func readLimited(resp *http.Response, limit int64) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, &core.TooLargeError{Size: resp.ContentLength, Limit: limit}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(b)) > limit {
		return nil, &core.TooLargeError{Limit: limit}
	}
	return b, nil
}
