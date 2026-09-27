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
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil {
			return c.budget.now().Add(time.Duration(secs) * time.Second), true
		}
		if t, err := http.ParseTime(s); err == nil {
			return t, true
		}
	}
	if rl, ok := parseRateLimit(resp.Header); ok && rl.Remaining == 0 {
		return rl.Reset, true
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return c.budget.now().Add(secondaryBackoff), true
	}
	return time.Time{}, false
}
