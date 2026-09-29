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
