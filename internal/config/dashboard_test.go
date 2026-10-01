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
		{"■", true},
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

func TestDashboardContributions(t *testing.T) {
	if got := Default().Dashboard.Contributions; got != Contributions90d {
		t.Errorf("contributions defaults to %q, want %q", got, Contributions90d)
	}
	tests := []struct {
		value string
		days  int
		ok    bool
	}{
		{Contributions30d, 30, true},
		{Contributions90d, 90, true},
		{ContributionsYear, 0, true},
		{"", 0, false},
		{"7d", 0, false},
		{"Year", 0, false},
	}
	for _, tt := range tests {
		cfg := Default()
		cfg.Dashboard.Contributions = tt.value
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("contributions %q: Validate() = %v, want ok = %v", tt.value, err, tt.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "dashboard.contributions") {
			t.Errorf("contributions %q: Validate() = %v, want it to name the field", tt.value, err)
		}
		if tt.ok && cfg.Dashboard.ContributionDays() != tt.days {
			t.Errorf("contributions %q: %d days, want %d", tt.value, cfg.Dashboard.ContributionDays(), tt.days)
		}
	}
}

func TestDashboardFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("dashboard:\n  calendar_glyph: \"▪\"\n  contributions: 30d\nkeys:\n  dashboard: [\"~\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvLog, "")
	cfg, _, err := loadBase(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dashboard.CalendarGlyph != "▪" {
		t.Errorf("calendar_glyph = %q, want ▪", cfg.Dashboard.CalendarGlyph)
	}
	if cfg.Dashboard.Contributions != Contributions30d {
		t.Errorf("contributions = %q, want 30d", cfg.Dashboard.Contributions)
	}
	if got := cfg.Keys[ActionDashboard]; !slices.Equal(got, []string{"~"}) {
		t.Errorf("dashboard = %v, want [~]", got)
	}
}

// The old dashboard.prefetch switch is read as the switch of Waiting on
// you: off turns the pane off, and on, as it was by default, leaves it to
// inherit.
func TestDashboardPrefetchRenamed(t *testing.T) {
	t.Setenv(EnvLog, "")
	tests := []struct {
		name, yaml string
		want       bool
		from       string
		// fails is what the error names, if loading fails.
		fails string
	}{
		{"off", "dashboard:\n  prefetch: false\n", false, "prefetch.dashboard.waiting_on_you.enabled", ""},
		{"on", "dashboard:\n  prefetch: true\n", true, "prefetch.enabled", ""},
		{"on, with prefetch off", "prefetch:\n  enabled: false\ndashboard:\n  prefetch: true\n", false, "prefetch.enabled", ""},
		{"not a switch", "dashboard:\n  prefetch: often\n", false, "", "line 2"},
		{"both names", "dashboard:\n  prefetch: false\nprefetch:\n  dashboard:\n    waiting_on_you: {enabled: true}\n", false, "",
			"set only prefetch.dashboard.waiting_on_you.enabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, renamed, err := loadBase(path)
			if (err == nil) != (tt.fails == "") {
				t.Fatalf("Load = %v, want it to fail = %v", err, tt.fails != "")
			}
			if err != nil {
				if !strings.Contains(err.Error(), tt.fails) {
					t.Errorf("Load = %v, want it to name %s", err, tt.fails)
				}
				return
			}
			if len(renamed) != 1 || renamed[0].Old != "dashboard.prefetch" {
				t.Errorf("renamed = %v, want dashboard.prefetch", renamed)
			}
			r, err := cfg.Prefetch.Resolve("dashboard", "waiting_on_you")
			if err != nil {
				t.Fatal(err)
			}
			if r.Enabled != tt.want || r.From.Enabled != tt.from {
				t.Errorf("waiting_on_you enabled %v (from %s), want %v (from %s)", r.Enabled, r.From.Enabled, tt.want, tt.from)
			}
		})
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
