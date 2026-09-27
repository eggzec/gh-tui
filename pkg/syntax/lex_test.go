package syntax

import (
	"context"
	"errors"
	"runtime"
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
	before := runtime.NumGoroutine()
	l := newStalling()
	start := time.Now()
	toks, err := Head(t.Context(), l, "stalled", 10*time.Millisecond)
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v", d)
	}
	if got := text(toks); got != "st" || err != nil {
		t.Errorf("gave %q, %v, want the tokens before the stall", got, err)
	}
	if _, err := Head(t.Context(), Lexer("go"), "x := 1", time.Second); !errors.Is(err, ErrBusy) {
		t.Errorf("another lexer ran beside the stalled one: %v", err)
	}
	if _, took, err := Tokenise(Lexer("go"), "x := 1", time.Second); !errors.Is(err, ErrBusy) || took != 0 {
		t.Errorf("another lexer ran beside the stalled one for %v: %v", took, err)
	}
	if n := runtime.NumGoroutine(); n != before+1 {
		t.Errorf("%d goroutines run, want the lexer's beside the %d before", n, before)
	}
	// A stall past Limit after the lexer was asked to stop is an overrun.
	time.Sleep(2 * Limit)
	close(l.release)
	if n := settle(before); n > before {
		t.Fatalf("%d goroutines run after the lexer ended, %d before", n, before)
	}
	if _, err := Head(t.Context(), l, "stalled", time.Second); !errors.Is(err, ErrOverran) {
		t.Errorf("the code the lexer stalled on was lexed again: %v", err)
	}
	if toks, err := Head(t.Context(), l, "other", time.Second); text(toks) != "other" || err != nil {
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
	_, _ = Head(t.Context(), l, "slow", 10*time.Millisecond)
	close(l.release)
	idle(t)
	if toks, err := Head(t.Context(), l, "slow", time.Second); text(toks) != "slow" || err != nil {
		t.Errorf("gave %q, %v the second time", text(toks), err)
	}
}

func TestHeadStopsWhenCancelled(t *testing.T) {
	idle(t)
	l := newStalling()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if toks, _ := Head(ctx, l, "code", time.Second); toks != nil || l.runs.Load() != 0 {
		t.Error("lexed for a cancelled context")
	}
	ctx, cancel = context.WithCancel(t.Context())
	time.AfterFunc(10*time.Millisecond, cancel)
	start := time.Now()
	_, _ = Head(ctx, l, "code", time.Minute)
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("took %v after the context was cancelled", d)
	}
	close(l.release)
	idle(t)
}

// Head waits a while for a lexer that overran to end, as one does soon
// after it was asked to stop, and runs once it did.
func TestHeadWaitsForTheGate(t *testing.T) {
	idle(t)
	lexing <- struct{}{}
	time.AfterFunc(Wait/10, func() { <-lexing })
	if toks, err := Head(t.Context(), Lexer("go"), "x := 1", time.Second); text(toks) != "x := 1" || err != nil {
		t.Errorf("gave %q, %v once the gate was free", text(toks), err)
	}
	lexing <- struct{}{}
	defer func() { <-lexing }()
	start := time.Now()
	if _, err := Head(t.Context(), Lexer("go"), "x := 1", time.Second); !errors.Is(err, ErrBusy) {
		t.Errorf("ran while the gate was taken: %v", err)
	}
	if d := time.Since(start); d > 10*Wait {
		t.Errorf("waited %v for the gate", d)
	}
}
