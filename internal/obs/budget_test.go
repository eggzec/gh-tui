package obs

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// withBudget sets the share of each quota reads ahead may spend for the
// rest of the test.
func withBudget(t *testing.T, percent int) {
	t.Helper()
	SetPrefetchBudget(percent)
	t.Cleanup(func() { SetPrefetchBudget(0) })
}

func TestPrefetchBudget(t *testing.T) {
	tests := []struct {
		name string
		// limit is the GraphQL limit GitHub reported, or 0 for none.
		limit int
		// costs are those of the queries read ahead, in order.
		costs      []int
		wantPoints int64
		wantBudget int64
		wantSpent  bool
	}{
		{name: "nothing read", wantBudget: 500},
		{name: "under the fallback", costs: []int{100, 200}, wantPoints: 300, wantBudget: 500},
		{name: "at the fallback", costs: []int{250, 250}, wantPoints: 500, wantBudget: 500, wantSpent: true},
		{name: "unknown costs count one", costs: []int{0, 0, 0}, wantPoints: 3, wantBudget: 500},
		{name: "a tenth of the limit", limit: 1000, costs: []int{60, 40}, wantPoints: 100, wantBudget: 100, wantSpent: true},
		{name: "under a larger limit", limit: 10000, costs: []int{600}, wantPoints: 600, wantBudget: 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStats()
			prev := SetDefault(s)
			t.Cleanup(func() { SetDefault(prev) })
			withBudget(t, 10)
			if tt.limit > 0 {
				s.HTTP(HTTP{API: GraphQL, Rate: Rate{Resource: "graphql", Limit: tt.limit}})
			}
			ctx := ForPrefetch(context.Background())
			for _, c := range tt.costs {
				ChargeGraphQL(ctx, c)
				// What the user asks for costs the budget nothing.
				ChargeGraphQL(context.Background(), c)
			}
			got := s.Summary().Budget.GraphQL
			if (got.Until != nil) != tt.wantSpent {
				t.Errorf("until = %v, want one only once spent", got.Until)
			}
			got.Until = nil
			want := BudgetUse{Used: tt.wantPoints, Budget: tt.wantBudget, Spent: tt.wantSpent}
			if got != want {
				t.Errorf("budget = %+v, want %+v", got, want)
			}
			if PrefetchSpent() != tt.wantSpent {
				t.Errorf("PrefetchSpent() = %v, want %v", PrefetchSpent(), tt.wantSpent)
			}
		})
	}
}

func TestPrefetchBudgetRenews(t *testing.T) {
	tests := []struct {
		name string
		// reset is when the GraphQL quota GitHub reported refills, after
		// the start, or 0 for none reported.
		reset time.Duration
		// renew is when the budget should start over.
		renew time.Duration
	}{
		{"with the quota", 20 * time.Minute, 20 * time.Minute},
		{"an hour on without a reported quota", 0, time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewStats()
				prev := SetDefault(s)
				defer SetDefault(prev)
				withBudget(t, 10)
				if tt.reset > 0 {
					s.HTTP(HTTP{API: GraphQL, Rate: Rate{Resource: "graphql", Limit: 1000, Reset: time.Now().Add(tt.reset)}})
				}
				ctx := ForPrefetch(context.Background())
				ChargeGraphQL(ctx, 1000)
				if !PrefetchSpent() {
					t.Fatal("PrefetchSpent() = false once spent")
				}
				time.Sleep(tt.renew - time.Second)
				if !PrefetchSpent() {
					t.Fatal("the budget came back before the quota refilled")
				}
				time.Sleep(time.Second)
				if PrefetchSpent() {
					t.Fatal("the budget didn't come back once the quota refilled")
				}
				if b := s.Summary().Budget.GraphQL; b.Used != 0 || b.Spent || b.Until != nil {
					t.Errorf("budget = %+v, want it started over", b)
				}
				// It is spent again only by a new budget's worth.
				ChargeGraphQL(ctx, 1)
				if PrefetchSpent() {
					t.Error("one point spent the renewed budget")
				}
			})
		})
	}
}

