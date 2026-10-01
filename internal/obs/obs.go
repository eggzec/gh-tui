// Package obs is how gh-tui tells what it did: structured log records that
// carry the ids of the session, the user action or background job, and the
// HTTP request they belong to, and counters that a summary record reports
// from time to time.
//
// Records go through log/slog. Start a trace where a user action or a
// background job starts, and pass its context down: a record logged with
// that context, such as with [slog.InfoContext], carries the trace's id once
// the default logger's handler is a [Handler].
package obs

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"
)

// Prefixes of the ids, so that an id tells what it names.
const (
	SessionPrefix = "s_"
	TracePrefix   = "t_"
	RequestPrefix = "r_"
)

// NewID returns prefix followed by 10 random hex digits. Ids need only tell
// apart what one log file holds, not be secret, so they come from the fast
// random source.
func NewID(prefix string) string {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], rand.Uint64())
	return prefix + hex.EncodeToString(b[:5])
}

// trace is the user action or background job a context belongs to.
type trace struct {
	id   string
	name string
}

type traceKey struct{}

// WithTrace returns a context for a new trace named name, such as
// "open.pull" or "revalidate.pass", with an id of its own. Start one per user
// action or background job; the work it does, down to each HTTP request,
// logs with its id.
func WithTrace(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, traceKey{}, &trace{id: NewID(TracePrefix), name: name})
}

// TraceID returns the id and the name of the trace of ctx, or empty strings
// outside of one.
func TraceID(ctx context.Context) (id, name string) {
	if t, ok := ctx.Value(traceKey{}).(*trace); ok {
		return t.id, t.name
	}
	return "", ""
}

// From returns the default logger with the ids of ctx, for code that logs
// without a context.
func From(ctx context.Context) *slog.Logger {
	l := slog.Default()
	if t, ok := ctx.Value(traceKey{}).(*trace); ok {
		return l.With(slog.String("trace_id", t.id), slog.String("trace", t.name))
	}
	return l
}

// Begin starts a trace named name, as WithTrace does, and returns its
// context with a function that ends it. Ending it logs how long it took at
// debug level, or the error at error level, unless the trace was canceled,
// such as by navigating away. Args are added to the end record, as in
// [slog.Log].
func Begin(ctx context.Context, name string) (traced context.Context, end func(err error, args ...any)) {
	ctx = WithTrace(ctx, name)
	start := time.Now()
	return ctx, func(err error, args ...any) {
		End(ctx, start, err, args...)
	}
}

// End logs the end of the trace of ctx that started at start, as the
// function Begin returns does.
func End(ctx context.Context, start time.Time, err error, args ...any) {
	EndWith(ctx, slog.Default(), start, err, args...)
}

// EndWith is End through l, for work in the background that should log to
// the logger of the session that started it, even when the default logger
// has changed by the time it ends.
func EndWith(ctx context.Context, l *slog.Logger, start time.Time, err error, args ...any) {
	level, msg := slog.LevelDebug, "done"
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled), ctx.Err() != nil:
		msg = "canceled"
	default:
		level, msg = slog.LevelError, "failed"
	}
	if !l.Enabled(ctx, level) {
		return
	}
	attrs := make([]any, 0, 2+len(args))
	attrs = append(attrs, slog.Float64("duration_ms", Millis(time.Since(start))))
	if err != nil {
		attrs = append(attrs, slog.String("err", err.Error()))
	}
	attrs = append(attrs, args...)
	l.Log(ctx, level, msg, attrs...)
}

// Enabled reports whether the default logger logs records at level with
// ctx. Check it before building the arguments of a record on a hot path.
func Enabled(ctx context.Context, level slog.Level) bool {
	return slog.Default().Enabled(ctx, level)
}

// Millis returns d in milliseconds, to a microsecond.
func Millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}
