package github

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"slices"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

const (
	// foregroundWait is the longest that a read the user waits for, or a
	// change they asked for, is held for a spent quota. One whose quota
	// refills later fails at once, so that what was kept of it can be
	// shown instead, with when the limit lifts.
	foregroundWait = 2 * time.Second
	// maxSecondaryWait is the longest that they are held for a secondary
	// limit, as its Retry-After says.
	maxSecondaryWait = 10 * time.Second
	// maxSecondaryBackoff bounds how long a secondary limit that doesn't
	// say how long it lasts is taken to last, which doubles each time one
	// comes again before a request succeeds.
	maxSecondaryBackoff = 15 * time.Minute
	// deadlineMargin is how long before its deadline a held request must
	// be sent at the latest. One that can't be fails at once, rather than
	// wait only to run out of time.
	deadlineMargin = 50 * time.Millisecond
	// minStagger and maxStagger bound the time between two held requests
	// let go one after the other, so that they don't all go the instant
	// their limit lifts.
	minStagger = 2 * time.Millisecond
	maxStagger = 10 * time.Millisecond
	// prefetchReserve is the share of a quota, in percent, that reads
	// ahead leave to what the user asks for.
	prefetchReserve = 10
	// probeTimeout bounds the probe of the rate limits, which the
	// requests held wait for, so no longer than the foreground waits.
	probeTimeout = foregroundWait
)

// class is who waits for a request, which decides what the gate does with
// it while its rate limit is on. Held requests are let go in the order of
// their classes.
type class int

const (
	// classForeground is a read the user waits for.
	classForeground class = iota
	// classMutation is a change the user asked for.
	classMutation
	// classBackground is a request of a background loop
	// (obs.IsBackground).
	classBackground
	// classPrefetch is a read ahead of its use (obs.IsPrefetch), which is
	// never held.
	classPrefetch
)

func (k class) String() string {
	return [...]string{"foreground", "mutation", "background", "prefetch"}[k]
}

// classOf returns the class of req, whose call is c, if it has one.
func classOf(req *http.Request, c *call) class {
	ctx := req.Context()
	switch {
	case obs.IsPrefetch(ctx):
		return classPrefetch
	case obs.IsBackground(ctx):
		return classBackground
	case req.Method == http.MethodGet, req.Method == http.MethodHead, c != nil && c.query:
		return classForeground
	}
	return classMutation
}

// gate is what the budget keeps to hold requests back while their rate
// limits are on. b.mu guards it.
type gate struct {
	// queues are the requests held, by resource.
	queues map[string]*queue
	// probe asks GitHub for the rate limits of every resource. probing is
	// set while it runs.
	probe   func(context.Context) (map[string]RateLimit, error)
	probing bool
	// jitter returns a number in [0, 1) that spreads the releases.
	jitter func() float64
	// next is when the request of the user let go last is sent, and
	// nextBackground the one of a background loop. The next one let go
	// follows by a stagger: one of the user only the user's, so that the
	// background never delays it, and one of the background both.
	next, nextBackground time.Time
	seq                  uint64
	// reached is the reset of the window of each resource whose limit was
	// logged as reached.
	reached map[string]time.Time

	// secondary is when the secondary limit lifts, which holds every
	// request, or zero. backoff is how long the next one that doesn't say
	// lasts, or zero for secondaryBackoff. timer moves the queues along
	// when it lifts.
	secondary time.Time
	backoff   time.Duration
	timer     *time.Timer
	// lifting is set once the secondary limit lifted with requests held,
	// until the first of them, the scout, is answered; the others wait
	// for its answer.
	lifting bool
	scout   uint64
	// closed is set once the client is closed, and then no timer is set.
	closed bool
}

func newGate() gate {
	return gate{queues: make(map[string]*queue), jitter: rand.Float64, reached: make(map[string]time.Time)}
}

// queue is the requests of one resource held until its limit lifts, by
// class and then in the order they came.
type queue struct {
	resource string
	held     []*hold
	// timer moves the queue along when its limit is due to lift.
	timer *time.Timer
	// scout is the reservation of the held request sent to learn whether
	// the limit lifted, since no probe could tell, or 0. noProbe is set
	// once a probe couldn't tell, until the limit lifts.
	scout   uint64
	noProbe bool
	// since is when the first of the held requests came, and released,
	// dropped and failed count what became of them, for the log.
	since                     time.Time
	released, dropped, failed int
}

