package actions

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// runSpan is how long the latest attempt of r ran, or has run so far.
func runSpan(r core.Run, now time.Time) (time.Duration, bool) {
	start := r.RunStartedAt
	if start.IsZero() {
		start = r.CreatedAt
	}
	var end time.Time
	if r.Done() {
		end = r.UpdatedAt
	}
	return ui.Span(start, end, now)
}
