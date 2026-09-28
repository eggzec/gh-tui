package history

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestOnlineRetriesOnce checks that GitHub answering again reads once
// what failed for want of an answer: the branches, the history and the
// commit, which it reads again although it was asked for before. What
// GitHub refused, or what loaded, costs nothing.
func TestOnlineRetriesOnce(t *testing.T) {
	offline := fmt.Errorf("github: GET: %w", core.ErrOffline)
	refused := fmt.Errorf("github: 403 Forbidden: %w", core.ErrForbidden)
	tests := []struct {
		name string
		errs map[string]error
		want []string
	}{
		{
			"branches and commit offline",
			map[string]error{"branches": offline, "commit " + short(main0): offline},
			[]string{"branches", "commit " + short(main0)},
		},
		{"history offline", map[string]error{"commits main": offline}, []string{"commits main"}},
		{"refused", map[string]error{"branches": refused, "commit " + short(main0): refused}, nil},
		{"loaded", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			maps.Copy(f.errs, tt.errs)
			m, h := newModal(t, f, 108, 30)
			f.mu.Lock()
			clear(f.errs)
			f.mu.Unlock()
			f.took()

			h.run(func() tea.Msg { return ui.OnlineMsg{} })
			h.run(func() tea.Msg { return ui.OnlineMsg{} })
			// Once the history loads, the commit under the cursor and the
			// comparisons are read as always; only what failed counts.
			var got []string
			for _, c := range f.took() {
				if tt.errs[c] != nil {
					got = append(got, c)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("reads after two OnlineMsg = %q, want %q", got, tt.want)
			}
			if tt.want != nil && (m.branches.err != nil || m.graph.model.Err() != nil || m.commit.err != nil || !m.commit.loaded) {
				t.Errorf("once online: branches %v, history %v, commit %v, loaded %v; want them all",
					m.branches.err, m.graph.model.Err(), m.commit.err, m.commit.loaded)
			}
		})
	}
}
