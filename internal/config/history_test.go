package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHistoryDateFormats(t *testing.T) {
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
	}
	for _, tt := range tests {
		cfg := Default()
		cfg.History.DateFormat = tt.format
		if err := cfg.Validate(); (err == nil) != tt.ok {
			t.Errorf("date_format %q: Validate() = %v, want ok = %v", tt.format, err, tt.ok)
		}
	}
}

func TestHistoryNeedsARowField(t *testing.T) {
	cfg := Default()
	cfg.History.Row = nil
	cfg.History.Detail = nil
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "history.row: needs at least one field") {
		t.Errorf("Validate() = %v, want the row to need a field", err)
	}
	// The header may show the subject alone.
	if strings.Contains(err.Error(), "history.detail") {
		t.Errorf("Validate() = %v, want an empty detail allowed", err)
	}
}

func TestHistoryListsReplaceTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("history:\n  row: [subject]\n  detail: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.History.Row; !slices.Equal(got, []string{FieldSubject}) {
		t.Errorf("row = %v, want [subject] alone", got)
	}
	if got := cfg.History.Detail; len(got) != 0 {
		t.Errorf("detail = %v, want none", got)
	}
	if got := cfg.History.Prefetch; got != defaultHistory().Prefetch {
		t.Errorf("prefetch = %+v, want the default", got)
	}
}

func TestHistoryActions(t *testing.T) {
	defaults := Default().Keys
	for action, want := range map[string]string{ActionHistory: "B", ActionResetBase: "H", ActionUseAsBase: "space"} {
		if got := defaults[action]; !slices.Equal(got, []string{want}) {
			t.Errorf("default %s = %v, want [%s]", action, got, want)
		}
	}
}
