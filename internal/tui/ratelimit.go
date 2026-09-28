package tui

import "github.com/eggzec/gh-tui/internal/core"

// RateLimits tells the rate limits of the account, as the client knows
// them, without I/O.
type RateLimits interface {
	RateStatus() core.RateStatus
}

// WithRateStatus sets what tells the rate limits, which the app reads
// again on a ui.SyncMsg with the key core.SyncRateLimit.
func WithRateStatus(r RateLimits) Option {
	return func(m *Model) { m.rates = r }
}

// WithOnline sets the function called when GitHub answers again after the
// app couldn't reach it, such as to poll at once what backed off
// meanwhile. It is called from Update, as the sections are sent a
// ui.OnlineMsg, and must not block.
func WithOnline(online func()) Option {
	return func(m *Model) { m.online = online }
}
