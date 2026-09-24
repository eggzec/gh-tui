package obs

import (
	"context"
	"io"
	"log/slog"
)

// Handler adds the trace of the context a record is logged with to the
// record, after the session id, so that slog.InfoContext(ctx, …) carries
// the ids without the caller naming them.
type Handler struct {
	h slog.Handler
}

// NewHandler returns a handler that adds the trace ids to each record and
// passes it on to h.
func NewHandler(h slog.Handler) *Handler {
	return &Handler{h: h}
}

// NewLogger returns a logger that writes records at level and above to w,
// one JSON object per line, each with session as its session_id and the
// trace ids of its context.
func NewLogger(w io.Writer, level slog.Leveler, session string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(NewHandler(h).WithAttrs([]slog.Attr{slog.String("session_id", session)}))
}

// Enabled reports whether the handler it wraps handles level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.h.Enabled(ctx, level)
}

// Handle adds the trace ids of ctx, if any, before the record's own
// attributes, and passes the record on.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	t, ok := ctx.Value(traceKey{}).(*trace)
	if !ok {
		return h.h.Handle(ctx, r)
	}
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(slog.String("trace_id", t.id), slog.String("trace", t.name))
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(a)
		return true
	})
	return h.h.Handle(ctx, out)
}

// WithAttrs returns a handler whose records carry attrs.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{h: h.h.WithAttrs(attrs)}
}

// WithGroup returns a handler that puts the attributes of its records in a
// group. The trace ids go in the group too.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{h: h.h.WithGroup(name)}
}
