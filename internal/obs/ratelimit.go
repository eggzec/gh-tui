package obs

import (
	"cmp"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// RateEvent is what the client did with a request whose rate limit
// wouldn't let it through when it came.
type RateEvent int

// Rate events, counted per rate-limit resource.
const (
	// RateHeld is a request held until its limit lifts.
	RateHeld RateEvent = iota
	// RateReleased is a held request sent once its limit lifted.
	RateReleased
	// RateDropped is a held request whose context ended first.
	RateDropped
	// RateFailed is a request failed at once, rather than sent to be
	// refused, or held longer than it may wait.
	RateFailed
	numRate
)

// rateStats counts what the client did with requests that met a rate
// limit before they were sent.
type rateStats struct {
	// resources holds a *counters[RateEvent] per resource, and failed a
	// *atomic.Int64 per kind of request failed at once.
	resources, failed sync.Map
	// early counts the limits that outlasted the guard after their reset,
	// and probes the requests sent to learn whether a limit lifted.
	early, probes atomic.Int64
	// maxHold is the longest a released request was held, in
	// nanoseconds.
	maxHold atomic.Int64
}

// RateSummary covers what the client did with requests that met a rate
// limit before they were sent. FailedFast counts the requests failed at
// once by kind, such as prefetch or deadline, and Early the limits that
// were still on after the guard past their reset.
type RateSummary struct {
	Resources  []RateResourceSummary `json:"resources"`
	FailedFast map[string]int64      `json:"failed_fast"`
	Early      int64                 `json:"early_403"`
	Probes     int64                 `json:"probes"`
	MaxHoldMS  float64               `json:"max_hold_ms"`
}

// RateResourceSummary covers the requests of one rate-limit resource, such
// as core, that its limit stopped.
type RateResourceSummary struct {
	Resource string `json:"resource"`
	Held     int64  `json:"held"`
	Released int64  `json:"released"`
	Dropped  int64  `json:"dropped"`
	Failed   int64  `json:"failed"`
}

// Rate counts e for a request of resource.
func (s *Stats) Rate(resource string, e RateEvent) { count(&s.rate.resources, resource, e) }

// RateReleased counts a request of resource released after it was held
// for held.
func (s *Stats) RateReleased(resource string, held time.Duration) {
	s.Rate(resource, RateReleased)
	for {
		m := s.rate.maxHold.Load()
		if int64(held) <= m || s.rate.maxHold.CompareAndSwap(m, int64(held)) {
			return
		}
	}
}

// RateFailed counts a request of resource failed at once, since a request
// of its kind doesn't wait for its limit to lift.
func (s *Stats) RateFailed(resource, kind string) {
	s.Rate(resource, RateFailed)
	v, ok := s.rate.failed.Load(kind)
	if !ok {
		v, _ = s.rate.failed.LoadOrStore(kind, new(atomic.Int64))
	}
	v.(*atomic.Int64).Add(1)
}

// RateEarly counts a limit that was still on after the guard past its
// reset.
func (s *Stats) RateEarly() { s.rate.early.Add(1) }

// RateProbe counts a request sent to learn whether a limit lifted.
func (s *Stats) RateProbe() { s.rate.probes.Add(1) }

func (s *Stats) rateSummary() RateSummary {
	out := RateSummary{
		FailedFast: make(map[string]int64),
		Early:      s.rate.early.Load(),
		Probes:     s.rate.probes.Load(),
		MaxHoldMS:  Millis(time.Duration(s.rate.maxHold.Load())),
	}
	for resource, c := range each[RateEvent](&s.rate.resources) {
		out.Resources = append(out.Resources, RateResourceSummary{
			Resource: cmp.Or(resource, "none"), Held: c.get(RateHeld), Released: c.get(RateReleased),
			Dropped: c.get(RateDropped), Failed: c.get(RateFailed),
		})
	}
	slices.SortFunc(out.Resources, func(a, b RateResourceSummary) int { return cmp.Compare(a.Resource, b.Resource) })
	s.rate.failed.Range(func(k, v any) bool {
		out.FailedFast[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return out
}

// CountRate counts e for a request of resource in the default stats.
func CountRate(resource string, e RateEvent) { Default().Rate(resource, e) }

// CountRateReleased counts a held request released in the default stats.
func CountRateReleased(resource string, held time.Duration) {
	Default().RateReleased(resource, held)
}

// CountRateFailed counts a request failed at once in the default stats.
func CountRateFailed(resource, kind string) { Default().RateFailed(resource, kind) }

// CountRateEarly counts a limit that outlasted its guard in the default
// stats.
func CountRateEarly() { Default().RateEarly() }

// CountRateProbe counts a probe of the limits in the default stats.
func CountRateProbe() { Default().RateProbe() }
