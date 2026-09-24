package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDashboardCalendarGlyph(t *testing.T) {
	tests := []struct {
		glyph string
		ok    bool
	}{
		{DefaultCalendarGlyph, true},
		{"▪", true},
		{"#", true},
		{"", false},
		{"##", false},
		{"世", false},
		{"\x1b[31m#", false},
	}
	for _, tt := range tests {
		cfg := Default()
		cfg.Dashboard.CalendarGlyph = tt.glyph
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("calendar_glyph %q: Validate() = %v, want ok = %v", tt.glyph, err, tt.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "dashboard.calendar_glyph") {
			t.Errorf("calendar_glyph %q: Validate() = %v, want it to name the field", tt.glyph, err)
		}
	}
}

func TestDashboardFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("dashboard:\n  calendar_glyph: \"▪\"\nkeys:\n  dashboard: [\"~\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvLog, "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dashboard.CalendarGlyph != "▪" {
		t.Errorf("calendar_glyph = %q, want ▪", cfg.Dashboard.CalendarGlyph)
	}
	if got := cfg.Keys[ActionDashboard]; !slices.Equal(got, []string{"~"}) {
		t.Errorf("dashboard = %v, want [~]", got)
	}
}

func TestDashboardActions(t *testing.T) {
	defaults := Default().Keys
	for action, want := range map[string][]string{
		ActionDashboard:   {"0"},
		ActionPane4:       {"4"},
		ActionPane5:       {"5"},
		ActionNextOwner:   {"]", "right"},
		ActionPrevOwner:   {"[", "left"},
		ActionCurrentRepo: {"."},
	} {
		if got := defaults[action]; !slices.Equal(got, want) {
			t.Errorf("default %s = %v, want %v", action, got, want)
		}
	}
	// The dashboard key is on every screen, so no other action may take it.
	for action, keys := range defaults {
		if action != ActionDashboard && slices.Contains(keys, "0") {
			t.Errorf("%s also binds 0", action)
		}
	}
}
