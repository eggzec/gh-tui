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
