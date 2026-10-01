package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		want Size
	}{
		{"0", 0},
		{"512", 512},
		{"512B", 512},
		{"64KiB", 64 * KiB},
		{"64kib", 64 * KiB},
		{"1MB", 1_000_000},
		{"1.5 MiB", MiB + MiB/2},
		{"2kB", 2000},
		{"1GiB", GiB},
		{" 3 gb ", 3_000_000_000},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "KiB", "-1", "64 KB B", "12 parsecs", "1..2MB", "1e3", "9999999999GiB"} {
		if got, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) = %d, want an error", in, got)
		}
	}
}

func TestSizeString(t *testing.T) {
	tests := []struct {
		in   Size
		want string
	}{
		{0, "0B"},
		{1000, "1000B"},
		{64 * KiB, "64KiB"},
		{MiB, "1MiB"},
		{MiB + MiB/2, "1536KiB"},
		{2 * GiB, "2GiB"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Size(%d).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLoadSizes(t *testing.T) {
	tests := []struct {
		yaml    string
		want    Size
		wantErr string
	}{
		{yaml: "65536", want: 64 * KiB},
		{yaml: `"32KiB"`, want: 32 * KiB},
		{yaml: "8KB", want: 8000},
		{yaml: "0", want: 0},
		{yaml: "big", wantErr: `line 4: invalid size "big"`},
		{yaml: "[1]", wantErr: "line 4: want a size"},
		{yaml: "-5", wantErr: `invalid size "-5"`},
		{yaml: "2MiB", wantErr: "prefetch.files.preview.max_size: must be between 0B and files.preview.max_size (1MiB), got 2MiB"},
	}
	for _, tt := range tests {
		t.Run(tt.yaml, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			data := "prefetch:\n  files:\n    preview:\n      max_size: " + tt.yaml + "\n"
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, _, err := loadBase(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load error = %v", err)
			}
			p := cfg.Prefetch.Files.Preview
			if p.MaxSize != tt.want {
				t.Errorf("max_size = %v, want %v", p.MaxSize, tt.want)
			}
			r, _ := cfg.Prefetch.Resolve("files", "preview")
			if r.Window != (Window{After: 32}) || cfg.Files.Preview.MaxSize != MiB {
				t.Errorf("window %+v, files %+v, want the other fields defaulted", r.Window, cfg.Files)
			}
		})
	}
}

func TestValidateFileSizes(t *testing.T) {
	cfg := Default()
	cfg.Files.Preview.MaxSize = 0
	cfg.Prefetch.Files.Preview.MaxSize = -1
	err := cfg.Validate()
	for _, w := range []string{
		"files.preview.max_size: must be between 1B and 100MiB, got 0B",
		"prefetch.files.preview.max_size: must be between 0B and files.preview.max_size (0B), got -1B",
	} {
		if err == nil || !strings.Contains(err.Error(), w) {
			t.Errorf("Validate() = %v, want it to contain %q", err, w)
		}
	}
	cfg.Files.Preview.MaxSize = 101 * MiB
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "files.preview.max_size") {
		t.Errorf("Validate() = %v, want preview.max_size over GitHub's limit rejected", err)
	}
}
