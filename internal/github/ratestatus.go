package github

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// resourceOrder is the order the resources gh-tui uses most are shown in.
// Others follow by name.
var resourceOrder = []string{resourceCore, resourceGraphQL, resourceSearch, resourceCodeSearch}

// RateStatus returns the rate limits of the account as the client knows
// them now, with no request.
func (c *Client) RateStatus() core.RateStatus {
	return c.budget.snapshot()
}

// snapshot returns every quota at one moment, taken under one lock so
// that none is from before an answer that another reflects.
func (b *budget) snapshot() core.RateStatus {
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	s := core.RateStatus{
		Quotas:   make([]core.Quota, 0, len(b.quotas)),
		Answered: b.answered,
		Failed:   b.failed,
		At:       now,
	}
	if now.Before(b.gate.secondary) {
		s.SecondaryUntil = b.gate.secondary
	}
	if b.rejected.After(b.accepted) {
		s.Rejected = b.rejected
	}
	for resource, q := range b.quotas {
		cq := core.Quota{
			Resource:  resource,
			Limit:     q.limit,
			Remaining: b.left(q, now),
			Reset:     b.local(q.reset),
			SeenAt:    q.seenAt,
			Held:      b.held(resource),
		}
		if q.release.After(now) {
			cq.LimitedUntil = q.release
		}
		s.Quotas = append(s.Quotas, cq)
	}
	slices.SortFunc(s.Quotas, func(x, y core.Quota) int {
		return cmp.Or(cmp.Compare(resourceRank(x.Resource), resourceRank(y.Resource)), cmp.Compare(x.Resource, y.Resource))
	})
	return s
}

// left returns what is left of q at now, as GitHub said: never more
// within a window than before, since report takes no answer that says
// more. The requests in flight aren't taken off, although the gate does
// (est): one that GitHub doesn't count, such as a 304, one that got no
// answer, or a query that cost less than it was counted for, gives back
// what it took when it settles, which would show as the quota going up.
// Once its reset has passed the window has refilled, although GitHub may
// not have said so yet, so the full limit is left then. b.mu must be
// held.
func (b *budget) left(q *quota, now time.Time) int {
	if !b.local(q.reset).After(now) {
		return q.limit
	}
	return max(q.remaining, 0)
}

// resourceRank is where resource comes in resourceOrder, or after them
// all.
func resourceRank(resource string) int {
	if i := slices.Index(resourceOrder, resource); i >= 0 {
		return i
	}
	return len(resourceOrder)
}

// notifyEvery is the shortest time between two calls of the function
// that WithRateNotify sets.
const notifyEvery = time.Second

// WithRateNotify sets the function called when the rate limits change
// enough to show: a resource is spent or released, the count of requests
// held for a release changes, what is left crosses a step of 1% of the
// limit, a window starts or its reset passes, a secondary limit starts or
// lifts, or GitHub is answering again or no longer. It
// is called at most once a second, and a change within that second is
// told once it passes, so the last one is never missed. It is never
// called while the client holds a lock, so it may call RateStatus, and it
// must not block.
func WithRateNotify(notify func()) Option {
	return func(o *options) { o.notify = notify }
}

// changed tells the notifier, if any, that the rate limits may have
// changed. b.mu must not be held.
func (b *budget) changed() {
	if b.notifier != nil {
		b.notifier.poke()
	}
}

// rateNotifier calls notify when what a status bar shows of the rate
// limits changes, at most once each notifyEvery.
type rateNotifier struct {
	budget *budget
	notify func()

	mu sync.Mutex
	// shown is the view last told of, and told when.
	shown rateView
	told  time.Time
	// trailing tells of the changes made since told, once notifyEvery
	// has passed, or is nil if none is waiting.
	trailing *time.Timer
	// wake looks again at wakeAt, when a window refills or a spent
	// resource is released, since no answer need come then.
	wake    *time.Timer
	wakeAt  time.Time
	stopped bool
}

