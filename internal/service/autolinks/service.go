// Package autolinks verifies that what a text names, such as #123, @login or
// a commit hash, exists, so that it can be linked. It asks GitHub with one
// conditional request each, which costs nothing against the rate limit when
// the answer is unchanged, and keeps the answers, misses too.
package autolinks

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// API is the part of the GitHub client the service uses.
type API interface {
	VerifyIssue(ctx context.Context, repo core.RepoRef, number int, cond github.Conditional) (core.Verified, github.Response, error)
	VerifyUser(ctx context.Context, login string, cond github.Conditional) (core.Verified, github.Response, error)
	VerifyCommit(ctx context.Context, repo core.RepoRef, sha string, cond github.Conditional) (core.Verified, github.Response, error)
}

// Candidate is something a text may name. Kind is VerifiedIssue for a
// number, which may turn out to be a pull request, VerifiedUser for a login
// and VerifiedCommit for a hash of 7 to 40 hex digits.
type Candidate struct {
	Kind core.VerifiedKind
	// Repo is "owner/name", or empty for the repository the call names.
	Repo string
	// Ref is the number, the login or the hash.
	Ref string
}

// Result is what is known of a candidate.
type Result struct {
	Candidate Candidate
	core.Verified
	// Known is false if nothing is known yet: Cached found nothing, or the
	// read failed, which Err says.
	Known bool
	Err   error
}

// Service verifies candidates. It is safe for concurrent use.
type Service struct {
	api     API
	host    string
	publish func(key string)
	mem     *cache.Cache[core.Verified]
	ttl     time.Duration
}

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl      time.Duration
	capacity int
	host     string
	publish  func(string)
}

// WithTTL sets how long an answer stays fresh; a miss stays fresh for a
// tenth of it. Without it the default of the config applies.
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithCapacity sets how many answers the service keeps.
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithHost sets the host of the session, which keys the answers.
func WithHost(host string) Option { return func(o *options) { o.host = host } }

// WithPublish sets the function that Verify calls with SyncKey after a
// batch that learned something, so that views read the answers again.
func WithPublish(f func(key string)) Option { return func(o *options) { o.publish = f } }

// New returns a Service that reads through api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	o.ttl = cmp.Or(o.ttl, d.Cache.TTL.Autolinks)
	o.capacity = cmp.Or(o.capacity, d.Cache.Memory.Entries)
	return &Service{
		api: api, host: cmp.Or(o.host, core.DefaultHost), publish: o.publish, ttl: o.ttl,
		mem: cache.New[core.Verified](cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)),
	}
}

// SyncKey is the key Verify publishes after a batch that learned something.
func SyncKey(host string) string { return "autolinks:" + host }

// missFraction is how much of the TTL a miss stays fresh: a thing that
// doesn't exist yet may soon do.
const missFraction = 10

// verifyWorkers is how many candidates are verified at once.
const verifyWorkers = 4

func (s *Service) key(repo string, c Candidate) (string, bool) {
	r := cmp.Or(c.Repo, repo)
	switch c.Kind {
	case core.VerifiedUser:
		return "autolink:" + s.host + ":@" + strings.ToLower(c.Ref), c.Ref != ""
	case core.VerifiedIssue, core.VerifiedCommit:
		ref := c.Ref
		if c.Kind == core.VerifiedCommit {
			ref = strings.ToLower(ref)
		} else if _, err := strconv.Atoi(ref); err != nil {
			return "", false
		}
		_, ok := splitRepo(r)
		return "autolink:" + s.host + ":" + string(c.Kind) + ":" + strings.ToLower(r) + ":" + ref, ok && ref != ""
	case core.VerifiedPull, core.VerifiedOrg:
		// Only what a text names is asked: a number is an issue until
		// GitHub says it is a pull request.
	}
	return "", false
}

