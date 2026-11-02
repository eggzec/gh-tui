package panes

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestEdgesMatchFrame(t *testing.T) {
	t.Parallel()
	st := DefaultStyles(true)
	cases := map[string]Content{
		"all four":  {TopLeft: []string{"Title"}, TopRight: []string{"info"}, BottomLeft: []string{"hint"}, BottomRight: []string{"1/2"}},
		"focus":     {TopLeft: []string{"Title"}, BottomRight: []string{"1/2"}, Focus: true},
		"none":      {},
		"body only": {Body: "a\nb"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lines := strings.Split(st.Frame(c, 30, 5), "\n")
			top, bottom := st.Edges(c, 30)
			if top != lines[0] || bottom != lines[len(lines)-1] {
				t.Errorf("Edges = %q, %q; Frame has %q, %q", top, bottom, lines[0], lines[len(lines)-1])
			}
		})
	}
	if top, bottom := st.Edges(Content{}, 1); top != "" || bottom != "" {
		t.Errorf("Edges at width 1 = %q, %q, want empty", top, bottom)
	}
}

func TestSides(t *testing.T) {
	t.Parallel()
	st := DefaultStyles(true)
	for _, focus := range []bool{false, true} {
		l, r := st.Sides(focus)
		line := strings.Split(st.Frame(Content{Focus: focus, Body: "x"}, 5, 3), "\n")[1]
		if !strings.HasPrefix(line, l) || !strings.HasSuffix(line, r) {
			t.Errorf("focus %v: Sides %q %q are not the ends of %q", focus, l, r, line)
		}
		if ansi.StringWidth(l) != 1 || ansi.StringWidth(r) != 1 {
			t.Errorf("focus %v: sides are not one cell wide", focus)
		}
	}
}

func TestEdgesLinkedForm(t *testing.T) {
	t.Parallel()
	st := DefaultStyles(true)
	link := "\x1b]8;;https://example.com\x1b\\owner/repo\x1b]8;;\x1b\\"
	top, _ := st.Edges(Content{TopLeft: []string{link}, TopRight: []string{"info"}}, 30)
	if got := ansi.StringWidth(top); got != 30 {
		t.Errorf("top is %d cells wide, want 30", got)
	}
	if !strings.Contains(ansi.Strip(top), "owner/repo") || !strings.Contains(top, "https://example.com") {
		t.Errorf("top lost the linked title: %q", top)
	}
}

func TestHintForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		hints []Hint
		want  []string
	}{
		{"none", nil, nil},
		{"empty text ignored", []Hint{{"", 1}}, nil},
		{"one", []Hint{{"a", 1}}, []string{"a"}},
		{"lowest first", []Hint{{"a", 2}, {"b", 1}, {"c", 3}}, []string{"a b c", "a c", "c"}},
		{"ties drop the last", []Hint{{"a", 1}, {"b", 1}, {"c", 1}}, []string{"a b c", "a b", "a"}},
		{"order kept", []Hint{{"a", 1}, {"b", 5}, {"c", 1}, {"d", 5}}, []string{"a b c d", "a b d", "b d", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HintForms(tt.hints, " "); !slices.Equal(got, tt.want) {
				t.Errorf("HintForms = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPosition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		top, shown, total int
		more              bool
		want              []string
	}{
		{"all shown", 0, 10, 10, false, nil},
		{"more to load", 0, 10, 10, true, []string{"1-10/10+", "10/10+", "100%"}},
		{"middle", 10, 5, 40, false, []string{"11-15/40", "15/40", "37%"}},
		{"end", 35, 5, 40, false, []string{"36-40/40", "40/40", "100%"}},
		{"empty with more", 0, 0, 0, true, []string{"1-0/0+", "0/0+", "100%"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Position(tt.top, tt.shown, tt.total, tt.more); !slices.Equal(got, tt.want) {
				t.Errorf("Position = %q, want %q", got, tt.want)
			}
		})
	}
}
