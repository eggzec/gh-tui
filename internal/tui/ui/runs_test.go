package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// A job or step that was cancelled as it started never ran, so it shows
// no duration, while one that ran shows how long, however short.
func TestTookOfWhatNeverRan(t *testing.T) {
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	st := NewRunStyles(NewTheme(p, true), NewIcons(config.IconsASCII))
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	tests := []struct {
		name       string
		conclusion core.Conclusion
		end        time.Time
		want       string
	}{
		{"cancelled while queued", core.ConclusionCancelled, start, ""},
		{"cancelled after a run", core.ConclusionCancelled, start.Add(5 * time.Second), "5s"},
		{"a quick success", core.ConclusionSuccess, start, "0s"},
		{"skipped", core.ConclusionSkipped, start, "skipped"},
	}
	for _, tt := range tests {
		got := ansi.Strip(st.Took(core.RunCompleted, tt.conclusion, start, tt.end, now))
		if got != tt.want {
			t.Errorf("%s: Took = %q, want %q", tt.name, got, tt.want)
		}
	}
	if got := st.Took(core.RunCompleted, core.ConclusionCancelled, time.Time{}, now, now); got != "" {
		t.Errorf("never started: Took = %q, want nothing", got)
	}
}
