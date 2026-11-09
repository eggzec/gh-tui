package syntax

import (
	"context"
	"errors"
	"math/rand/v2"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
)

// stalling is a lexer that makes a token of each byte, and stalls on the
// third until release is closed, as one that never finishes does. It
// counts its runs.
type stalling struct {
	chroma.Lexer
	release chan struct{}
	runs    atomic.Int32
}

func newStalling() *stalling {
	return &stalling{Lexer: Lexer("go"), release: make(chan struct{})}
}

func (l *stalling) Tokenise(_ *chroma.TokeniseOptions, code string) (chroma.Iterator, error) {
	l.runs.Add(1)
	i := 0
	return func() chroma.Token {
		if i == len(code) {
			return chroma.EOF
		}
		if i == 2 {
			<-l.release
		}
		i++
		return chroma.Token{Type: chroma.Text, Value: code[i-1 : i]}
	}, nil
}

// A lexer that stalls gives the tokens it made within the limit, and goes
// on alone: while it runs nothing else lexes, and once it ends the code it
// stalled on isn't lexed again.
func TestHeadStopsAtTheLimit(t *testing.T) {
	idle(t)
	l := newStalling()
	stalled, other := unique(t, "stalled"), unique(t, "other")
	toks, err := guardedHead(t.Context(), t, l, stalled, 10*time.Millisecond)
	if got := text(toks); got != "st" || err != nil {
		t.Errorf("gave %q, %v, want the tokens before the stall", got, err)
	}
	if _, err := Head(t.Context(), Lexer("go"), "x := 1", time.Second); !errors.Is(err, ErrBusy) {
		t.Errorf("another lexer ran beside the stalled one: %v", err)
	}
	if _, took, err := Tokenise(Lexer("go"), "x := 1", time.Second); !errors.Is(err, ErrBusy) || took != 0 {
		t.Errorf("another lexer ran beside the stalled one for %v: %v", took, err)
	}
	// A stall past Limit after the lexer was asked to stop is an overrun.
	time.Sleep(2 * Limit)
	close(l.release)
	// The gate is free again once the lexer has ended.
	idle(t)
	if _, err := Head(t.Context(), l, stalled, time.Second); !errors.Is(err, ErrOverran) {
		t.Errorf("the code the lexer stalled on was lexed again: %v", err)
	}
	if toks, err := Head(t.Context(), l, other, time.Second); text(toks) != other || err != nil {
		t.Errorf("other code gave %q, %v", text(toks), err)
	}
	if n := l.runs.Load(); n != 2 {
		t.Errorf("the lexer ran %d times, want once more for the other code", n)
	}
}

// A lexer cut short that stops soon after, as a slow one does on a long
// file, runs on the same code again.
func TestHeadCutsSlowLexersShort(t *testing.T) {
	idle(t)
	l := newStalling()
	slow := unique(t, "slow")
	_, _ = guardedHead(t.Context(), t, l, slow, 10*time.Millisecond)
	close(l.release)
	idle(t)
	if toks, err := Head(t.Context(), l, slow, time.Second); text(toks) != slow || err != nil {
		t.Errorf("gave %q, %v the second time", text(toks), err)
	}
}

// A lexer that runs beside another that has not overrun waits for it, as
// renders that meet do, instead of leaving its code plain.
func TestTokeniseWaitsForALexerThatHasNotOverrun(t *testing.T) {
	idle(t)
	l := newStalling()
	held := make(chan struct{})
	go func() {
		defer close(held)
		_, _, _ = Tokenise(l, unique(t, "held"), time.Hour)
	}()
	for l.runs.Load() == 0 {
		runtime.Gosched()
	}
	time.AfterFunc(20*time.Millisecond, func() { close(l.release) })
	toks, _, err := Tokenise(Lexer("go"), "x := 1", time.Second)
	if err != nil || text(toks) != "x := 1" {
		t.Errorf("gave %q, %v, want the code lexed once the other lexer ended", text(toks), err)
	}
	<-held
}

// The wait for a lexer that has not overrun counts in the time Tokenise
// returns and is cut at the limit, so a caller that spends one budget on
// several blocks waits no longer than the budget in all.
func TestTokeniseWaitIsBoundedByTheLimit(t *testing.T) {
	idle(t)
	gate := Lexing()
	gate <- struct{}{}
	defer func() { <-gate }()
	budget := 60 * time.Millisecond
	start := time.Now()
	for range 10 {
		if budget <= 0 {
			break
		}
		_, took, err := Tokenise(Lexer("go"), "x := 1", budget)
		if !errors.Is(err, ErrBusy) || took <= 0 {
			t.Fatalf("gave %v after %v, want ErrBusy and the time waited", err, took)
		}
		budget -= took
	}
	if d := time.Since(start); d > 5*time.Second/10 {
		t.Errorf("ten blocks waited %v for a held lexer, want about the budget", d)
	}
	if budget > 0 {
		t.Errorf("%v of the budget is left after the waits", budget)
	}
}

func TestHeadStopsWhenCancelled(t *testing.T) {
	idle(t)
	l := newStalling()
	code := unique(t, "code")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if toks, _ := Head(ctx, l, code, time.Second); toks != nil || l.runs.Load() != 0 {
		t.Error("lexed for a cancelled context")
	}
	ctx, cancel = context.WithCancel(t.Context())
	time.AfterFunc(10*time.Millisecond, cancel)
	// The limit is out of reach, so only the context ends Head; one that
	// ignored it fails the guard.
	_, _ = guardedHead(ctx, t, l, code, time.Hour)
	close(l.release)
	idle(t)
}

// Head waits a while for a lexer that overran to end, as one does soon
// after it was asked to stop, and runs once it did.
func TestHeadWaitsForTheGate(t *testing.T) {
	idle(t)
	lexing <- struct{}{}
	time.AfterFunc(Wait/10, func() { <-lexing })
	if toks, err := guardedHead(t.Context(), t, Lexer("go"), "x := 1", time.Second); text(toks) != "x := 1" || err != nil {
		t.Errorf("gave %q, %v once the gate was free", text(toks), err)
	}
	lexing <- struct{}{}
	defer func() { <-lexing }()
	if _, err := guardedHead(t.Context(), t, Lexer("go"), "x := 1", time.Second); !errors.Is(err, ErrBusy) {
		t.Errorf("ran while the gate was taken: %v", err)
	}
}

// guardedHead is Head that fails the test if it hasn't returned after a
// bound that only a hang reaches. The bound is not a limit on how long Head
// may take: without it, a Head that ignored its limit, its context or its
// wait would end in the test's timeout and a dump of every goroutine.
func guardedHead(ctx context.Context, t *testing.T, l chroma.Lexer, code string, limit time.Duration) ([]chroma.Token, error) {
	t.Helper()
	type result struct {
		toks []chroma.Token
		err  error
	}
	done := make(chan result, 1)
	go func() {
		toks, err := Head(ctx, l, code, limit)
		done <- result{toks, err}
	}()
	select {
	case r := <-done:
		return r.toks, r.err
	case <-time.After(30 * time.Second):
		t.Fatal("Head didn't return: it ignored its limit, its context or its wait")
		return nil, nil
	}
}

// unique returns code with a suffix of its own: overruns are kept for as
// long as the process runs, so each run of a test lexes code of its own.
func unique(t *testing.T, code string) string {
	t.Helper()
	return code + " " + t.Name() + strconv.Itoa(rand.Int())
}