// remove takes h out of q.
func (q *queue) remove(h *hold) {
	if i := slices.Index(q.held, h); i >= 0 {
		q.held = slices.Delete(q.held, i, i+1)
	}
}

// hold is a request held for its rate limit.
type hold struct {
	class    class
	seq      uint64
	resource string
	route    string
	cost     int
	// at is when it was held, and deadline when its context ends, or
	// zero if it doesn't.
	at, deadline time.Time
	// ready is closed once done is set: then r is what the request is sent
	// with, at sendAt, or err what it fails with.
	ready  chan struct{}
	done   bool
	r      *reservation
	sendAt time.Time
	err    error
}

// byTurn orders held requests by class, and then in the order they came.
func byTurn(a, b *hold) int {
	return cmp.Or(cmp.Compare(a.class, b.class), cmp.Compare(a.seq, b.seq))
}

// limit is what stops a request: until when it lasts, or zero if nothing
// says, whether it is a secondary limit, and whether an answer may lift it
// sooner: one to the requests in flight, since only they make the quota
// short of what GitHub reported left, or one that finds out whether a
// limit lifted, a probe's or a scout's.
type limit struct {
	until     time.Time
	secondary bool
	inFlight  bool
}

// tooLate returns why h can't wait for l to lift, or "" if it can: it
// couldn't be sent deadlineMargin before its deadline, it would be held
// longer than maxWindow, or longer than a request of its class waits for
// l. A limit that an answer may lift is waited for as long as the class
// waits, and then no longer.
func (h *hold) tooLate(l limit, now time.Time) string {
	until := later(l.until, now)
	if l.inFlight {
		until = now
	}
	switch {
	case !h.deadline.IsZero() && h.deadline.Before(until.Add(deadlineMargin)):
		return "deadline"
	case until.Sub(h.at) > maxWindow:
		return "cap"
	case h.class == classBackground:
	case until.Sub(h.at) > l.wait():
		return h.class.String()
	}
	return ""
}

// wait returns how long a read the user waits for, or a change they
// asked for, waits for l.
func (l limit) wait() time.Duration {
	if l.secondary {
		return maxSecondaryWait
	}
	return foregroundWait
}

// expiry returns when the first of held, which l stops, can wait no
// longer for l, if that comes before l lifts: once it was held longer than
// maxWindow, or than its class waits for an answer. It is zero if held is
// empty and l doesn't say when it lifts.
func (l limit) expiry(held []*hold) time.Time {
	at := l.until
	for _, h := range held {
		end := h.at.Add(maxWindow)
		if l.inFlight {
			if h.class != classBackground {
				end = h.at.Add(l.wait())
			}
			end = end.Add(time.Nanosecond)
		}
		if at.IsZero() || end.Before(at) {
			at = end
		}
	}
	return at
}

// admit counts req against its resource before it is sent, if its rate
// limit lets it through: a request of a resource whose quota is spent, or
// that costs more than is left of it once the requests in flight are
// answered, isn't sent only to be refused, nor any request while a
// secondary limit is on. What becomes of it then depends on who waits for
// it. A read ahead fails at once, and so does one that would take what is
// kept of the quota for what the user asks for (prefetchReserve). A read
// the user waits for, or a change they asked for, is held if the limit
// lifts within foregroundWait, or maxSecondaryWait for a secondary limit,
// and fails at once otherwise; if only the requests in flight make the
// quota short of what GitHub reported left, it waits up to foregroundWait
// for their answers. A request of a background loop is held until the
// limit lifts. A request that couldn't be sent deadlineMargin before its
// deadline fails at once, and one held longer than maxWindow fails then.
// Each fails with a *core.RateLimitError that says when the limit lifts,
// and a request whose context ends while it is held with the context's
// error.
//
// admit returns the reservation that observe or forget settles, or nil
// for a request to a host outside the API, which no limit covers, and how
// long the request was held.
func (b *budget) admit(req *http.Request) (*reservation, time.Duration, error) {
	ctx := req.Context()
	c, _ := ctx.Value(callKey{}).(*call)
	if c != nil && c.external || req.URL.Host != b.host {
		return nil, 0, nil
	}
	resource := b.classify(req)
	var route string
	if resource == resourceCore {
		route = b.route(req)
	}
	h := &hold{class: classOf(req, c), resource: resource, route: route}
	h.deadline, _ = ctx.Deadline()
	r, until, err := b.enter(ctx, h, c)
	if r != nil || err != nil {
		return r, 0, err
	}
	if obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "rate limit hold", "span", "http", "resource", h.resource,
			"class", h.class.String(), "until", until)
	}
	return b.wait(ctx, h)
}

