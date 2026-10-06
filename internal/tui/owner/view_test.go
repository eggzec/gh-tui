package owner

import (
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// The page of a user and of an organization, wide and narrow, in the
// unicode and ASCII icon sets, with images off.
func TestView(t *testing.T) {
	tests := []struct {
		name          string
		login         string
		width, height int
		icons         string
		keys          []string
	}{
		{"user 120x40 unicode", "octocat", 120, 40, config.IconsUnicode, nil},
		{"user 120x40 ascii", "octocat", 120, 40, config.IconsASCII, nil},
		{"user 80x24 unicode", "octocat", 80, 24, config.IconsUnicode, nil},
		{"user 80x24 ascii", "octocat", 80, 24, config.IconsASCII, nil},
		{"org 120x40 unicode", "github", 120, 40, config.IconsUnicode, nil},
		{"org 120x40 ascii", "github", 120, 40, config.IconsASCII, nil},
		{"org 80x24 unicode", "github", 80, 24, config.IconsUnicode, nil},
		{"org 80x24 ascii", "github", 80, 24, config.IconsASCII, nil},
		{"user 80x24 pinned", "octocat", 80, 24, config.IconsUnicode, []string{"1", "right"}},
		{"user 120x40 zoomed", "octocat", 120, 40, config.IconsUnicode, []string{"z", "down"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), tt.login, tt.width, tt.height, WithIcons(ui.NewIcons(tt.icons)))
			press(t, s, tt.keys...)
			golden.RequireEqual(t, s.View())
		})
	}
}

func TestViewStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		s := New(t.Context(), newFake(), config.Default().Keys, WithNow(func() time.Time { return now }))
		s.SetSize(80, 24)
		s.Update(ui.OwnerMsg{Login: "octocat"})
		s.Focus()
		golden.RequireEqual(t, s.View())
	})
	t.Run("empty", func(t *testing.T) {
		svc := newFake()
		o := user()
		o.Pinned = nil
		svc.owners["octocat"] = o
		svc.repos["octocat"] = nil
		s := newSection(t, svc, "octocat", 120, 40)
		golden.RequireEqual(t, s.View())
	})
	t.Run("failed", func(t *testing.T) {
		svc := newFake()
		svc.fail["header"] = errors.New("github: decode: unexpected EOF")
		s := newSection(t, svc, "octocat", 120, 40)
		golden.RequireEqual(t, s.View())
	})
	t.Run("repositories failed", func(t *testing.T) {
		svc := newFake()
		svc.fail["repos"] = core.ErrRateLimited
		s := newSection(t, svc, "octocat", 80, 24)
		golden.RequireEqual(t, s.View())
	})
	t.Run("offline", func(t *testing.T) {
		svc := newFake()
		svc.offline = true
		s := newSection(t, svc, "octocat", 120, 40)
		golden.RequireEqual(t, s.View())
	})
	t.Run("filtered", func(t *testing.T) {
		s := newSection(t, newFake(), "octocat", 80, 24)
		run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go sort:stars-desc"}))
		golden.RequireEqual(t, s.View())
	})
	// GitHub hid some pins from the token, or all of them.
	t.Run("some pins hidden", func(t *testing.T) {
		svc := newFake()
		o := user()
		o.Pinned, o.HiddenPins = o.Pinned[:2], true
		svc.owners["octocat"] = o
		s := newSection(t, svc, "octocat", 80, 24)
		press(t, s, "1")
		golden.RequireEqual(t, s.View())
	})
	t.Run("every pin hidden", func(t *testing.T) {
		svc := newFake()
		o := user()
		o.Pinned, o.HiddenPins = nil, true
		svc.owners["octocat"] = o
		s := newSection(t, svc, "octocat", 80, 24)
		press(t, s, "1")
		golden.RequireEqual(t, s.View())
	})
}
