package markdown

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// underlinedCells returns, for each line of raw, the text drawn with the
// underline on and whether the underline is still on at the end of the line.
// It reads the SGR sequences themselves, so a reset that is missing shows.
func underlinedCells(raw string) (drawn []string, open []bool) {
	on := false
	for line := range strings.SplitSeq(raw, "\n") {
		var b strings.Builder
		for i := 0; i < len(line); {
			if line[i] != 0x1b {
				r, n := utf8.DecodeRuneInString(line[i:])
				if on {
					b.WriteRune(r)
				}
				i += n
				continue
			}
			j := i + 1
			if j < len(line) && line[j] == ']' {
				for j < len(line) && line[j] != 0x07 && (line[j] != 0x1b || j+1 >= len(line) || line[j+1] != '\\') {
					j++
				}
				if j < len(line) && line[j] == 0x1b {
					j++
				}
				i = j + 1
				continue
			}
			j++
			for j < len(line) && (line[j] < '@' || line[j] > '~') {
				j++
			}
			if j >= len(line) {
				break
			}
			if line[j] == 'm' {
				on = underlineAfter(line[i+2:j], on)
			}
			i = j + 1
		}
		drawn = append(drawn, b.String())
		open = append(open, on)
	}
	return drawn, open
}

// underlineAfter returns whether the underline is on after the SGR
// parameters params, given whether it was on before.
func underlineAfter(params string, on bool) bool {
	if params == "" {
		return false
	}
	for p := range strings.SplitSeq(params, ";") {
		n, _, _ := strings.Cut(p, ":")
		switch c, _ := strconv.Atoi(n); c {
		case 0, 24:
			on = false
		case 4:
			on = !strings.HasSuffix(p, ":0")
		}
	}
	return on
}

// The underline of a link ends with the text of the link: it is off at the
// end of every line, and no text but the link's is drawn with it.
func TestRenderUnderlineDoesNotSpill(t *testing.T) {
	// Serial: links are a process-wide setting.
	tests := []struct {
		name, src string
		width     int
		want      string
	}{
		{"hash", "Fixes [`a1b2c3d`](https://example.com/c/a1b2c3d) now.", 80, "a1b2c3d"},
		{"wrapped", "Read the [very long documentation page of the project](https://example.com/d) now and later.", 20, "very long documentation page of the project"},
		{"list", "- see [a1b2c3d](https://example.com/c/a1b2c3d)\n- next item", 40, "a1b2c3d"},
		{"quote", "> see [a1b2c3d](https://example.com/c/a1b2c3d)\n> next line", 40, "a1b2c3d"},
		{"heading", "# Title [a1b2c3d](https://example.com/c/a1b2c3d) end", 40, "a1b2c3d"},
		{"autolink", "see https://example.com/c/a1b2c3d and more text", 40, "https://example.com/c/a1b2c3d"},
		{"emphasis", "[*a1b2c3d* x](https://example.com/c/a1b2c3d) tail", 40, "a1b2c3d x"},
		{"bold", "**[a1b2c3d](https://example.com/c/a1b2c3d)** then plain", 40, "a1b2c3d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Links are a process-wide setting, so the render is made
			// by a renderer that asks for them.
			linksOn(t, true)
			raw := New(DefaultStyle(true)).Render(tt.src, tt.width)
			drawn, open := underlinedCells(raw)
			var got []string
			for i, d := range drawn {
				got = append(got, strings.Fields(d)...)
				if open[i] {
					t.Errorf("line %d ends with the underline on: %q", i, raw)
				}
			}
			if g := strings.Join(got, " "); g != tt.want {
				t.Errorf("underlined %q, want %q: %q", g, tt.want, raw)
			}
		})
	}
}
