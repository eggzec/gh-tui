package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

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
	loc, resp, err := c.logLocationOf(ctx, repo, jobID)
	if err != nil {
		return nil, false, err
	}
	if resp != nil {
		defer resp.Body.Close()
		b, err := readLimited(ctx, resp, limit)
		return b, false, err
	}
	p, err := c.download(storageCall(ctx, repo), loc, limit)
	return p.Text, p.Start > 0, err
}

// LogPart is part of a job log: its bytes from Start on, of a log of
// Size bytes as the storage had it.
type LogPart struct {
	Text        []byte
	Start, Size int64
}

// JobLogFrom reads the log of job jobID of repo from byte offset on, for a
// job in progress, whose log GitHub's storage publishes in blocks as the
// runner uploads them. A part with no Text and Start at offset means that
// nothing was added. A part that starts elsewhere replaces what was read
// before, as when the log is shorter than offset, since it started over,
// or more than limit bytes were added, when JobLogFrom reads the last limit
// bytes at most, from the start of a line, as JobLog does.
//
// Each redirect to the storage costs a request of the rate limit, so its
// signed URL is used again for logURLTTL, and asked for again once the
// storage refuses it. Until the runner uploaded a block, reading fails
// with core.ErrLogPending. The storage sees no token, as with JobLog.
func (c *Client) JobLogFrom(ctx context.Context, repo core.RepoRef, jobID, offset, limit int64) (LogPart, error) {
	p, err := c.jobLogFrom(ctx, repo, jobID, offset, limit)
	if err != nil {
		return LogPart{}, fmt.Errorf("read log of job %d of %s from %d: %w", jobID, repo, offset, err)
	}
	return p, nil
}

func (c *Client) jobLogFrom(ctx context.Context, repo core.RepoRef, jobID, offset, limit int64) (LogPart, error) {
	key := strings.ToLower(repo.String()) + "/" + strconv.FormatInt(jobID, 10)
	loc, kept := c.logURLs.get(key, c.budget.now())
	if !kept {
		var resp *http.Response
		var err error
		if loc, resp, err = c.logLocationOf(ctx, repo, jobID); err != nil {
			return LogPart{}, err
		}
		if resp != nil {
			defer resp.Body.Close()
			b, err := readLimited(ctx, resp, limit)
			return LogPart{Text: b, Size: int64(len(b))}, err
		}
		c.logURLs.put(key, loc, c.budget.now())
	}
	p, err := c.downloadPart(storageCall(ctx, repo), loc, offset, limit)
	if kept && errors.Is(err, errStorageRefused) {
		// The signature ran out sooner than GitHub says.
		c.logURLs.drop(key)
		return c.jobLogFrom(ctx, repo, jobID, offset, limit)
	}
	return p, err
}

// downloadPart reads the log at loc from byte offset on, or else the last
// limit bytes of it at most.
func (c *Client) downloadPart(ctx context.Context, loc *url.URL, offset, limit int64) (LogPart, error) {
	if offset > 0 {
		if p, ok, err := c.downloadFrom(ctx, loc, offset, limit); ok || err != nil {
			return p, err
		}
	}
	return c.download(ctx, loc, limit)
}

// logURLTTL is how long a signed URL of the storage is used again: GitHub
// says that it expires after a minute, and a read must end before then.
const logURLTTL = 50 * time.Second

// logURLs keeps the signed URLs of the logs of jobs in progress, by job,
// until they expire.
type logURLs struct {
	mu sync.Mutex
	m  map[string]logURL
}

type logURL struct {
	loc   *url.URL
	until time.Time
}

func (u *logURLs) get(key string, now time.Time) (*url.URL, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	e, ok := u.m[key]
	if !ok || !now.Before(e.until) {
		return nil, false
	}
	return e.loc, true
}

// put keeps loc, which GitHub sent at now, under key, and forgets the URLs
// that expired.
func (u *logURLs) put(key string, loc *url.URL, now time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.m == nil {
		u.m = map[string]logURL{}
	}
	maps.DeleteFunc(u.m, func(_ string, e logURL) bool { return !now.Before(e.until) })
	u.m[key] = logURL{loc: loc, until: now.Add(logURLTTL)}
}

func (u *logURLs) drop(key string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.m, key)
}

// logLocationOf asks the API for the log of job jobID of repo, and returns
// where its storage serves it, or else the response of a server that
// serves the log itself, from the API host, which the caller must close.
func (c *Client) logLocationOf(ctx context.Context, repo core.RepoRef, jobID int64) (*url.URL, *http.Response, error) {
	u, err := c.resolve(jobPath(repo, jobID) + "/logs")
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, nil, err
	}
	// The redirect is followed by the caller, by hand, so that nothing of
	// the request reaches the storage but its URL.
	hc := *c.http
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.sendWith(&hc, req)
	if err != nil {
		return nil, nil, err
	}

	switch {
	case resp.StatusCode == http.StatusGone:
		defer resp.Body.Close()
		// Refused holds for this 410 too, but the log path doesn't ask it:
		// the service reads a kept log before GitHub, so there is nothing
		// to drop, and callers test ErrLogExpired instead.
		return nil, nil, fmt.Errorf("%w: %w", core.ErrLogExpired, c.httpError(resp))
	case isRedirect(resp.StatusCode):
		// The redirect has no body to wait for.
		resp.Body.Close()
		loc, err := c.logLocation(resp)
		return loc, nil, err
	case resp.StatusCode >= http.StatusMultipleChoices:
		defer resp.Body.Close()
		return nil, nil, c.httpError(resp)
	}
	return nil, resp, nil
}

