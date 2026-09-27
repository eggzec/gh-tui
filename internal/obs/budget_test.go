package obs

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

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
			if tt.limit > 0 {
				s.HTTP(HTTP{API: GraphQL, Rate: Rate{Resource: "graphql", Limit: tt.limit}})
			}
			ctx := ForPrefetch(context.Background())
			for _, c := range tt.costs {
				ChargeGraphQL(ctx, c)
				// What the user asks for costs the budget nothing.
				ChargeGraphQL(context.Background(), c)
			}
			got := s.Summary().Budget
			if (got.Until != nil) != tt.wantSpent {
				t.Errorf("until = %v, want one only once spent", got.Until)
			}
			got.Until = nil
			want := BudgetSummary{Points: tt.wantPoints, Budget: tt.wantBudget, Spent: tt.wantSpent}
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
				if b := s.Summary().Budget; b.Points != 0 || b.Spent || b.Until != nil {
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
