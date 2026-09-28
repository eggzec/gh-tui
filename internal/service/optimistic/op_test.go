package optimistic

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
)

// recorder logs calls so tests can check what ran and in which order.
type recorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *recorder) fn(name string) func() {
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, name)
	}
}

func (r *recorder) send(err error) func(context.Context) error {
	return func(context.Context) error {
		r.fn("send")()
		return err
	}
}

func (r *recorder) check(t *testing.T, want ...string) {
	t.Helper()
	if !slices.Equal(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestDoSucceeds(t *testing.T) {
	var r recorder
	op := New(r.send(nil), r.fn("undo"))
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	op.Rollback()
	r.check(t, "send")
}

func TestDoFailureRollsBackInReverse(t *testing.T) {
	var r recorder
	errBoom := errors.New("boom")
	op := New(r.send(errBoom), r.fn("undo list"), r.fn("undo detail"))
	if err := op.Do(t.Context()); !errors.Is(err, errBoom) {
		t.Fatalf("Do error = %v, want %v", err, errBoom)
	}
	r.check(t, "send", "undo detail", "undo list")
}

func TestRollbackBeforeDo(t *testing.T) {
	var r recorder
	op := New(r.send(nil), r.fn("undo"))
	op.Rollback()
	op.Rollback()
	if err := op.Do(t.Context()); !errors.Is(err, ErrDone) {
		t.Errorf("Do after Rollback = %v, want ErrDone", err)
	}
	r.check(t, "undo")
}

func TestDoTwice(t *testing.T) {
	var r recorder
	op := New(r.send(nil))
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if err := op.Do(t.Context()); !errors.Is(err, ErrDone) {
		t.Errorf("second Do = %v, want ErrDone", err)
	}
	r.check(t, "send")
}

func TestDoPassesContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	op := New(func(ctx context.Context) error { return ctx.Err() })
	if err := op.Do(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Do = %v, want context.Canceled", err)
	}
}

func TestConcurrentDoAndRollback(t *testing.T) {
	var r recorder
	op := New(r.send(nil), r.fn("undo"))
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { _ = op.Do(t.Context()) })
		wg.Go(op.Rollback)
	}
	wg.Wait()
	if len(r.calls) != 1 {
		t.Errorf("calls = %v, want exactly one of send or undo", r.calls)
	}
}

func TestRefused(t *testing.T) {
	errNo := errors.New("no")
	op := Refused(errNo)
	if err := op.Do(t.Context()); !errors.Is(err, errNo) {
		t.Errorf("Do = %v, want %v", err, errNo)
	}
	if err := op.Do(t.Context()); !errors.Is(err, ErrDone) {
		t.Errorf("second Do = %v, want ErrDone", err)
	}
}