func splitRepo(s string) (core.RepoRef, bool) {
	owner, name, ok := strings.Cut(s, "/")
	return core.RepoRef{Owner: owner, Name: name}, ok && owner != "" && name != "" && !strings.Contains(name, "/")
}

// Cached returns what is cached of each candidate, fresh or stale, without
// I/O. The results align with cands.
func (s *Service) Cached(repo string, cands []Candidate) []Result {
	out := make([]Result, len(cands))
	for i, c := range cands {
		out[i].Candidate = c
		if k, ok := s.key(repo, c); ok {
			if e, st := s.mem.Get(k); st != cache.Miss {
				out[i].Verified, out[i].Known = e.Value, true
			}
		}
	}
	return out
}

// Verify asks GitHub about each distinct candidate that isn't fresh in the
// cache, at most verifyWorkers at once, and returns a result for each of
// cands. Repo is the repository of the candidates that name none. A thing
// that doesn't exist is a result with Exists unset, not an error. If any
// answer was learned, Verify publishes SyncKey.
func (s *Service) Verify(ctx context.Context, repo string, cands []Candidate) []Result {
	type job struct {
		key string
		c   Candidate
	}
	seen := map[string]bool{}
	var jobs []job
	for _, c := range cands {
		if k, ok := s.key(repo, c); ok && !seen[k] {
			seen[k] = true
			jobs = append(jobs, job{k, c})
		}
	}
	errs := make(map[string]error, len(jobs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, verifyWorkers)
	learned := false
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			stored := false
			_, err := s.mem.Fetch(ctx, j.key, func(ctx context.Context, prev cache.Entry[core.Verified], ok bool) (cache.Entry[core.Verified], error) {
				e, err := s.load(ctx, repo, j.c, prev, ok)
				// A 304 comes back as an error here, and learns nothing, so
				// it publishes nothing.
				stored = err == nil
				return e, err
			})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[j.key] = err
			} else if stored {
				learned = true
			}
		}()
	}
	wg.Wait()
	if learned && s.publish != nil && ctx.Err() == nil {
		s.publish(SyncKey(s.host))
	}
	out := s.Cached(repo, cands)
	for i := range out {
		if k, ok := s.key(repo, out[i].Candidate); ok {
			out[i].Err = errs[k]
		}
	}
	if err := ctx.Err(); err != nil {
		slog.DebugContext(ctx, "autolinks", "span", "service.autolinks", "err", err)
	}
	return out
}

// load asks GitHub, conditionally if prev has an ETag.
func (s *Service) load(ctx context.Context, repo string, c Candidate, prev cache.Entry[core.Verified], ok bool) (cache.Entry[core.Verified], error) {
	var cond github.Conditional
	if ok && prev.Value.Exists {
		cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
	}
	r, _ := splitRepo(cmp.Or(c.Repo, repo))
	var (
		v   core.Verified
		res github.Response
		err error
	)
	switch c.Kind {
	case core.VerifiedUser:
		v, res, err = s.api.VerifyUser(ctx, c.Ref, cond)
	case core.VerifiedCommit:
		v, res, err = s.api.VerifyCommit(ctx, r, c.Ref, cond)
	default:
		n, convErr := strconv.Atoi(c.Ref)
		if convErr != nil {
			return cache.Entry[core.Verified]{}, errors.New("verify issue: not a number: " + c.Ref)
		}
		v, res, err = s.api.VerifyIssue(ctx, r, n, cond)
	}
	if err != nil {
		return cache.Entry[core.Verified]{}, err
	}
	if res.NotModified {
		return cache.Entry[core.Verified]{}, cache.ErrNotModified
	}
	e := cache.Entry[core.Verified]{Value: v, ETag: res.ETag, LastModified: res.LastModified, FetchedAt: time.Now()}
	if !v.Exists {
		// Backdated, so a miss goes stale sooner than a hit.
		e.FetchedAt = e.FetchedAt.Add(-s.ttl + s.ttl/missFraction)
	}
	return e, nil
}
