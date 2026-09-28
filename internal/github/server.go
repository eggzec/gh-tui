package github

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Headers of GitHub's answers that the server record reads.
const (
	apiVersionSelectedHeader = "X-GitHub-Api-Version-Selected"
	requestIDHeader          = "X-GitHub-Request-Id"
)

// maxSkew is how far GitHub's clock may be from the local one before the
// server record warns: the times the rate limits reset are GitHub's.
const maxSkew = 30 * time.Second

// observeServer logs, once per client and so once per host, what the
// first answer of GitHub says of the server: its Enterprise Server
// version, if it is one, the API version it chose, how far its clock is
// from the local one, and whether it limits the rate. An answer without
// X-GitHub-Request-Id may be a proxy's, so it is passed over. It warns
// when the API version isn't the one asked for, or the clocks are far
// apart.
func (c *Client) observeServer(ctx context.Context, h http.Header, arrived time.Time) {
	if h.Get(requestIDHeader) == "" || !c.toldServer.CompareAndSwap(false, true) {
		return
	}
	level := slog.LevelInfo
	attrs := []slog.Attr{slog.String("span", "http")}
	if v := h.Get(enterpriseHeader); v != "" {
		attrs = append(attrs, slog.String("ghes_version", v))
	}
	if v := h.Get(apiVersionSelectedHeader); v != "" {
		attrs = append(attrs, slog.String("api_version_selected", v))
		if v != apiVersion {
			level = slog.LevelWarn
			attrs = append(attrs, slog.String("api_version_sent", apiVersion))
		}
	}
	// Date is in whole seconds, so the skew is too, give or take one.
	if date, err := http.ParseTime(h.Get("Date")); err == nil {
		skew := date.Sub(arrived.Truncate(time.Second))
		attrs = append(attrs, slog.Int64("skew_ms", skew.Milliseconds()))
		if skew.Abs() > maxSkew {
			level = slog.LevelWarn
		}
	}
	attrs = append(attrs, slog.Bool("rate_limits", h.Get("X-RateLimit-Limit") != ""))
	slog.LogAttrs(ctx, level, "server", attrs...)
}

// enterpriseVersion returns the version of the Enterprise Server the
// client talks to, as its answers told it, or "".
func (c *Client) enterpriseVersion() string {
	if v := c.version.Load(); v != nil {
		return *v
	}
	return ""
}
