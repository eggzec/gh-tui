package markdown

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFootnotes(t *testing.T) {
	tests := []struct {
		name, src string
		want      []string
		hidden    []string
	}{
		{
			name:   "numbered by first reference, at the end",
			src:    "Here[^b] and there[^A].\n\n[^a]: First one.\n[^b]: Second,\n  on two lines.\n\nAfter.",
			want:   []string{"Here¹ and there².", "After.", "-----", "1. Second,\non two lines.", "2. First one."},
			hidden: []string{"[^"},
		},
		{
			name: "a reference without a definition stays",
			src:  "Missing[^x] here.",
			want: []string{"Missing[^x] here."},
		},
		{
			name:   "an unreferenced definition doesn't show",
			src:    "Text.\n\n[^1]: Nobody asks.",
			want:   []string{"Text."},
			hidden: []string{"Nobody asks", "-----"},
		},
		{
			name:   "the first of two definitions wins",
			src:    "See[^1].\n\n[^1]: Kept.\n[^1]: Dropped.",
			want:   []string{"See¹.", "1. Kept."},
			hidden: []string{"Dropped"},
		},
		{
			name: "code and escapes are left as they are",
			src:  "In `a[^1]` and \\[^1] but[^1].\n\n```\n[^1]\n```\n\n[^1]: Note.",
			want: []string{"a[^1]", "[^1] but¹.", "1. Note."},
		},
		{
			name: "an escaped backslash leaves the reference",
			src:  "Path\\\\[^1] here.\n\n[^1]: Note.",
			want: []string{"Path\\¹ here.", "1. Note."},
		},
		{
			name: "one footnote refers to another",
			src:  "Text[^1].\n\n[^1]: See[^2].\n[^2]: Deeper.",
			want: []string{"Text¹.", "1. See².", "2. Deeper."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ansi.Strip(New(DefaultStyle(true)).Render(tt.src, 60))
			norm := strings.Join(strings.Fields(strings.ReplaceAll(out, "\n", " \n ")), " ")
			for _, w := range tt.want {
				wn := strings.Join(strings.Fields(strings.ReplaceAll(w, "\n", " \n ")), " ")
				if !strings.Contains(norm, wn) {
					t.Errorf("the render lacks %q:\n%s", w, out)
				}
			}
			for _, h := range tt.hidden {
				if strings.Contains(out, h) {
					t.Errorf("the render shows %q:\n%s", h, out)
				}
			}
		})
	}
}

// A definition ends at a line that starts another block, which stays
// where it was.
func TestFootnoteDefinitionEnds(t *testing.T) {
	for _, next := range []string{"# Heading", "- item", "1. item", "> quoted", "```"} {
		lines := []string{"Text[^1].", "", "[^1]: Note,", "still the note.", next}
		ps := make([]piece, 0, len(lines))
		for _, l := range lines {
			ps = append(ps, piece{line: l, text: true})
		}
		out := footnotes(ps)
		got := make([]string, 0, len(out))
		for _, p := range out {
			got = append(got, p.line)
		}
		want := []string{"Text¹.", "", next, "", "---", "", "1. Note,", "   still the note."}
		if !slices.Equal(got, want) {
			t.Errorf("after %q: lines = %q, want %q", next, got, want)
		}
	}
}
