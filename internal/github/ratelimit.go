package github

import (
	"net/http"
	"strconv"
	"time"
)

// secondaryBackoff is how long to wait after a secondary rate limit that
// does not say when it lifts, as GitHub's docs recommend.
const secondaryBackoff = time.Minute

// RateLimit is the quota GitHub reported with a response.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
	// Resource names the quota, such as core, graphql or search. Each has
	// its own limit.
	Resource string
}

// RateLimit returns the quota of resource, such as core or graphql, as
// GitHub last reported it, or the zero value before it did.
func (c *Client) RateLimit(resource string) RateLimit {
	st, _ := c.budget.status(resource)
	return st.reported
}

func parseRateLimit(h http.Header) (RateLimit, bool) {
	limit, err := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	if err != nil {
		return RateLimit{}, false
	}
	remaining, err := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if err != nil {
		return RateLimit{}, false
	}
	reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return RateLimit{}, false
	}
	return RateLimit{
		Limit:     limit,
		Remaining: remaining,
		Reset:     time.Unix(reset, 0),
		Resource:  h.Get("X-RateLimit-Resource"),
	}, true
}

// rateLimitReset reports whether resp was refused by a primary or secondary
// rate limit, and when to try again.
func (c *Client) rateLimitReset(resp *http.Response) (time.Time, bool) {
	if at, ok := c.retryAfter(resp.Header); ok {
		return at, true
	}
	if rl, ok := parseRateLimit(resp.Header); ok && rl.Remaining == 0 {
		var resource string
		if resp.Request != nil {
			resource = c.budget.classify(resp.Request)
		}
		return c.limitedUntil(resource, rl), true
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return c.budget.now().Add(secondaryBackoff), true
	}
	return time.Time{}, false
}

// retryAfter returns when the Retry-After of h says to send again, in
// seconds or as a date of GitHub's clock, if it says.
func (c *Client) retryAfter(h http.Header) (time.Time, bool) {
	s := h.Get("Retry-After")
	if secs, err := strconv.Atoi(s); err == nil {
		return c.budget.now().Add(time.Duration(secs) * time.Second), true
	}
	if t, err := http.ParseTime(s); err == nil {
		return c.budget.localTime(t), true
	}
	return time.Time{}, false
}

// limitedUntil returns when the quota of rl, which GitHub refused a request
// for, may be used again: the release of its resource, which is resource
// if rl doesn't name one. A reset too far away to be real is taken as a
// minute, as for a secondary limit that doesn't say.
func (c *Client) limitedUntil(resource string, rl RateLimit) time.Time {
	if at, ok := c.budget.refused(resource, rl); ok {
		return at
	}
	return c.budget.now().Add(secondaryBackoff)
}
