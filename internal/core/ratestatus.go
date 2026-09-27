package core

import "time"

// Quota is one rate-limit resource as the client last saw it.
type Quota struct {
	// Resource names the quota, such as core, graphql, search or
	// code_search.
	Resource string
	Limit    int
	// Remaining is what the client expects to be left once the requests
	// in flight are answered, never below zero. Once Reset has passed it
	// is the full Limit less those requests, until GitHub reports the new
	// window.
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
	// At is when the status was taken.
	At time.Time
}

// SyncRateLimit is the sync key published when the rate limits change
// enough to show.
const SyncRateLimit = "ratelimit"
