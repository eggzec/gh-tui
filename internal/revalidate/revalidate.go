// Package revalidate re-checks cached entries in the background, each with
// one conditional request, so that what a view reads is already known to be
// current and costs no request at load time.
//
// It knows nothing about what an entry holds or how to check it: the
// services that own the entries list them, each with the function that
// checks it, and report what the check found. The revalidator decides which
// entries to check and when: those of the selected repository first, then
// the ones used most recently, within a budget of requests per minute, a
// few at a time, and not at all while GitHub can't be reached or the rate
// limit is used up.
package revalidate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// ErrRan is returned by Run when the revalidator has already been run.
var ErrRan = errors.New("revalidate: already ran")

// Status is what checking an entry found.
type Status int

// Statuses of a check.
const (
	// Skipped means no request was needed: the entry is fresh, has no
	// validators, is being fetched already, or is gone.
	Skipped Status = iota
	// NotModified means GitHub answered 304: the entry is current, and
	// now counts as fetched just now.
	NotModified
	// Changed means the check learned that the entry changed: GitHub sent
	// a new value, which is now cached, or a source left the new value to
	// the views, which read it again on seeing Result.Sync.
	Changed
	// Gone means GitHub refused the request, and the entry was forgotten.
	Gone
	// Offline means GitHub couldn't be reached. The pass stops.
	Offline
	// Limited means a rate limit refused the request. The pass stops,
	// and no other starts before Result.RetryAt.
	Limited
	// Failed means the check failed otherwise, such as with an
	// unexpected response.
	Failed
)

// String returns the status's name.
func (s Status) String() string {
	switch s {
	case Skipped:
		return "skipped"
	case NotModified:
		return "not modified"
	case Changed:
		return "changed"
	case Gone:
		return "gone"
	case Offline:
		return "offline"
	case Limited:
		return "limited"
	case Failed:
		return "failed"
	default:
		return "unknown"
	}
}

// Result is the outcome of checking an entry.
type Result struct {
	Status Status
	// Sync is the sync key of what a Changed entry belongs to, such as
	// the issues of its repository, which is published so that the views
	// showing it read it again. Empty publishes nothing.
	Sync string
	// RetryAt is when a Limited check may be tried again.
	RetryAt time.Time
	// Err is the error of an Offline, Limited, Gone or Failed check.
	Err error
}

// Entry is a cached entry that one conditional request can check.
type Entry struct {
	// ID names the entry among every source's entries.
	ID string
	// Repo is the repository the entry belongs to, or the zero RepoRef
	// for one that belongs to none, such as the inbox.
	Repo core.RepoRef
	// UsedAt is when the entry was last used, and CheckedAt when it was
	// last fetched or found current.
	UsedAt    time.Time
	CheckedAt time.Time
	// FreshFor is how long after it was fetched or found current the
	// entry is left alone, such as the TTL of its cache, since a read
	// wouldn't ask GitHub about it either. A source must set it: zero
	// leaves the entry due on every pass.
	FreshFor time.Duration
	// Check sends the conditional request and stores what it brings. It
	// is called in the revalidator's goroutines, a few at once.
	Check func(ctx context.Context) Result
}

// Source lists the entries that a service can check. It is called at the
// start of each pass, in the revalidator's goroutine, so it may read the
// disk.
type Source func() []Entry

// Revalidator checks the entries of its sources in passes. Its methods are
// safe for concurrent use.
type Revalidator struct {
	sources []Source
	cfg     config
	budget  window
	kick    chan struct{}
	// online starts a pass at once after passes found GitHub unreachable.
	online chan struct{}

	mu     sync.Mutex
	repo   core.RepoRef
	active bool
	ran    bool
	// checked holds until when each entry this revalidator found current
	// stays fresh, by ID, in case its source can't tell yet.
	checked map[string]time.Time
	// pending holds the sync keys of changes not yet published.
	pending map[string]bool
}

