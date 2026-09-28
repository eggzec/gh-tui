// Package watch polls subscribed keys on a schedule and publishes an event
// when one of them changes. It knows nothing about what a key means: callers
// supply the poll function and decide what to refetch when an event arrives.
package watch

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRan is returned by Run when the engine has already been run.
var ErrRan = errors.New("watch: engine already ran")

// PollFunc checks a key for changes. It should be cheap, for example a
// conditional request that the server answers with 304 Not Modified.
type PollFunc func(ctx context.Context) (Result, error)

// Result is the outcome of a successful poll.
type Result struct {
	// Changed reports whether the data behind the key changed.
	Changed bool
	// Interval is the server's requested polling interval, such as
	// X-Poll-Interval. Zero means use the engine default.
	Interval time.Duration
}

// Engine schedules polls for subscribed keys. Its methods are safe for
// concurrent use.
type Engine struct {
	cfg    config
	events chan Event
	notify chan struct{}

	mu      sync.Mutex
	pollers map[string]*poller
	active  bool
	ctx     context.Context // set while Run is running
	ran     bool
	stopped bool
	wg      sync.WaitGroup
	queue   []string // keys with a pending event, oldest first
	pending map[string]*pending
	seq     uint64
}

// New returns an active engine. It polls nothing until keys are subscribed
// and Run is called.
func New(opts ...Option) *Engine {
	cfg := config{
		interval:    DefaultInterval,
		minInterval: DefaultMinInterval,
		maxBackoff:  DefaultMaxBackoff,
		idle:        DefaultIdleMultiplier,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Engine{
		cfg:     cfg,
		events:  make(chan Event),
		notify:  make(chan struct{}, 1),
		pollers: make(map[string]*poller),
		active:  true,
		pending: make(map[string]*pending),
	}
}

// Subscribe starts polling key with fn and returns a function that ends the
// subscription. Subscribers to the same key share one poller, which uses the
// fn of the first subscriber and stops when the last one unsubscribes. The
// first poll happens one interval after the poller starts; call Refresh to
// poll at once. Calling the returned function more than once has no effect.
func (e *Engine) Subscribe(key string, fn PollFunc) (unsubscribe func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.pollers[key]
	if !ok {
		p = &poller{key: key, fn: fn, wake: make(chan struct{}, 1)}
		e.pollers[key] = p
		e.start(p)
	}
	p.refs++
	return sync.OnceFunc(func() { e.release(p) })
}

func (e *Engine) release(p *poller) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p.refs--
	if p.refs > 0 {
		return
	}
	delete(e.pollers, p.key)
	e.drop(p.key)
	if p.cancel != nil {
		p.cancel()
	}
}

// start launches the poller's goroutine if Run is running. The caller holds
// e.mu.
func (e *Engine) start(p *poller) {
	if e.ctx == nil || e.stopped {
		return
	}
	ctx, cancel := context.WithCancel(e.ctx)
	p.cancel = cancel
	e.wg.Go(func() { e.poll(ctx, p) })
}

// Refresh polls key at once. It does nothing if key has no subscribers.
func (e *Engine) Refresh(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p, ok := e.pollers[key]; ok {
		p.kick()
	}
}

// SetActive tells the engine whether the user is looking. While inactive, for
// example when the terminal loses focus, intervals are multiplied by the idle
// multiplier. Becoming active again polls every key at once, so the screen
// catches up with what changed in the meantime.
func (e *Engine) SetActive(active bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active == active {
		return
	}
	e.active = active
	if !active {
		return
	}
	for _, p := range e.pollers {
		p.kick()
	}
}

// SetInterval sets the interval used when a poll returns no server hint,
// as WithInterval does, from the next poll of each key on: a poll already
// scheduled keeps its time, so that the server's hints hold. Values <= 0
// are ignored.
func (e *Engine) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg.interval = d
}

// Events returns the channel that change events are delivered on. It is
// closed when Run returns.
func (e *Engine) Events() <-chan Event {
	return e.events
}

// Run polls subscribed keys until ctx is done, then waits for every poller to
// exit, closes the events channel and returns ctx.Err(). An engine can be run
// only once.
func (e *Engine) Run(ctx context.Context) error {
	e.mu.Lock()
	if e.ran {
		e.mu.Unlock()
		return ErrRan
	}
	e.ran = true
	e.ctx = ctx
	for _, p := range e.pollers {
		e.start(p)
	}
	e.mu.Unlock()

	e.dispatch(ctx)

	e.mu.Lock()
	e.stopped = true
	e.mu.Unlock()
	e.wg.Wait()
	close(e.events)
	return ctx.Err()
}
