package tree

import (
	"errors"
	"slices"
	"testing"
)

func TestReveal(t *testing.T) {
	tests := []struct {
		name string
		path []string
		fail string
		// want is the node the cursor ends on, and open the branches
		// expanded on the way.
		want string
		open []string
	}{
		{"file", []string{"internal", "internal/core", "internal/core/pull.go"}, "", "internal/core/pull.go", []string{"internal", "internal/core"}},
		{"top level", []string{"go.mod"}, "", "go.mod", nil},
		{"branch", []string{"cmd", "cmd/gh-tui"}, "", "cmd/gh-tui", []string{"cmd"}},
		{"missing", []string{"internal", "internal/nope", "internal/nope/x.go"}, "", "internal", []string{"internal"}},
		{"failed load", []string{"internal", "internal/core", "internal/core/pull.go"}, "internal/core", "internal/core", []string{"internal", "internal/core"}},
		{"leaf on the way", []string{"go.mod", "go.mod/x"}, "", "go.mod", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := repo()
			if tt.fail != "" {
				f.setFail(tt.fail, errors.New("boom"))
			}
			m := load(t, f)
			m = run(t, m, m.Reveal(tt.path...))
			if got := selectedID(m); got != tt.want {
				t.Errorf("cursor on %q, want %q", got, tt.want)
			}
			for _, id := range tt.open {
				if !m.nodes[id].expanded {
					t.Errorf("%s isn't expanded", id)
				}
			}
			if m.goal != nil {
				t.Errorf("goal = %v after the walk ended", m.goal)
			}
			assertVisible(t, m)
		})
	}
}

// TestRevealWaitsForLoads reveals a node before the tree has loaded, and
// follows the loads as they arrive.
func TestRevealWaitsForLoads(t *testing.T) {
	f := repo()
	m := newModel(f.children, WithSize(40, 10), WithFocused(true))
	reveal := m.Reveal("internal", "internal/tui", "internal/tui/app.go")
	if reveal != nil || m.goal == nil {
		t.Fatal("Reveal didn't wait for the top-level nodes")
	}
	m = run(t, m, m.Init())
	if got := selectedID(m); got != "internal/tui/app.go" {
		t.Errorf("cursor on %q, want internal/tui/app.go", got)
	}
	want := []string{"cmd", "docs", "internal", "internal/core", "internal/tui", "internal/tui/app.go", "README.md", "go.mod"}
	if got := rowIDs(m); !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestRevealGivesUpOnKeys(t *testing.T) {
	f := repo()
	m := load(t, f)
	cmd := m.Reveal("internal", "internal/core", "internal/core/pull.go")
	if got := selectedID(m); got != "internal" {
		t.Fatalf("cursor on %q while internal loads, want internal", got)
	}
	// internal has no rows below it yet, so j moves past it.
	m = keys(t, m, "j")
	m = run(t, m, cmd)
	if got := selectedID(m); got != "README.md" {
		t.Errorf("cursor on %q, want where the key moved it", got)
	}
	if m.nodes["internal/core"].expanded {
		t.Error("the reveal went on after a key")
	}
}

func TestResetForgetsReveal(t *testing.T) {
	m := load(t, repo())
	_ = m.Reveal("cmd", "cmd/gh-tui", "cmd/gh-tui/main.go")
	cmd := m.Reset()
	if m.goal != nil {
		t.Fatal("Reset kept the reveal")
	}
	m = run(t, m, cmd)
	if got := selectedID(m); got != "cmd" {
		t.Errorf("cursor on %q after Reset, want the first node", got)
	}
}
