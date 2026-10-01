package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUIIcons(t *testing.T) {
	if got := Default().UI.Icons; got != IconsNerd {
		t.Errorf("default icons = %q, want %q", got, IconsNerd)
	}
	for _, tt := range []struct {
		icons string
		ok    bool
	}{
		{IconsNerd, true},
		{IconsUnicode, true},
		{IconsASCII, true},
		{"", false},
		{"Nerd", false},
		{"emoji", false},
	} {
		cfg := Default()
		cfg.UI.Icons = tt.icons
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("icons %q: Validate() = %v, want ok = %v", tt.icons, err, tt.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "ui.icons") {
			t.Errorf("icons %q: Validate() = %v, want it to name the field", tt.icons, err)
		}
	}
}

func TestUIFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  icons: ascii\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvLog, "")
	cfg, _, err := loadBase(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UI.Icons != IconsASCII {
		t.Errorf("icons = %q, want %q", cfg.UI.Icons, IconsASCII)
	}
}

func TestDateFormats(t *testing.T) {
	tests := []struct {
		format string
		ok     bool
	}{
		{DateRelative, true},
		{DateAbsolute, true},
		{"2006-01-02 15:04", true},
		{"Jan _2", true},
		{"", false},
		{"   ", false},
		{"yesterday", false},
		{"1/2/2006 3:04PM", true},
		{"Jan 2\n15:04", false},
		{"2006\x1b[31m", false},
		// "Wednesday, 30 September 2026 23:59:59.999999999" is 49 cells.
		{"Monday, 2 January 2006 15:04:05.000000000", false},
		// "Wednesday, 30 September 2026 23:59 UTC" is 38.
		{"Monday, 2 January 2006 15:04 MST", true},
	}
	for _, tt := range tests {
		cfg := Default()
		cfg.UI.DateFormat = tt.format
		if err := cfg.Validate(); (err == nil) != tt.ok {
			t.Errorf("date_format %q: Validate() = %v, want ok = %v", tt.format, err, tt.ok)
		}
	}
}