// enter decides, in one step with counting it, what becomes of h, a
// request of call c that came to the gate, as admit says: it returns the
// reservation of a request that may be sent at once, or the error of one
// that fails at once, or neither for one it holds, and until when its
// limit lasts.
func (b *budget) enter(ctx context.Context, h *hold, c *call) (*reservation, time.Time, error) {
	defer b.changed()
	b.mu.Lock()
	defer b.unlock()
	now := b.now()
	h.at = now
	h.resource = cmp.Or(b.learned[h.route], h.resource)
	h.cost = b.cost(h.resource, c)
	l, on := b.limitOn(h.resource, h.cost, now)
	until := l.until
	var why string
	switch {
	case on && h.class == classPrefetch:
		why = h.class.String()
	case on:
		why = h.tooLate(l, now)
	case h.class == classPrefetch && b.short(h.resource, h.cost, now):
		why, until = "reserve", b.local(b.quotas[h.resource].reset)
	default:
		return b.reserve(h.resource, h.route, h.cost, now), until, nil
	}
	if why != "" {
		if on && !l.secondary {
			b.logReached(ctx, h.resource, until)
		}
		return nil, until, b.refuse(h.resource, why, until, now)
	}
	b.enqueue(h, now)
	if !l.secondary {
		b.logReached(ctx, h.resource, until)
	}
	b.pump()
	return nil, until, nil
}

// cost returns what a request of resource costs: a GraphQL query what a
// query of its shape cost last time, any other request 1, and one that
// counts against no resource nothing. A mutation costs 1 of the primary
// limit. b.mu must be held.
func (b *budget) cost(resource string, c *call) int {
	switch {
	case resource == "":
		return 0
	case resource == resourceGraphQL && c != nil && c.query:
		return max(b.opCost[c.shape], 1)
	}
	return 1
}

// limitOn reports whether a rate limit stops a request of resource that
// costs cost, and until when: a secondary limit, which stops every
// request, or else the limit of its quota (spent). b.mu must be held.
func (b *budget) limitOn(resource string, cost int, now time.Time) (limit, bool) {
	g := &b.gate
	switch {
	case now.Before(g.secondary):
		return limit{until: g.secondary, secondary: true}, true
	case g.lifting:
		return limit{secondary: true, inFlight: true}, true
	}
	return b.spent(resource, cost, now)
}

// spent reports whether the quota of resource stops a request that costs
// cost, and until when: the release of the quota, if it is spent, or
// else, if what is left of it once the requests in flight are answered
// doesn't cover cost, the end of its window and a guard. An until that
// passed is a limit that may have lifted, which no answer showed yet.
// b.mu must be held.
func (b *budget) spent(resource string, cost int, now time.Time) (limit, bool) {
	q := b.quotas[resource]
	switch {
	case q == nil:
	case !q.release.IsZero():
		return limit{until: q.release}, true
	case b.est(resource, q) < cost && !b.far(q.reset, now):
		return limit{until: b.local(q.reset).Add(q.guard), inFlight: q.remaining >= cost}, true
	}
	return limit{}, false
}

// short reports whether a read ahead of resource that costs cost would
// take of what is kept of its quota for what the user asks for. b.mu must
// be held.
func (b *budget) short(resource string, cost int, now time.Time) bool {
	q := b.quotas[resource]
	return q != nil && !b.far(q.reset, now) && b.est(resource, q)-cost < q.limit*prefetchReserve/100
}

// refuse returns the error of a request of resource that fails at once
// for why, since its limit lifts at until, and counts it. b.mu must be
// held.
func (b *budget) refuse(resource, why string, until, now time.Time) error {
	obs.CountRateFailed(resource, why)
	return &core.RateLimitError{Reset: later(until, now)}
}

