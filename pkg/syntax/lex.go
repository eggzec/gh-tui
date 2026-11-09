package syntax

import (
	"context"
	"errors"
	"fmt"
	"hash/maphash"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alecthomas/chroma/v2"
)

// Limit is how long a lexer may take on some code, or on one token of it,
// before it is taken to never finish on that code. It isn't run on it
// again.
const Limit = 50 * time.Millisecond

// lexing holds a token while a lexer runs. Chroma can't be stopped within
// a token, so a lexer that overruns goes on in the background until it
// ends, which may be never; while it does, code shows plain rather than
// start another.
var lexing = make(chan struct{}, 1)

// Lexing returns the channel that holds a token while a lexer runs.
// Taking its token, and giving it back, waits for a lexer that overran to
// end, as a test does so that what it lexes next highlights.
func Lexing() chan struct{} { return lexing }

type key struct {
	lexer string
	code  uint64
}

// maxOverruns is how many overruns are kept at most, a few bytes each.
const maxOverruns = 4096

var (
	seed       = maphash.MakeSeed()
	overrunsMu sync.Mutex
	// overruns keeps the code each lexer overran on for as long as the
	// process runs, so it shows plain without running the lexer again,
	// however often it is asked for.
	overruns map[key]bool
)

func keyOf(l chroma.Lexer, code string) key {
	return key{lexer: l.Config().Name, code: maphash.String(seed, code)}
}

func overran(k key) bool {
	overrunsMu.Lock()
	defer overrunsMu.Unlock()
	return overruns[k]
}

func remember(k key) {
	overrunsMu.Lock()
	defer overrunsMu.Unlock()
	if overruns == nil || len(overruns) >= maxOverruns {
		overruns = make(map[key]bool)
	}
	overruns[k] = true
}

// ErrBusy is the error of a lexer that didn't run because another still
// does, one that overran or one that didn't end in time to be waited for.
// Code it leaves plain may highlight later.
var ErrBusy = errors.New("syntax: a lexer that overran still runs")

// ErrOverran is the error of a lexer that overran on the code, now or
// before.
var ErrOverran = errors.New("syntax: the lexer overran")

// Tokenise returns the tokens of code if l tokenises all of it within
// limit, and how long it took, the wait for the lexer that runs first
// included; a lexer that doesn't finish isn't waited for. If limit is at
// least Limit, l isn't run on code again once it overran. It returns
// ErrOverran without running l, and no time, if l overran on code before.
// If another lexer runs it waits for it up to Wait, or limit if that is
// less, unless that one overran, and returns ErrBusy and the time waited
// if it doesn't end.
func Tokenise(l chroma.Lexer, code string, limit time.Duration) (toks []chroma.Token, took time.Duration, err error) {
	k := keyOf(l, code)
	if overran(k) {
		return nil, 0, ErrOverran
	}
	r := lex(l, code, k)
	var waited time.Duration
	if r == nil {
		var err error
		if r, waited, err = lexAfter(l, code, k, min(Wait, limit)); err != nil {
			return nil, waited, err
		}
	}
	select {
	case <-r.done:
		<-lexing
		toks, err := r.result()
		if err != nil {
			remember(k)
			return nil, waited + r.took(), fmt.Errorf("tokenise: %w", err)
		}
		return toks, waited + r.took(), nil
	case <-r.clock.After(max(limit-waited, 0)):
		r.halt()
		if limit >= Limit {
			remember(k)
		}
		return nil, limit, ErrOverran
	}
}

// Wait is how long Head waits for a lexer that overran to end, as one
// asked to stop does once it finishes the token it is on.
const Wait = 300 * time.Millisecond

// Head returns the tokens l makes of code within limit, or until ctx is
// done: all of them if it finishes, else those of the start of code, and
// the lexer stops at the next token. It waits up to Wait for a lexer that
// overran to end first, and returns ErrBusy if it doesn't. It returns
// ErrOverran without running l if l overran on a token of code before.
func Head(ctx context.Context, l chroma.Lexer, code string, limit time.Duration) ([]chroma.Token, error) {
	k := keyOf(l, code)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if overran(k) {
		return nil, ErrOverran
	}
	r := lex(l, code, k)
	if r == nil {
		var err error
		if r, err = await(ctx, l, code, k, Wait); err != nil {
			return nil, err
		}
	}
	select {
	case <-r.done:
		<-lexing
	case <-r.clock.After(limit):
		r.halt()
	case <-ctx.Done():
		r.halt()
	}
	toks, err := r.result()
	if err != nil {
		return nil, fmt.Errorf("tokenise: %w", err)
	}
	return toks, nil
}

