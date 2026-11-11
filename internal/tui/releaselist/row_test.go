package releaselist

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestRowColumns(t *testing.T) {
	t.Parallel()
	s := started(t, fakeService{})
	r := core.Release{
		Tag: "v1.2.0", Name: "First stable", Latest: true,
		Author:      core.User{Login: "octocat"},
		Assets:      make([]core.ReleaseAsset, 7),
		PublishedAt: time.Now().Add(-72 * time.Hour),
	}
	tests := []struct {
		width         int
		author, files bool
		latest        bool
	}{
		{100, true, true, true},
		{50, false, true, true},
		{40, false, false, false},
		{32, false, false, false},
	}
	for _, tt := range tests {
		got := ansi.Strip(s.render(r, false, tt.width))
		if w := ansi.StringWidth(got); w > tt.width {
			t.Errorf("width %d: row is %d wide: %q", tt.width, w, got)
		}
		for _, c := range []struct {
			name string
			want bool
			text string
		}{
			{"author", tt.author, "octocat"},
			{"files", tt.files, "7"},
			{"latest", tt.latest, "latest"},
			{"tag", true, "v1.2.0"},
		} {
			if has := strings.Contains(got, c.text); has != c.want {
				t.Errorf("width %d: %s shown = %v, want %v: %q", tt.width, c.name, has, c.want, got)
			}
		}
	}
}