// logReached logs that the limit of resource, which lifts at until, stopped
// a request, once per window of its quota, once b.mu is released. b.mu
// must be held.
func (b *budget) logReached(ctx context.Context, resource string, until time.Time) {
	q := b.quotas[resource]
	if q == nil || b.gate.reached[resource].Equal(q.reset) {
		return
	}
	b.gate.reached[resource] = q.reset
	limit, reset, held := q.limit, b.local(q.reset), b.held(resource)
	b.log(func() {
		slog.InfoContext(ctx, "rate limit reached", "span", "http", "resource", resource,
			"limit", limit, "reset", reset, "release", until, "held", held)
	})
}

// enqueue puts h in the queue of its resource. b.mu must be held.
func (b *budget) enqueue(h *hold, now time.Time) {
	g := &b.gate
	q := g.queues[h.resource]
	if q == nil {
		q = &queue{resource: h.resource, since: now}
		g.queues[h.resource] = q
	}
	g.seq++
	h.seq = g.seq
	h.ready = make(chan struct{})
	i, _ := slices.BinarySearchFunc(q.held, h, byTurn)
	q.held = slices.Insert(q.held, i, h)
	obs.CountRate(h.resource, obs.RateHeld)
}

// wait waits until h is let go and then until its turn to be sent comes,
// or until it fails or ctx ends. It returns the reservation to send it
// with, and how long it was held.
func (b *budget) wait(ctx context.Context, h *hold) (*reservation, time.Duration, error) {
	select {
	case <-h.ready:
	case <-ctx.Done():
		if r := b.drop(h); r != nil {
			b.forget(r, false)
		}
		return nil, 0, ctx.Err()
	}
	if h.err != nil {
		return nil, 0, h.err
	}
	if err := sleep(ctx, h.sendAt.Sub(b.now())); err != nil {
		b.forget(h.r, false)
		return nil, 0, err
	}
	return h.r, h.sendAt.Sub(h.at), nil
}

// drop takes h out of its queue, if it is still held, and counts it as
// dropped, since its context ended. It returns the reservation of h if it
// was let go already.
func (b *budget) drop(h *hold) *reservation {
	defer b.changed()
	b.mu.Lock()
	defer b.unlock()
	if h.done {
		return h.r
	}
	q := b.gate.queues[h.resource]
	q.remove(h)
	h.done = true
	close(h.ready)
	q.dropped++
	obs.CountRate(h.resource, obs.RateDropped)
	b.pump()
	return nil
}

// fail fails h, taken out of q, for why, since its limit lifts at until.
// b.mu must be held.
func (b *budget) fail(q *queue, h *hold, why string, until, now time.Time) {
	h.err = b.refuse(h.resource, why, until, now)
	h.done = true
	close(h.ready)
	q.failed++
}

// letGo lets h go, taken out of q: it counts h against its resource, to
// be sent a stagger after the request let go before it that it follows
// (gate.next), unless that is too close to its deadline. b.mu must be
// held.
func (b *budget) letGo(q *queue, h *hold, now time.Time) {
	q.remove(h)
	g := &b.gate
	after := later(g.next, now)
	if h.class == classBackground {
		after = later(after, g.nextBackground)
	}
	at := after.Add(minStagger + time.Duration(g.jitter()*float64(maxStagger-minStagger)))
	if !h.deadline.IsZero() && h.deadline.Before(at.Add(deadlineMargin)) {
		b.fail(q, h, "deadline", at, now)
		return
	}
	if h.class == classBackground {
		g.nextBackground = at
	} else {
		g.next = at
	}
	h.r = b.reserve(h.resource, h.route, h.cost, now)
	h.sendAt = at
	h.done = true
	close(h.ready)
	q.released++
	obs.CountRateReleased(h.resource, at.Sub(h.at))
}

