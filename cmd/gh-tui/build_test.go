package main

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
)

// The dashboard reads ahead only when both its switch and that of the
// details are on.
func TestDashboardPrefetch(t *testing.T) {
	tests := []struct {
		dashboard, details, want bool
	}{
		{true, true, true},
		{true, false, false},
		{false, true, false},
		{false, false, false},
	}
	for _, tt := range tests {
		cfg := config.Default()
		cfg.Dashboard.Prefetch = tt.dashboard
		cfg.Details.Prefetch.Enabled = tt.details
		if got := dashboardPrefetch(cfg); got != tt.want {
			t.Errorf("dashboard.prefetch %v, details.prefetch.enabled %v: read ahead = %v, want %v", tt.dashboard, tt.details, got, tt.want)
		}
	}
	if !dashboardPrefetch(config.Default()) {
		t.Error("the default config doesn't read the dashboard ahead")
	}
}