// storageCall marks the requests with ctx as requests of the log of a job
// of repo to GitHub's storage, which the rate limits don't count, and
// whose log record leaves the URL out.
func storageCall(ctx context.Context, repo core.RepoRef) context.Context {
	return withCall(ctx, &call{op: jobLogRoute, repo: repo.String(), external: true})
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

// download reads the log at loc with none of the headers the API takes,
// or its last limit bytes at most, from the start of a line. Its log
// record comes from the call in ctx, which leaves the URL out.
func (c *Client) download(ctx context.Context, loc *url.URL, limit int64) (LogPart, error) {
	resp, err := c.get(ctx, loc, "")
	if err != nil {
		return LogPart{}, err
	}
	defer resp.Body.Close()
	if err := downloadError(resp); err != nil {
		return LogPart{}, err
	}
	if resp.ContentLength <= limit {
		b, err := readLimited(ctx, resp, limit)
		return LogPart{Text: b, Size: int64(len(b))}, err
	}

	size := resp.ContentLength
	resp.Body.Close()
	start := size - limit
	tail, err := c.get(ctx, loc, "bytes="+strconv.FormatInt(start, 10)+"-")
	if err != nil {
		return LogPart{}, err
	}
	defer tail.Body.Close()
	if err := downloadError(tail); err != nil {
		return LogPart{}, err
	}
	if tail.StatusCode != http.StatusPartialContent {
		return LogPart{}, &core.TooLargeError{Size: size, Limit: limit}
	}
	b, err := readLimited(ctx, tail, limit)
	if err != nil {
		return LogPart{}, err
	}
	// The range starts within a line most likely.
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b, start = b[i+1:], start+int64(i)+1
	}
	return LogPart{Text: b, Start: start, Size: size}, nil
}

// downloadFrom reads the log at loc from byte offset on, as download does,
// and reports false if the part to read isn't there: the log is shorter
// than offset, or more than limit bytes were added.
func (c *Client) downloadFrom(ctx context.Context, loc *url.URL, offset, limit int64) (LogPart, bool, error) {
	resp, err := c.get(ctx, loc, "bytes="+strconv.FormatInt(offset, 10)+"-")
	if err != nil {
		return LogPart{}, false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusRequestedRangeNotSatisfiable:
		// Nothing was added, unless the log became shorter, or the storage
		// didn't say how long it is.
		size, ok := rangeSize(resp.Header.Get("Content-Range"))
		if !ok || size < offset {
			return LogPart{}, false, nil
		}
		return LogPart{Start: offset, Size: offset}, true, nil
	case http.StatusPartialContent:
		start, size, ok := contentRange(resp.Header.Get("Content-Range"))
		if !ok || start != offset || size-offset > limit || resp.ContentLength > limit {
			return LogPart{}, false, nil
		}
		b, err := readLimited(ctx, resp, limit)
		if errors.Is(err, core.ErrTooLarge) {
			// More than limit was added to a log of unknown size.
			return LogPart{}, false, nil
		}
		if size < 0 {
			size = offset + int64(len(b))
		}
		return LogPart{Text: b, Start: offset, Size: size}, true, err
	case http.StatusOK:
		// A storage that doesn't serve ranges sends the whole log.
		if resp.ContentLength > limit {
			return LogPart{}, false, nil
		}
		b, err := readLimited(ctx, resp, limit)
		return LogPart{Text: b, Size: int64(len(b))}, true, err
	}
	return LogPart{}, false, downloadError(resp)
}

// contentRange parses the Content-Range of a part, "bytes 10-19/20", into
// the offset of its first byte and the size of the whole, which is -1 if
// the storage didn't say.
func contentRange(s string) (start, size int64, ok bool) {
	rng, total, ok := strings.Cut(strings.TrimPrefix(s, "bytes "), "/")
	first, _, ok2 := strings.Cut(rng, "-")
	if !ok || !ok2 {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(first, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	size = -1
	if total != "*" {
		if size, err = strconv.ParseInt(total, 10, 64); err != nil {
			return 0, 0, false
		}
	}
	return start, size, true
}

// rangeSize parses the Content-Range of a range refused as unsatisfiable,
// "bytes */20", into the size of the whole.
func rangeSize(s string) (int64, bool) {
	total, ok := strings.CutPrefix(s, "bytes */")
	if !ok {
		return 0, false
	}
	size, err := strconv.ParseInt(total, 10, 64)
	return size, err == nil
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
		return nil, offline(ctx, err)
	}
	return resp, nil
}

// errStorageRefused is how the storage refuses a signed URL, such as one
// that expired.
var errStorageRefused = errors.New("the storage refused the signed URL")

// downloadError is the error of a response of the storage: a 404 means
// that it holds none of the log yet, which GitHub stores whole once the
// job completed, and before that only in blocks of about 2 MiB.
func downloadError(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return core.ErrLogPending
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("download: %s: %w", resp.Status, errStorageRefused)
	case resp.StatusCode >= http.StatusMultipleChoices:
		return fmt.Errorf("download: %s", resp.Status)
	default:
		return nil
	}
}

// readLimited reads the body of resp, the response to a request with ctx,
// which must be at most limit bytes.
func readLimited(ctx context.Context, resp *http.Response, limit int64) ([]byte, error) {
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
