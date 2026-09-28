package obs

import (
	"path/filepath"
	"testing"
)

func TestShortHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "ali")
	t.Setenv("HOME", home)
	cache := filepath.Join(home, ".cache", "gh-tui", "entry")
	other := filepath.Join(filepath.Dir(home), "alice", "x")
	short := filepath.Join("~", ".cache", "gh-tui", "entry")
	tests := []struct{ in, want string }{
		{"open " + cache + ": no space left on device", "open " + short + ": no space left on device"},
		{`rename "` + cache + `" "` + cache + `.gz": denied`, `rename "` + short + `" "` + short + `.gz": denied`},
		{home, "~"},
		{"in " + home + ": gone", "in ~: gone"},
		{"open " + other + ": denied", "open " + other + ": denied"},
		{"no path here", "no path here"},
	}
	for _, tt := range tests {
		if got := ShortHome(tt.in); got != tt.want {
			t.Errorf("ShortHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
