package core

import "time"

// LowQuotaShare is the percent of a quota left under which the status bar
// warns of it, and under which reads ahead stop, leaving the rest to what
// the user asks for. It is one value, so that the bar turning to warn
// says that reads ahead have stopped.
const LowQuotaShare = 10

// Quota is one rate-limit resource as the client last saw it.
type Quota struct {
	// Resource names the quota, such as core, graphql, search or
	// code_search.
	Resource string
	Limit    int
	// Remaining is what GitHub last said is left, never below zero, and
	// never more than before within a window: an answer that says more
	// for the same Reset came from a request GitHub counted earlier. The
	// requests in flight aren't taken off. Once Reset has passed it is the
	// full Limit, until GitHub reports the new window.
	Remaining int
	// Reset is when the window refills, in local time. It stays the
	// passed reset until GitHub reports the new window.
	Reset time.Time
	// Held is how many requests wait for the resource to be released.
	Held int
	// LimitedUntil is when the spent resource is used again, in local
	// time, or zero if it isn't spent. It is a guard after Reset, so it
	// may outlast it.
	LimitedUntil time.Time
	// SeenAt is when GitHub last reported the quota.
	SeenAt time.Time
}

// RateStatus is the rate limits of one account on one host, and how
// recently GitHub answered it.
type RateStatus struct {
	// Quotas are the resources GitHub reported, in the order core,
	// graphql, search, code_search, then the others by name. A host with
	// rate limits turned off reports none.
	Quotas []Quota
	// SecondaryUntil is when a secondary rate limit that holds every
	// request lifts, or zero if none does.
	SecondaryUntil time.Time
	// Answered is when GitHub last answered a request, whatever its
	// status, and Failed when a request last got no answer. Before either
	// the connection is unknown, and it is offline only while Failed is
	// after Answered.
	Answered, Failed time.Time
	// Rejected is when GitHub rejected the token, if its last answer
	// did, or zero: the token expired or was revoked, and every request
	// fails until it is refreshed.
	Rejected time.Time
	// Failing is since when GitHub's answers, or a proxy's on the way to
	// it, have been server errors, or zero. It is the first error of the
	// run, and stays put while more come. A run shows only once its
	// errors have kept coming for a few seconds, so that a blip a retry
	// mends doesn't show, and hides once half a minute passed with no
	// more of them and GitHub answered well since. A later error of the
	// same resource within the hour takes the run up again, since when
	// it began, unless that resource answered well meanwhile, so that
	// GraphQL failing at each poll keeps the start of its outage; taken
	// up, it shows by the same rules as a new run. Reads are served what
	// earlier ones kept meanwhile.
	Failing time.Time
	// Mended is when the resource of a run of server errors that showed
	// first answered well after the run's last error, for the last run
	// that ended so, or zero. A run that only went quiet, because other
	// resources answered well meanwhile, hides from Failing but doesn't
	// move Mended: nothing that failed would read any better.
	Mended time.Time
	// At is when the status was taken.
	At time.Time
}

// SyncRateLimit is the sync key published when the rate limits change
// enough to show.
const SyncRateLimit = "ratelimit"
