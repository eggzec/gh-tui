package main

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// newRevalidator returns the revalidator of what sources list, as cfg says,
// slowed down by slowdown while the terminal is unfocused, which publishes
// what changed with publish. It returns nil when cfg turns
// revalidation off. An entry fetched within the TTL is left alone, since a
// read wouldn't ask GitHub about it either.
func newRevalidator(cfg config.Cache, slowdown int, publish func(key string), sources ...revalidate.Source) *revalidate.Revalidator {
	r := cfg.Revalidate
	if !r.Enabled {
		return nil
	}
	scope := revalidate.ScopeRecent
	if r.Scope == config.ScopeAll {
		scope = revalidate.ScopeAll
	}
	return revalidate.New(sources,
		revalidate.WithInterval(r.Interval),
		revalidate.WithBudget(r.Budget),
		revalidate.WithScope(scope),
		revalidate.WithFreshFor(cfg.TTL),
		revalidate.WithIdleMultiplier(slowdown),
		revalidate.WithPublish(publish),
	)
}

// fanOut returns a function that calls each of fns.
func fanOut[T any](fns ...func(T)) func(T) {
	return func(v T) {
		for _, fn := range fns {
			fn(v)
		}
	}
}
