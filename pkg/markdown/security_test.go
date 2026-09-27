package markdown

import (
	"fmt"
	"regexp"
	"testing"
	"unicode/utf8"
)

// diagramLink matches the hyperlinks a head of a diagram opens and closes.
var diagramLink = regexp.MustCompile(`^\x1b\]8;;(?:https://mermaid\.live/view#pako:[A-Za-z0-9_-]+)?\x1b\\`)

// unsafe describes the first thing in out that isn't a style sequence of
// digits, semicolons and colons, a diagram's link or printable text, or
// returns "".
func unsafe(out string) string {
	for i := 0; i < len(out); {
		c := out[i]
		if m := diagramLink.FindString(out[i:]); m != "" {
			i += len(m)
			continue
		}
		if c == 0x1b {
			// Only CSI [0-9;:]* m allowed.
			if i+1 >= len(out) || out[i+1] != '[' {
				return fmt.Sprintf("non-CSI ESC at %d: %q", i, out[i:min(len(out), i+12)])
			}
			j := i + 2
			for j < len(out) && (out[j] >= '0' && out[j] <= '9' || out[j] == ';' || out[j] == ':') {
				j++
			}
			if j >= len(out) || out[j] != 'm' {
				return fmt.Sprintf("non-SGR CSI at %d: %q", i, out[i:min(len(out), i+16)])
			}
			i = j + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(out[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			return fmt.Sprintf("invalid UTF-8 at %d: %q", i, out[i:min(len(out), i+8)])
		case r == '\n':
		case r < 0x20 || r == 0x7f:
			return fmt.Sprintf("C0 control %U at %d", r, i)
		case r >= 0x80 && r < 0xa0:
			return fmt.Sprintf("C1 control %U at %d", r, i)
		case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200e, r == 0x200f, r == 0x61c:
			return fmt.Sprintf("bidi control %U at %d", r, i)
		}
		i += size
	}
	return ""
}

// payloads are what a comment could hold to take over the terminal: escape
// sequences, raw and as character references, which glamour decodes after
// the source was cleaned, controls, invalid UTF-8 and bidi overrides.
var payloads = []string{
	"\x1b]0;pwned\x07",
	"\x1b]52;c;cHduZWQ=\x07",
	"\x1b]8;;http://evil\x1b\\x\x1b]8;;\x1b\\",
	"\x1bP+q544e\x1b\\",
	"\x1b_apc\x1b\\",
	"\x1b[2J\x1b[H",
	"\x1b[>4;2m",
	"\x9b31m",
	"\xc2\x9b2J",
	"\xc2\x9d0;t\xc2\x9c",
	"\u202eevil\u202c",
	"\u2066x\u2069",
	"\xff\xfe\x80",
	"\r",
	"\x08\x07\x00",
	// entities that decode to controls
	"&#27;[2J",
	"&#x1b;]0;pwned&#x07;",
	"&#x1B;[>4;2m",
	"&#155;2J",
	"&#x9b;31m",
	"&#x9d;0;t&#x9c;",
	"&#8238;rtl",
	"&#x202E;rtl",
	"&#x2066;iso",
	"&#0;",
	"&#127;",
	"&#8;",
	"&#13;",
	"&#x85;", "&#9;x", "&Tab;x", "&#x8d;", "&#x90;q&#x9c;",
}

// contexts returns markdown with p in each place markdown can hold text.
func contexts(p string) []string {
	return []string{
		"plain " + p + " text",
		"# heading " + p,
		"## heading " + p,
		"**bold " + p + "** and _em " + p + "_",
		"`code " + p + "`",
		"```\n" + p + "\n```",
		"```go\nx := \"" + p + "\"\n```",
		"    indented " + p,
		"[link " + p + "](https://example.com/" + p + ")",
		"[link](<https://ex.com/" + p + ">)",
		"<https://ex.com/" + p + ">",
		"https://ex.com/" + p,
		"![alt " + p + "](https://ex.com/i.png)",
		"![alt](https://ex.com/" + p + ".png \"title " + p + "\")",
		"<img alt=\"" + p + "\" src=\"https://ex.com/" + p + "\">",
		"<img alt='" + p + "' src='x'>",
		"| a " + p + " | b |\n|---|---|\n| " + p + " | c |",
		"> quote " + p,
		"> [!NOTE]\n> " + p,
		"- item " + p + "\n  - nested " + p,
		"- [ ] task " + p,
		"1. " + p,
		"<details><summary>" + p + "</summary>\n\n" + p + "\n\n</details>",
		"<!-- " + p + " -->after",
		"<div title=\"" + p + "\">" + p + "</div>",
		"<span>" + p + "</span>",
		"<b>" + p + "</b>",
		"Footnote[^1]\n\n[^1]: " + p,
		"[ref " + p + "][r]\n\n[r]: https://ex.com/" + p + " \"t " + p + "\"",
		"~~strike " + p + "~~",
		"term\n: " + p,
		p,
		"a\\" + p,
		"---\n" + p + "\n---",
		"Setext " + p + "\n===",
		"<pre>" + p + "</pre>",
		"<code>" + p + "</code>",
		"<a href=\"" + p + "\">" + p + "</a>",
		":smile: " + p,
	}
}

// Whatever a comment holds, and wherever, the render only colors text.
func TestSecurityPayloads(t *testing.T) {
	for _, dark := range []bool{true, false} {
		r := New(DefaultStyle(dark))
		for _, p := range payloads {
			for _, src := range contexts(p) {
				for _, w := range []int{1, 5, 20, 76} {
					out := r.Render(src, w)
					if why := unsafe(out); why != "" {
						t.Errorf("dark=%v w=%d src=%q: %s\nout=%q", dark, w, src, why, out)
					}
				}
			}
		}
	}
}

// FuzzRenderSafe checks that no source makes the render do more than
// color text.
func FuzzRenderSafe(f *testing.F) {
	for _, p := range payloads {
		for _, c := range contexts(p) {
			f.Add(c, 40)
		}
	}
	r := New(DefaultStyle(true))
	f.Fuzz(func(t *testing.T, src string, w int) {
		w = w%200 + 1
		if w <= 0 {
			w = -w + 1
		}
		if len(src) > 4096 {
			return
		}
		out := r.Render(src, w)
		if why := unsafe(out); why != "" {
			t.Fatalf("src=%q w=%d: %s\nout=%q", src, w, why, out)
		}
	})
}