// rateView is what a status bar shows of the rate limits, as far as
// telling of a change goes.
type rateView struct {
	quotas []quotaView
	// offline is whether the last request that ended got no answer, and
	// rejected whether GitHub's last answer rejected the token.
	offline, rejected bool
	// secondary is when a secondary limit that holds every request
	// lifts, or zero if none does.
	secondary time.Time
}

type quotaView struct {
	resource string
	// window is the reset of the quota in GitHub's clock, which changes
	// with a new window only, not when the skew is learned better.
	window  time.Time
	limited bool
	held    int
	// step is the whole percent of the limit left, as GitHub reported
	// it, or all of it once the reset has passed.
	step int
}

func newRateNotifier(b *budget, notify func()) *rateNotifier {
	return &rateNotifier{budget: b, notify: notify}
}

// poke calls notify if the view changed since it was last told of, or
// has a timer do so once notifyEvery has passed since then.
func (n *rateNotifier) poke() {
	now := n.budget.now()
	v, next := n.budget.view(now)
	n.mu.Lock()
	if n.stopped {
		n.mu.Unlock()
		return
	}
	if next.After(now) && !next.Equal(n.wakeAt) {
		if n.wake != nil {
			n.wake.Stop()
		}
		n.wake, n.wakeAt = time.AfterFunc(next.Sub(now), n.poke), next
	}
	if n.trailing != nil || v.equal(n.shown) {
		n.mu.Unlock()
		return
	}
	if wait := n.told.Add(notifyEvery).Sub(now); wait > 0 {
		n.trailing = time.AfterFunc(wait, n.fire)
		n.mu.Unlock()
		return
	}
	n.shown, n.told = v, now
	n.mu.Unlock()
	n.notify()
}

// fire tells of the changes that waited for the throttle.
func (n *rateNotifier) fire() {
	n.mu.Lock()
	n.trailing = nil
	n.mu.Unlock()
	n.poke()
}

// stop stops the timers, and notify from being called again, except by
// a call already under way. It is safe on a nil notifier.
func (n *rateNotifier) stop() {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stopped = true
	for _, t := range []*time.Timer{n.trailing, n.wake} {
		if t != nil {
			t.Stop()
		}
	}
}

// view returns what a status bar shows of the rate limits at now, and
// the next time after now that it changes with no answer, when a window
// refills, a spent resource is released or a secondary limit lifts, or
// zero if none is.
func (b *budget) view(now time.Time) (v rateView, next time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v.offline = b.failed.After(b.answered)
	v.rejected = b.rejected.After(b.accepted)
	if now.Before(b.gate.secondary) {
		v.secondary = b.gate.secondary
		next = v.secondary
	}
	v.quotas = make([]quotaView, 0, len(b.quotas))
	soonest := func(t time.Time) {
		if t.After(now) && (next.IsZero() || t.Before(next)) {
			next = t
		}
	}
	for resource, q := range b.quotas {
		// What the snapshot shows, which a request in flight at a step
		// doesn't move to and fro.
		step := b.left(q, now)
		if q.limit > 0 {
			step = step * 100 / q.limit
		}
		soonest(b.local(q.reset))
		soonest(q.release)
		v.quotas = append(v.quotas, quotaView{
			resource: resource, window: q.reset, limited: q.release.After(now), held: b.held(resource), step: step,
		})
	}
	slices.SortFunc(v.quotas, func(x, y quotaView) int { return cmp.Compare(x.resource, y.resource) })
	return v, next
}

func (v rateView) equal(w rateView) bool {
	return v.offline == w.offline && v.rejected == w.rejected && v.secondary.Equal(w.secondary) && slices.EqualFunc(v.quotas, w.quotas, func(x, y quotaView) bool {
		return x.resource == y.resource && x.window.Equal(y.window) && x.limited == y.limited && x.held == y.held && x.step == y.step
	})
}