// run is a lexer running on some code in a goroutine of its own.
type run struct {
	// clock is what the run is timed by, from start to end, whatever
	// clock lexers started after it use.
	clock Clock
	start time.Time
	// done is closed once the lexer finished.
	done chan struct{}

	mu   sync.Mutex
	toks []chroma.Token
	err  error
	// halted is when the lexer was asked to stop, or zero.
	halted   time.Time
	finished bool
}

// lex starts l on code, or returns nil if a lexer that overran still
// runs.
func lex(l chroma.Lexer, code string, k key) *run {
	select {
	case lexing <- struct{}{}:
		return start(l, code, k)
	default:
		return nil
	}
}

// overrunning is how many lexers were asked to stop and still run.
var overrunning atomic.Int32

// await starts l on code once the lexer that holds the token ends, waiting
// at most wait by the lexers' clock or until ctx is done. It returns
// ErrBusy if the token isn't free by then, and ErrOverran if l overran on
// code while it waited.
func await(ctx context.Context, l chroma.Lexer, code string, k key, wait time.Duration) (*run, error) {
	select {
	case lexing <- struct{}{}:
		if overran(k) {
			<-lexing
			return nil, ErrOverran
		}
		return start(l, code, k), nil
	case <-now().After(wait):
		return nil, ErrBusy
	case <-ctx.Done():
		return nil, ErrBusy
	}
}

// lexAfter starts l on code once the lexer that holds the token ends, if
// that one has not overrun, as another render that lexes at the same time
// does: it ends within its limit, so it is waited for, and code that
// highlights alone doesn't show plain because two renders met. It waits at
// most wait, which is what the caller can spend, and returns how long it
// waited. A lexer that overran at the start is not waited for, and one that
// overruns during the wait only ends it at wait.
func lexAfter(l chroma.Lexer, code string, k key, wait time.Duration) (*run, time.Duration, error) {
	if overrunning.Load() > 0 {
		return nil, 0, ErrBusy
	}
	c := now()
	t0 := c.Now()
	r, err := await(context.Background(), l, code, k, wait)
	return r, c.Now().Sub(t0), err
}

// start starts l on code with the lexing token the caller took, which
// the lexer holds until it ends, or whoever waits for it until then.
func start(l chroma.Lexer, code string, k key) *run {
	c := now()
	r := &run{clock: c, start: c.Now(), done: make(chan struct{})}
	go func() {
		// Whatever ends the lexer must go through finish, or the token
		// is never given back: a recover added here would call it with
		// an error.
		if halted := r.finish(r.lex(l, code, k)); halted {
			overrunning.Add(-1)
			<-lexing
		}
	}()
	return r
}

// lex runs l on code, keeping its tokens, until it finishes or is asked
// to stop.
func (r *run) lex(l chroma.Lexer, code string, k key) error {
	it, err := l.Tokenise(nil, code)
	if err != nil {
		return err
	}
	for r.running(k) {
		t := it()
		if t == chroma.EOF {
			break
		}
		r.add(t)
	}
	return nil
}

// running reports whether the lexer is to go on, as it is until asked to
// stop. A step it took long after it was asked to was an overrun on the
// code it lexes.
func (r *run) running(k key) bool {
	r.mu.Lock()
	halted := r.halted
	r.mu.Unlock()
	if halted.IsZero() {
		return true
	}
	if r.clock.Now().Sub(halted) > Limit {
		remember(k)
	}
	return false
}

// add keeps t, unless the lexer was asked to stop.
func (r *run) add(t chroma.Token) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.halted.IsZero() {
		r.toks = append(r.toks, t)
	}
}

// finish ends the run with err, and reports whether the lexer was asked
// to stop, so that no one waits for it and it gives its token back
// itself. Whoever waits for it gives the token back, so that it can
// start another lexer at once.
func (r *run) finish(err error) (halted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
	r.finished = true
	close(r.done)
	return !r.halted.IsZero()
}

// halt asks the lexer to stop at its next token, or gives its token back
// if it finished already.
func (r *run) halt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		<-lexing
		return
	}
	r.halted = r.clock.Now()
	overrunning.Add(1)
}

// took returns how long the lexer has run.
func (r *run) took() time.Duration {
	return r.clock.Now().Sub(r.start)
}

// result returns the tokens made so far, which the lexer no longer
// changes once halted or done.
func (r *run) result() ([]chroma.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clip(r.toks), r.err
}
