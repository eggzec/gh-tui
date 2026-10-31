package feed

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLoading(t *testing.T) {
	t.Parallel()
	failing := newSource(10, 10)
	failing.setFail("", errors.New("boom"))
	fetching := newModel(newSource(10, 10).fetch, renderItem, WithSize(40, 5))
	tests := []struct {
		name string
		m    Model[item]
		want bool
	}{
		{"empty and fetching", fetching, true},
		{"empty and done", load(t, newSource(0, 5)), false},
		{"error", load(t, failing), false},
		{"rows present", load(t, newSource(10, 10)), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.m.Loading(); got != tt.want {
				t.Fatalf("Loading() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDimmed(t *testing.T) {
	t.Parallel()
	stale := lipgloss.NewStyle().Foreground(lipgloss.Color("#123456"))
	style := func(m Model[item]) Model[item] {
		s := DefaultStyles(true)
		s.Stale = stale
		m.SetStyles(s)
		return m
	}

	t.Run("nil without rows", func(t *testing.T) {
		t.Parallel()
		if got := load(t, newSource(0, 5)).Dimmed(); got != nil {
			t.Fatalf("Dimmed() = %q, want nil", got)
		}
	})

	check := func(t *testing.T, m Model[item]) {
		t.Helper()
		m = style(m)
		want := strings.Split(ansi.Strip(m.View()), "\n")
		got := m.Dimmed()
		if len(got) != len(want) {
			t.Fatalf("Dimmed() has %d lines, want %d", len(got), len(want))
		}
		for i, l := range got {
			if ansi.Strip(l) != want[i] {
				t.Fatalf("line %d = %q, want text %q", i, ansi.Strip(l), want[i])
			}
			if want[i] != "" && l != stale.Render(want[i]) {
				t.Fatalf("line %d = %q, want it drawn with Stale", i, l)
			}
		}
	}

	t.Run("plain lines drawn with Stale", func(t *testing.T) {
		t.Parallel()
		check(t, load(t, newSource(10, 10)))
	})
	t.Run("groups keep every line", func(t *testing.T) {
		t.Parallel()
		check(t, loadGrouped(t, newSource(6, 6), []Group[item]{{Key: parity}}, nil))
	})
}