// New returns an active revalidator of the entries of sources, as s says.
// It checks nothing until Run is called.
func New(sources []Source, s Settings, opts ...Option) *Revalidator {
	s.Interval = max(s.Interval, time.Second)
	s.PerMinute = max(1, s.PerMinute)
	cfg := config{
		Settings:    s,
		concurrency: DefaultConcurrency,
		idle:        1,
		startDelay:  DefaultStartDelay,
		maxBackoff:  DefaultMaxBackoff,
		publish:     func(string) {},
		report:      func(Pass) {},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Revalidator{
		sources: sources,
		cfg:     cfg,
		kick:    make(chan struct{}, 1),
		online:  make(chan struct{}, 1),
		active:  true,
		checked: make(map[string]time.Time),
		pending: make(map[string]bool),
	}
}

// SetRepo makes repo the selected repository, whose entries are checked
// first. Selecting another one starts a pass soon, unless GitHub can't be
// reached or the rate limit is used up.
func (r *Revalidator) SetRepo(repo core.RepoRef) {
	r.mu.Lock()
	same := sameRepo(r.repo, repo)
	r.repo = repo
	r.mu.Unlock()
	if same {
		return
	}
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// Online tells the revalidator that GitHub answers again after it couldn't
// be reached. If the last pass found it unreachable, and so the next one
// backed off, the next one starts at once; otherwise nothing changes.
func (r *Revalidator) Online() {
	select {
	case r.online <- struct{}{}:
	default:
	}
}

// SetActive tells the revalidator whether the user is looking. While
// inactive, such as when the terminal loses focus, passes are further apart
// and the budget smaller, both by the idle multiplier.
func (r *Revalidator) SetActive(active bool) {
	r.mu.Lock()
	r.active = active
	r.mu.Unlock()
	if active {
		r.budget.wake()
	}
}

// Run checks entries in passes until ctx is done, then returns ctx.Err().
// The first pass starts after the start delay, and each one after the
// previous ended: an interval later, longer while inactive, doubled after
// each pass that found GitHub unreachable, and not before a rate limit
// resets, or at once when GitHub answers again after an unreachable pass
// (Online). A revalidator can be run only once.
func (r *Revalidator) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.ran {
		r.mu.Unlock()
		return ErrRan
	}
	r.ran = true
	r.mu.Unlock()

	var (
		offline int
		retryAt time.Time
	)
	timer := time.NewTimer(r.cfg.startDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		case <-r.kick:
			// A new repository doesn't make GitHub reachable, or lift a
			// rate limit.
			if offline > 0 || time.Now().Before(retryAt) {
				continue
			}
			timer.Stop()
		case <-r.online:
			if offline == 0 || time.Now().Before(retryAt) {
				continue
			}
			timer.Stop()
		}
		pctx := obs.ForBackground(obs.WithTrace(ctx, "revalidate.pass"))
		p := r.pass(pctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r.cfg.report(p)
		if p.Offline {
			offline++
		} else {
			offline = 0
		}
		retryAt = p.RetryAt
		next := max(r.delay(offline), time.Until(retryAt))
		obs.CountRevalidate(p.Sent, p.Budget)
		p.log(pctx, next)
		timer.Reset(next)
	}
}

// delay returns how long to wait for the next pass after offline passes in
// a row that found GitHub unreachable.
func (r *Revalidator) delay(offline int) time.Duration {
	d := r.cfg.Interval
	limit := max(r.cfg.maxBackoff, d)
	for range offline {
		if d >= limit {
			break
		}
		d *= 2
	}
	d = min(d, limit)
	if !r.isActive() {
		d *= time.Duration(r.cfg.idle)
	}
	return d
}

// limit returns how many requests a minute may bring now.
func (r *Revalidator) limit() int {
	if r.isActive() {
		return r.cfg.PerMinute
	}
	return max(1, r.cfg.PerMinute/r.cfg.idle)
}

func (r *Revalidator) isActive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

func (r *Revalidator) selected() core.RepoRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.repo
}

// sameRepo reports whether a and b name one repository. GitHub ignores
// case in owner and repository names.
func sameRepo(a, b core.RepoRef) bool {
	return strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Name, b.Name)
}
