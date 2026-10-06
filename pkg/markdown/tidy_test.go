package markdown

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestTidy(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "plain padding", in: "text   ", want: "text"},
		{name: "styled padding", in: "\x1b[38;5;252mtext\x1b[m\x1b[38;5;252m \x1b[m\x1b[38;5;252m \x1b[m", want: "\x1b[38;5;252mtext\x1b[m"},
		{
			name: "runs of one style join",
			in:   "\x1b[38;5;252mSteps to\x1b[m\x1b[38;5;252m reproduce\x1b[m",
			want: "\x1b[38;5;252mSteps to reproduce\x1b[m",
		},
		{
			name: "a new style resets",
			in:   "\x1b[38;5;252ma\x1b[m\x1b[38;5;39mb\x1b[m",
			want: "\x1b[38;5;252ma\x1b[m\x1b[38;5;39mb\x1b[m",
		},
		{
			name: "a style on top is added",
			in:   "\x1b[38;5;252ma\x1b[1mb\x1b[m",
			want: "\x1b[38;5;252ma\x1b[1mb\x1b[m",
		},
		{
			name: "a background shows its spaces",
			in:   "\x1b[1;48;5;63m Title \x1b[m\x1b[38;5;252m   \x1b[m",
			want: "\x1b[1;48;5;63m Title \x1b[m",
		},
		{name: "a 256 color that looks like a background", in: "\x1b[38;5;41ma \x1b[m", want: "\x1b[38;5;41ma\x1b[m"},
		{name: "reverse video shows spaces", in: "\x1b[7m  \x1b[m  ", want: "\x1b[7m  \x1b[m"},
		{
			name: "hyperlinks go",
			in:   "\x1b]8;id=1;https://x.test\x07docs\x1b]8;;\x07 \x1b]8;;https://y.test\x1b\\y\x1b]8;;\x1b\\",
			want: "docs y",
		},
		{name: "other sequences go", in: "a\x1b[2Jb\x1b[H", want: "ab"},
		{
			name: "spaces keep the style they follow",
			in:   "\x1b[38;5;35mdocs\x1b[m\x1b[38;5;252m \x1b[m\x1b[38;5;30;4murl\x1b[m",
			want: "\x1b[38;5;35mdocs \x1b[m\x1b[38;5;30;4murl\x1b[m",
		},
		{
			name: "leading spaces need no style",
			in:   "\x1b[38;5;252m  \x1b[m\x1b[38;5;39mfunc\x1b[m",
			want: "  \x1b[38;5;39mfunc\x1b[m",
		},
		{
			name: "an underline shows on spaces",
			in:   "a\x1b[4m b\x1b[m",
			want: "a\x1b[4m b\x1b[m",
		},
		{
			name: "spaces after an underline are drawn without it",
			in:   "\x1b[4ma\x1b[m\x1b[38;5;1m \x1b[mb",
			want: "\x1b[4ma\x1b[m b",
		},
		{name: "only padding", in: "\x1b[38;5;252m   \x1b[m", want: ""},
		{name: "reset with a style", in: "\x1b[1ma\x1b[0;38;5;1mb\x1b[0m", want: "\x1b[1ma\x1b[m\x1b[0;38;5;1mb\x1b[m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tidy(tt.in); got != tt.want {
				t.Errorf("tidy(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

// A quote's lines fit the width: glamour counts its indent as two cells,
// and the bar and the space after it take two.
func TestQuotesFitTheWidth(t *testing.T) {
	r := New(DefaultStyle(true))
	src := "> This is a long quoted line that should wrap across several lines of the view at forty cells.\n>\n> > nested quote with enough words to wrap as well, surely at this width."
	for _, width := range []int{20, 30, 40, 41, 80} {
		out := r.Render(src, width)
		for l := range strings.SplitSeq(out, "\n") {
			if w := xansi.StringWidth(l); w > width {
				t.Errorf("at %d cells a line is %d wide: %q", width, w, xansi.Strip(l))
			}
			if p := xansi.Strip(l); p != "" && !strings.HasPrefix(p, "│ ") && p != "│" {
				t.Errorf("at %d cells a line of the quote is %q, want it behind its bar", width, p)
			}
		}
		if !strings.Contains(xansi.Strip(out), "│ │ nested") {
			t.Errorf("at %d cells the nested quote reads\n%s", width, xansi.Strip(out))
		}
	}
}

// A quote of any shape fits the width with each of its lines behind its
// bar: nested deep, holding a list, a heading, code, a table or an alert,
// inside a list item, or holding words too long to wrap. A line glamour
// draws a cell too wide is wrapped again, so its last word would end up
// on a line of its own, outside the quote.
func TestQuotesOfEveryShapeFitTheWidth(t *testing.T) {
	words := strings.Repeat("lorem ipsum dolor sit amet consectetur adipiscing elit ", 6)
	srcs := map[string]string{
		"nested twice":       "> > " + words,
		"nested four deep":   "> > > > " + words,
		"nested and back":    "> a\n>\n> > " + words + "\n>\n> back " + words,
		"holding a list":     "> - " + words + "\n> - " + words,
		"holding a task":     "> - [ ] " + words,
		"holding numbers":    "> 1. " + words,
		"in a list item":     "- item\n\n  > " + words,
		"holding a heading":  "> # " + words,
		"holding an address": "> # https://example.com/" + strings.Repeat("path/", 30),
		"holding code":       "> ```\n> " + words + "\n> ```",
		"holding a table":    "> | a | b |\n> |---|---|\n> | " + words + " | x |",
		"an alert":           "> [!NOTE]\n> " + words,
		"broken by hand":     "> " + strings.ReplaceAll(words, " ", "\n> "),
		"emphasis":           "> **" + words + "** _" + words + "_",
		"one long word":      "> " + strings.Repeat("x", 300),
		"wide characters":    "> " + strings.Repeat("漢字かな ", 40),
	}
	r := New(DefaultStyle(true))
	for name, src := range srcs {
		for _, width := range []int{6, 7, 12, 19, 20, 33, 40, 41, 79, 80, 120} {
			for l := range strings.SplitSeq(r.Render(src, width), "\n") {
				if w := xansi.StringWidth(l); w > width {
					t.Errorf("%s at %d cells: a line is %d wide: %q", name, width, w, xansi.Strip(l))
				}
				p := strings.TrimLeft(xansi.Strip(l), " ")
				// The list item's own line comes before its quote.
				item := name == "in a list item" && p == "• item"
				if p != "" && !item && !strings.HasPrefix(p, "│") {
					t.Errorf("%s at %d cells: a line of the quote is %q, want it behind its bar", name, width, p)
				}
			}
		}
	}
}

func TestQuoteBars(t *testing.T) {
	q := quoteToken
	tests := []struct{ in, want string }{
		{q + q + " text", "│  text"},
		{"  " + q + q + " " + q + q + " nested", "  │  │  nested"},
		{"\x1b[38;5;252m" + q + "\x1b[m\x1b[38;5;252m" + q + "\x1b[m text", "\x1b[38;5;252m│\x1b[m\x1b[38;5;252m \x1b[m text"},
		{q + q + "│ starts with a bar", "│ │ starts with a bar"},
		{q + q + "││ a box", "│ ││ a box"},
		{"││ inner box │", "││ inner box │"},
		{"│ a table edge │", "│ a table edge │"},
		{"plain " + q + q + " inside", "plain " + q + q + " inside"},
		{q + q, "│ "},
	}
	for _, tt := range tests {
		if got := quoteBars(tt.in, Glyphs{}.orDefault()); got != tt.want {
			t.Errorf("quoteBars(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Only a quote's indent changes: bars in the text, in code or a table,
// inside a quote or not, stay as they are.
func TestQuoteBarsKeepText(t *testing.T) {
	r := New(DefaultStyle(true))
	tests := []struct {
		name, src string
		want      []string
	}{
		{"code outside a quote", "```\n││ inner box │\n```", []string{"││ inner box │"}},
		{"quoted line starting with a bar", "> │ starts with a bar", []string{"│ │ starts with a bar"}},
		{"quoted code", "> ```\n> │ x\n> ││ y\n> ```", []string{"│   │ x", "│   ││ y"}},
		{"three levels", "> one\n>\n> > two\n> >\n> > > three", []string{"│ one", "│ │ two", "│ │ │ three"}},
		{"marks in a paragraph", quoteToken + quoteToken + " text", []string{"││ text"}},
		{"marks in a quote", "> " + quoteToken + quoteToken + " x", []string{"│ ││ x"}},
		{"quoted table", "> | a | b |\n> |---|---|\n> | 1 | 2 |", []string{"│  a", "│ b", "│  1", "│ 2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := xansi.Strip(r.Render(tt.src, 40))
			if strings.Contains(out, "\u200b") {
				t.Errorf("a quote's mark is left in\n%s", out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("want %q in\n%s", w, out)
				}
			}
		})
	}
}