// REST reads ahead spend a budget of the core quota of their own: each
// request counts one, but an answer 304 Not Modified, which the quota
// doesn't count, and a request of another quota, count nothing.
func TestPrefetchBudgetREST(t *testing.T) {
	s := NewStats()
	prev := SetDefault(s)
	t.Cleanup(func() { SetDefault(prev) })
	withBudget(t, 10)
	s.HTTP(HTTP{API: REST, Rate: Rate{Resource: "core", Limit: 100}})
	ctx := ForPrefetch(context.Background())
	for range 9 {
		ChargeREST(ctx, "core", false)
		ChargeREST(ctx, "core", true)
		ChargeREST(ctx, "search", false)
		ChargeREST(context.Background(), "core", false)
	}
	if b := s.Summary().Budget; PrefetchSpent() || b.Spent || b.Core.Used != 9 || b.Core.Budget != 10 || b.GraphQL.Used != 0 {
		t.Fatalf("budget = %+v, want 9 requests of 10 and nothing spent", b)
	}
	ChargeREST(ctx, "core", false)
	if b := s.Summary().Budget; !PrefetchSpent() || !b.Spent || !b.Core.Spent || b.Core.Until == nil || b.GraphQL.Spent {
		t.Errorf("budget = %+v, want the core quota's spent", b)
	}
}

// The budget is a share of the quota GitHub reported, or of 5000 before it
// reported one, and follows the share as it changes.
func TestPrefetchBudgetShare(t *testing.T) {
	s := NewStats()
	prev := SetDefault(s)
	t.Cleanup(func() { SetDefault(prev) })
	withBudget(t, 1)
	if b := s.Summary().Budget; b.GraphQL.Budget != 50 || b.Core.Budget != 50 {
		t.Errorf("budget = %+v, want a percent of 5000 of each", b)
	}
	ctx := ForPrefetch(context.Background())
	ChargeGraphQL(ctx, 60)
	if !PrefetchSpent() {
		t.Fatal("60 points of 50 didn't spend the budget")
	}
	SetPrefetchBudget(2)
	if PrefetchSpent() {
		t.Error("60 points spent a budget raised to 100")
	}
	SetPrefetchBudget(0)
	if !PrefetchSpent() {
		t.Error("nothing set left a budget")
	}
}

// Before anything is read ahead, nothing is spent, whatever the share.
func TestPrefetchBudgetUnspent(t *testing.T) {
	s := NewStats()
	prev := SetDefault(s)
	t.Cleanup(func() { SetDefault(prev) })
	withBudget(t, 0)
	if PrefetchSpent() {
		t.Error("spent before any read ahead")
	}
}

// A budget that wasn't spent starts over too once the window of the quota
// it counts in ends, so that what it used in one hour isn't held against
// the next.
func TestPrefetchBudgetRenewsUnspent(t *testing.T) {
	for _, tt := range []struct {
		name         string
		reset, renew time.Duration
	}{
		{"with the quota", 20 * time.Minute, 20 * time.Minute},
		{"an hour on without a reported quota", 0, time.Hour},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewStats()
				prev := SetDefault(s)
				defer SetDefault(prev)
				withBudget(t, 10)
				if tt.reset > 0 {
					s.HTTP(HTTP{API: GraphQL, Rate: Rate{Resource: "graphql", Limit: 1000, Reset: time.Now().Add(tt.reset)}})
				}
				ctx := ForPrefetch(context.Background())
				ChargeGraphQL(ctx, 60)
				time.Sleep(tt.renew - time.Second)
				if b := s.Summary().Budget.GraphQL; b.Used != 60 {
					t.Fatalf("budget = %+v before the window ended, want 60 used", b)
				}
				time.Sleep(time.Second)
				if b := s.Summary().Budget.GraphQL; b.Used != 0 || b.Spent {
					t.Fatalf("budget = %+v once the window ended, want it started over", b)
				}
				// The next window's budget is whole: 60 more don't spend it.
				ChargeGraphQL(ctx, 60)
				if PrefetchSpent() {
					t.Error("the renewed budget was spent by what the last window used")
				}
			})
		})
	}
}

// A budget a lower share spent is logged as spent, as one a read ahead
// spent is.
func TestPrefetchBudgetLogsSpentByShare(t *testing.T) {
	var buf bytes.Buffer
	prevLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prevLog) })
	s := NewStats()
	prev := SetDefault(s)
	t.Cleanup(func() { SetDefault(prev) })
	withBudget(t, 10)
	ChargeGraphQL(ForPrefetch(context.Background()), 100)
	if strings.Contains(buf.String(), "prefetch budget spent") {
		t.Fatalf("logged %s before the budget was spent", buf.String())
	}
	SetPrefetchBudget(1)
	if !PrefetchSpent() {
		t.Fatal("100 points didn't spend a budget of 50")
	}
	if n := strings.Count(buf.String(), "prefetch budget spent"); n != 1 {
		t.Errorf("logged the spent budget %d times, want once:\n%s", n, buf.String())
	}
}