// pump moves what is held along as the limits now allow: it lets go what
// may be sent, fails what can't wait for its limit to lift, and sets the
// timers for the rest. Where a limit may have lifted, a probe of the
// limits, or else the first request held, finds out first. b.mu must be
// held.
func (b *budget) pump() {
	g := &b.gate
	if len(g.queues) == 0 && g.secondary.IsZero() && !g.lifting {
		return
	}
	now := b.now()
	if !g.secondary.IsZero() && !now.Before(g.secondary) {
		// No probe shows a secondary limit, so the first request held
		// goes first, and the others wait for its answer.
		g.secondary, g.lifting = time.Time{}, len(g.queues) > 0
	}
	if g.lifting && g.scout == 0 && !b.sendScout(now) {
		g.lifting = false
	}
	if g.secondary.IsZero() && !g.lifting {
		b.letGoInTurn(now)
		for _, q := range g.queues {
			b.pumpQueue(q, now)
		}
		b.tidy(now)
		return
	}
	// Until the secondary limit lifts, or the scout is answered, what is
	// held waits as long as its class may, and the timer fails it then.
	l := limit{until: g.secondary, secondary: true, inFlight: g.secondary.IsZero()}
	var at time.Time
	for _, q := range g.queues {
		b.expire(q, l, now)
		if end := l.expiry(q.held); !end.IsZero() && (at.IsZero() || end.Before(at)) {
			at = end
		}
	}
	b.tidy(now)
	if !at.IsZero() {
		g.timer = b.arm(g.timer, at.Sub(now))
	}
}

// sendScout lets go the first request held whose quota isn't spent, as the
// first sent once a secondary limit lifted, and reports whether there was
// one. b.mu must be held.
func (b *budget) sendScout(now time.Time) bool {
	for {
		var first *hold
		var from *queue
		for _, q := range b.gate.queues {
			for _, h := range q.held {
				if _, on := b.spent(q.resource, h.cost, now); on {
					continue
				}
				if first == nil || byTurn(h, first) < 0 {
					first, from = h, q
				}
				break
			}
		}
		if first == nil {
			return false
		}
		b.letGo(from, first, now)
		if first.r != nil {
			b.gate.scout = first.r.seq
			return true
		}
	}
}

// letGoInTurn lets go each request held whose quota covers it, of every
// resource, by class and then as they came. b.mu must be held.
func (b *budget) letGoInTurn(now time.Time) {
	var held []*hold
	for _, q := range b.gate.queues {
		held = append(held, q.held...)
	}
	slices.SortFunc(held, byTurn)
	for _, h := range held {
		if _, on := b.spent(h.resource, h.cost, now); !on {
			b.letGo(b.gate.queues[h.resource], h, now)
		}
	}
}

// pumpQueue fails the requests held in q that can't wait for the limit of
// its quota, and sets its timer for the rest, or, if the limit may have
// lifted, has a probe or the first of them find out. b.mu must be held.
func (b *budget) pumpQueue(q *queue, now time.Time) {
	if len(q.held) == 0 {
		q.noProbe = false
		return
	}
	l, _ := b.spent(q.resource, q.held[0].cost, now)
	if l.until.After(now) {
		b.expire(q, l, now)
		if len(q.held) > 0 {
			q.timer = b.arm(q.timer, l.expiry(q.held).Sub(now))
		}
		return
	}
	// The limit may have lifted, which a probe, or else the first request
	// held, finds out, and the others wait for its answer as long as
	// their class may.
	l = limit{inFlight: true}
	b.expire(q, l, now)
	g := &b.gate
	switch {
	case len(q.held) == 0, q.scout != 0, g.probing:
	case g.probe != nil && !q.noProbe:
		g.probing = true
		go b.runProbe()
	default:
		for len(q.held) > 0 && q.scout == 0 {
			h := q.held[0]
			b.letGo(q, h, now)
			if h.r != nil {
				q.scout = h.r.seq
			}
		}
	}
	if len(q.held) > 0 {
		q.timer = b.arm(q.timer, l.expiry(q.held).Sub(now))
	}
}

// expire fails the requests held in q that can't wait for l, their
// limit, to lift. b.mu must be held.
func (b *budget) expire(q *queue, l limit, now time.Time) {
	q.held = slices.DeleteFunc(q.held, func(h *hold) bool {
		why := h.tooLate(l, now)
		if why != "" {
			b.fail(q, h, why, l.until, now)
		}
		return why != ""
	})
}

// tidy drops the queues that emptied, and logs of each that let requests
// go that its limit lifted, if it did, once b.mu is released. A queue whose scout wasn't
// answered yet stays. b.mu must be held.
func (b *budget) tidy(now time.Time) {
	for resource, q := range b.gate.queues {
		if len(q.held) > 0 || q.scout != 0 {
			continue
		}
		if q.timer != nil {
			q.timer.Stop()
		}
		delete(b.gate.queues, resource)
		if _, on := b.spent(resource, 1, now); q.released > 0 && !on {
			released, dropped, failed, waited := q.released, q.dropped, q.failed, now.Sub(q.since)
			b.log(func() {
				slog.Info("rate limit lifted", "span", "http", "resource", resource, "released", released,
					"dropped", dropped, "failed", failed, "waited_ms", obs.Millis(waited))
			})
		}
	}
}

