package main

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

func TestNewRevalidator(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := config.Default().Cache
				cfg.Revalidate.Enabled = enabled
				var (
					mu        sync.Mutex
					checked   int
					published []string
				)
				source := func() []revalidate.Entry {
					return []revalidate.Entry{{ID: "issue:1", Check: func(context.Context) revalidate.Result {
						mu.Lock()
						defer mu.Unlock()
						checked++
						return revalidate.Result{Status: revalidate.Changed, Sync: "issues:octo/hello"}
					}}}
				}
				publish := func(key string) {
					mu.Lock()
					defer mu.Unlock()
					published = append(published, key)
				}
				r := newRevalidator(cfg, publish, source)
				if (r != nil) != enabled {
					t.Fatalf("newRevalidator = %v, want one only when enabled", r)
				}
				if r == nil {
					return
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				go func() { _ = r.Run(ctx) }()
				synctest.Sleep(time.Minute)
				mu.Lock()
				defer mu.Unlock()
				if checked != 1 || !slices.Equal(published, []string{"issues:octo/hello"}) {
					t.Errorf("checked %d, published %q; want the entry checked and its change published", checked, published)
				}
			})
		})
	}
}

func TestFanOut(t *testing.T) {
	var got []string
	fanOut(func(s string) { got = append(got, "a"+s) }, func(s string) { got = append(got, "b"+s) })("!")
	if !slices.Equal(got, []string{"a!", "b!"}) {
		t.Errorf("fanOut called %q", got)
	}
}
