package pager

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestViewDocs(t *testing.T) {
	const text = "a a\nb\na\nb a a a\n"
	tests := []struct {
		name   string
		width  int
		step   int
		search string
	}{
		{name: "second of three", width: 40, step: 1},
		{name: "narrow", width: 8, step: 2},
		{name: "with a search count", width: 50, search: "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "a.txt", text, WithSize(tt.width, 5))
			m.Add(Doc{Name: "b.txt", Text: text})
			m.Add(Doc{Name: "c.txt", Text: text})
			for range tt.step {
				m.Step(1)
			}
			if tt.search != "" {
				m, _ = keys(t, m, "/")
				m = typeText(t, m, tt.search)
				m, _ = keys(t, m, "enter")
			}
			v := m.View()
			assertFits(t, v, tt.width, 5)
			golden.RequireEqual(t, v)
		})
	}
}