// arm sets t, a timer that pumps, or a new one if it is nil, to fire after
// d, and returns it, unless the client is closed. b.mu must be held.
func (b *budget) arm(t *time.Timer, d time.Duration) *time.Timer {
	switch {
	case b.gate.closed:
		return t
	case t == nil:
		return time.AfterFunc(d, b.tick)
	}
	t.Reset(d)
	return t
}

// close stops the timers of the gate for good, as the client is closed.
func (b *budget) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	g := &b.gate
	g.closed = true
	if g.timer != nil {
		g.timer.Stop()
	}
	for _, q := range g.queues {
		if q.timer != nil {
			q.timer.Stop()
		}
	}
}

// tick moves what is held along, when a limit is due to lift.
func (b *budget) tick() {
	defer b.changed()
	b.mu.Lock()
	defer b.unlock()
	b.pump()
}

// settle lets the requests held for the answer to r go on, if r was a
// scout, now that it was answered, or has another request find out if it
// got no answer, and moves what is held along. b.mu must be held.
func (b *budget) settle(r *reservation, answered bool) {
	g := &b.gate
	if g.scout == r.seq {
		g.scout = 0
		g.lifting = g.lifting && !answered
	}
	if q := g.queues[r.resource]; q != nil && q.scout == r.seq {
		q.scout = 0
	}
	b.pump()
}

// limitSecondary records a secondary limit, which holds every request
// until at, or, if the answer didn't say, for secondaryBackoff, doubling
// each time one comes again before a request succeeds, up to
// maxSecondaryBackoff, as GitHub advises. It returns when the limit lifts.
func (b *budget) limitSecondary(ctx context.Context, at time.Time, said bool) time.Time {
	defer b.changed()
	b.mu.Lock()
	defer b.unlock()
	now := b.now()
	g := &b.gate
	if !said {
		wait := cmp.Or(g.backoff, secondaryBackoff)
		at = now.Add(wait)
		g.backoff = min(2*wait, maxSecondaryBackoff)
	}
	if !at.After(g.secondary) {
		return g.secondary
	}
	if !now.Before(g.secondary) {
		held := 0
		for _, q := range g.queues {
			held += len(q.held)
		}
		b.log(func() {
			slog.InfoContext(ctx, "rate limit reached", "span", "http", "secondary", true,
				"release", at, "held", held)
		})
	}
	g.secondary = at
	b.pump()
	return at
}

// succeeded notes that a request succeeded, so that the next secondary
// limit that doesn't say how long it lasts lasts secondaryBackoff again.
func (b *budget) succeeded() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gate.backoff = 0
}

// liftsAt returns when a secondary limit that says it lifts at at lifts,
// as far as the budget knows: at, or later if an answer since said so.
func (b *budget) liftsAt(at time.Time) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return later(at, b.gate.secondary)
}

// secondaryUntil returns when the secondary limit lifts, which holds every
// request until then, or zero if none is on.
func (b *budget) secondaryUntil() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.now().Before(b.gate.secondary) {
		return time.Time{}
	}
	return b.gate.secondary
}

// runProbe asks GitHub for the rate limits, and has the budget learn what
// they say: a limit whose window they show refilled lifts, and one they
// show in the window that should have ended lasts a guard twice as long.
// Where the probe can't tell, the first request held finds out instead.
func (b *budget) runProbe() {
	ctx, cancel := context.WithTimeout(obs.ForBackground(context.Background()), probeTimeout)
	defer cancel()
	obs.CountRateProbe()
	limits, err := b.gate.probe(ctx)
	if err != nil {
		slog.InfoContext(ctx, "rate limit probe failed", "span", "http", "err", err.Error())
	}
	defer b.changed()
	b.mu.Lock()
	defer b.unlock()
	b.gate.probing = false
	now := b.now()
	b.seq++
	for resource, rl := range limits {
		// A resource that no answer reported isn't one the client uses.
		if b.quotas[resource] == nil {
			continue
		}
		if guard := b.report(resource, rl, b.seq, now, now); guard > 0 {
			b.log(func() { outlasted(ctx, resource, guard) })
		}
	}
	for _, q := range b.gate.queues {
		if _, ok := limits[q.resource]; !ok {
			q.noProbe = true
			continue
		}
		if len(q.held) == 0 {
			continue
		}
		if l, on := b.spent(q.resource, q.held[0].cost, now); on && !l.until.After(now) {
			quota := b.quotas[q.resource]
			quota.guard = min(2*quota.guard, maxGuard)
			quota.release = now.Add(quota.guard)
			resource, guard := q.resource, quota.guard
			b.log(func() { outlasted(ctx, resource, guard) })
		}
	}
	b.pump()
}

