// Package optimistic runs changes that are shown before the server confirms
// them.
//
// A service applies a change to its cache at once and returns an Op. The tui
// re-renders from the cache in Update, then runs Op.Do in a tea.Cmd. If the
// server refuses the change, Do rolls the cache back and the tui re-renders
// again.
package optimistic

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// ErrDone is returned by Do when the Op was already sent or rolled back.
var ErrDone = errors.New("optimistic: op already done")

// Op is a change that is applied locally and not yet sent to the server. It
// is safe for concurrent use.
type Op struct {
	send      func(ctx context.Context) error
	rollbacks []func()

	mu   sync.Mutex
	done bool
}

// New returns an Op that sends the change with send. The rollbacks undo the
// local change; they run in reverse order, like deferred calls. send should
// store the server's response in the cache when it succeeds.
func New(send func(ctx context.Context) error, rollbacks ...func()) *Op {
	return &Op{send: send, rollbacks: rollbacks}
}

// Refused returns an Op for a change that was refused before it was
// applied, such as one the token lacks a scope for: it changes nothing,
// and its Do returns err without sending anything.
func Refused(err error) *Op {
	return New(func(context.Context) error { return err })
}

// Do sends the change. If sending fails, Do rolls back the local change and
// returns the error.
func (o *Op) Do(ctx context.Context) error {
	if !o.settle() {
		return ErrDone
	}
	if err := o.send(ctx); err != nil {
		o.undo()
		return err
	}
	return nil
}

// Rollback undoes the local change without sending it, for example when the
// user cancels before the change is sent. It does nothing once Do was called.
func (o *Op) Rollback() {
	if o.settle() {
		o.undo()
	}
}

// settle marks the Op done and reports whether it wasn't already.
func (o *Op) settle() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.done {
		return false
	}
	o.done = true
	return true
}

func (o *Op) undo() {
	for _, r := range slices.Backward(o.rollbacks) {
		r()
	}
}