// outlasted logs and counts that the limit of resource was still on after
// its guard, which is now guard.
func outlasted(ctx context.Context, resource string, guard time.Duration) {
	obs.CountRateEarly()
	slog.WarnContext(ctx, "rate limit outlasted its reset", "span", "http", "resource", resource,
		"guard_ms", obs.Millis(guard))
}

// held returns how many requests of resource are held for its limit.
// b.mu must be held.
func (b *budget) held(resource string) int {
	if q := b.gate.queues[resource]; q != nil {
		return len(q.held)
	}
	return 0
}

// later returns the later of a and b.
func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// rateTransport is the gate that each attempt at a request passes: it
// holds back or fails an attempt that its rate limit would refuse
// (budget.admit), counts the others against their limits before they are
// sent, and has the budget learn from their answers. It sits below retry,
// so that each attempt passes it and the attempts retry discards are
// counted too, and above the timeout and the limit, so that an attempt
// held takes neither its time nor a slot, and one waiting for a slot is
// counted already.
type rateTransport struct {
	base   http.RoundTripper
	budget *budget
}

// RoundTrip sends req once its limit lets it, and observes its answer.
func (t *rateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r, held, err := t.budget.admit(req)
	if err != nil {
		// A RoundTripper closes the body of the request, even on error.
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	if r == nil {
		return t.base.RoundTrip(req)
	}
	if held > 0 {
		req = req.WithContext(withHeld(req.Context(), held))
	}
	// A reservation left behind by a panic would take from the
	// estimate for good.
	settled := false
	defer func() {
		if !settled {
			t.budget.forget(r, false)
		}
	}()
	resp, err := t.base.RoundTrip(req)
	settled = true
	if err != nil {
		t.budget.forget(r, req.Context().Err() == nil)
		return nil, err
	}
	resp = t.noteSecondary(req, resp)
	if guard := t.budget.observe(r, resp.Header); guard > 0 {
		outlasted(req.Context(), r.resource, guard)
	}
	return resp, nil
}

// noteSecondary records the secondary limit that resp is, if it is one,
// before resp settles its request, which may be the scout the others
// wait for: a 403 or 429 of a quota that isn't spent, which says when to
// try again, or is a 429, or says so in its message. A success starts the
// backoff of secondary limits over, and the client tells of a GraphQL
// query's (budget.succeeded). It returns resp, whose body reads as
// it came.
func (t *rateTransport) noteSecondary(req *http.Request, resp *http.Response) *http.Response {
	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusTooManyRequests:
	default:
		// A GraphQL answer may be a secondary limit in its body, so
		// the client says whether a query succeeded once it read it.
		if resp.StatusCode < http.StatusBadRequest && req.URL.Path != t.budget.graphqlPath {
			t.budget.succeeded()
		}
		return resp
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return resp
	}
	at, said := t.budget.retryAfter(resp.Header)
	if !said && resp.StatusCode == http.StatusForbidden {
		var secondary bool
		if resp, secondary = peekSecondary(resp); !secondary {
			return resp
		}
	}
	t.budget.limitSecondary(req.Context(), at, said)
	return resp
}

// maxErrorPeek is the most of the body of a 403 read to learn whether it
// is a secondary limit. GitHub's error bodies are far smaller.
const maxErrorPeek = 64 << 10

// peekSecondary reads the body of resp, a 403, and reports whether its
// message is GitHub's for a secondary limit. resp reads the same body as
// it came.
func peekSecondary(resp *http.Response) (*http.Response, bool) {
	if resp.Body == nil {
		return resp, false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorPeek))
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(b), resp.Body), resp.Body}
	var body apiError
	if err != nil || json.Unmarshal(b, &body) != nil {
		return resp, false
	}
	return resp, secondaryLimit(body.Message)
}
