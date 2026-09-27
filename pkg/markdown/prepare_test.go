package markdown

import (
	"slices"
	"strings"
	"testing"
)

func TestPrepare(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "plain text is kept", in: "Hello **world**\nsecond line", want: "Hello **world**\nsecond line"},
		{name: "crlf", in: "a\r\nb\r\n", want: "a\nb\n"},
		{name: "tabs", in: "a\tb", want: "a   b"},
		{name: "escape sequences", in: "\x1b[31mred\x1b]0;title\x07", want: "�[31mred�]0;title�"},
		{name: "c1 controls", in: "a\u009bb", want: "a�b"},
		{name: "invalid utf-8", in: "a\xffb", want: "a�b"},
		{name: "comment on a line", in: "a\n<!-- hidden -->\nb", want: "a\n\nb"},
		{name: "comment in text", in: "a <!-- hidden --> b", want: "a  b"},
		{
			name: "comment over lines",
			in:   "<!--\nPlease describe\n```\nnot code\n-->\n## Summary",
			want: "\n\n\n\n\n## Summary",
		},
		{name: "comment in code span", in: "use `<!-- x -->` to hide", want: "use `<!-- x -->` to hide"},
		{name: "unclosed comment", in: "a\n<!-- to the end\nb", want: "a\n\n"},
		{
			name: "details",
			in:   "<details>\n<summary>Logs</summary>\n\nbody\n</details>",
			want: "\n**▸ Logs**\n\nbody\n",
		},
		{
			name: "details on one line",
			in:   "<details><summary> Logs </summary>",
			want: "**▸ Logs**",
		},
		{name: "empty summary", in: "<summary></summary>", want: "**▸ Details**"},
		{name: "summary over lines", in: "<summary>\nLogs\n</summary>", want: "▸ \nLogs\n"},
		{
			name: "img",
			in:   `<img width="200" alt="the [shot]" src="https://x.test/a.png">`,
			want: `![the \[shot\]](<https://x.test/a.png>)`,
		},
		{name: "img without alt", in: `<img src='https://x.test/a.png' />`, want: "![image](<https://x.test/a.png>)"},
		{name: "img without src", in: `a <img alt="x"> b`, want: "a  b"},
		{
			name: "img in a paragraph tag",
			in:   `<p align="center"><img src=https://x.test/a.png alt=logo></p>`,
			want: "![logo](<https://x.test/a.png>)",
		},
		{name: "image without text", in: "![](https://x.test/a.png)", want: "![image](https://x.test/a.png)"},
		{name: "img in code span", in: "`<img src=x>`", want: "`<img src=x>`"},
		{name: "other html is left to glamour", in: "a <kbd>b</kbd> < c", want: "a <kbd>b</kbd> < c"},
		{
			name: "fenced code is untouched",
			in:   "```html\n<!-- c -->\n<img src=x>\n<details>\n```\n<!-- gone -->",
			want: "```html\n<!-- c -->\n<img src=x>\n<details>\n```\n",
		},
		{
			name: "tilde fence and a longer close",
			in:   "~~~\n<!-- c -->\n~~~~\n<!-- gone -->",
			want: "~~~text\n<!-- c -->\n~~~\n",
		},
		{name: "unclosed fence runs to the end", in: "```\n<!-- c -->", want: "```text\n<!-- c -->\n```"},
		{name: "indented fence in a list", in: "- a\n  ```\n  <img src=x>\n  ```", want: "- a\n  ```text\n  <img src=x>\n  ```"},
		{name: "fence in a comment is hidden", in: "<!--\n```\n-->\nb", want: "\n\n\nb"},
		{name: "summary with its body on the line", in: "<details><summary>x</summary>body", want: "**▸ x**\n\nbody"},
		{name: "linked image", in: "[![build](https://x.test/b.svg)](https://x.test/ci)", want: "[🖼 build](https://x.test/ci)"},
		{name: "linked image without text", in: "[![](https://x.test/b.svg)](<https://x.test/ci>)", want: "[🖼 image](<https://x.test/ci>)"},
		{name: "link named x in a list", in: "- [X](https://x.com/share)\n1. [x](y)", want: "- [\u200bX](https://x.com/share)\n1. [\u200bx](y)"},
		{name: "task in a list", in: "- [x] done", want: "- [x] done"},
		{name: "link named x in a quoted list", in: "> - [ ](u)", want: "> - [\u200b ](u)"},
		{name: "alert", in: "> [!NOTE]\n> text", want: "> **ℹ Note**\n> text"},
		{name: "alert in any case", in: ">[!warning] ", want: ">**⚠ Warning**"},
		{name: "unknown alert", in: "> [!FOO]", want: "> [!FOO]"},
		{name: "alert in code", in: "```\n> [!NOTE]\n```", want: "```text\n> [!NOTE]\n```"},
		{name: "bidi overrides", in: "a\u202eb", want: "a�b"},
		{name: "a known language is kept", in: "~~~~ Go title\nx\n~~~~", want: "~~~~go\nx\n~~~~"},
		{name: "a language that can hang shows plain", in: "```jsonata\n\\\n```", want: "```text\n\\\n```"},
		{name: "long code shows plain in blocks", in: "```go\n" + strings.Repeat("x\n", 401) + "```", want: "```text\n" + strings.Repeat("x\n", 400) + "```\n```text\nx\n```"},
		{name: "long sources are cut", in: strings.Repeat("x\n", 1500), want: strings.Repeat("x\n", 1000) + "\n*⋯ The rest is too long to show here*"},
		{name: "wide tables are text", in: strings.Repeat("|a", 65), want: strings.Repeat("\\|a", 65)},
		{name: "tables up to the limit stay", in: strings.Repeat("|a", 64), want: strings.Repeat("|a", 64)},
		{name: "deep quotes", in: strings.Repeat("> ", 12) + "x", want: strings.Repeat("> ", 4) + "x"},
		{name: "deep lists", in: strings.Repeat("- ", 12) + "x", want: strings.Repeat("- ", 4) + "x"},
		{name: "deep quotes and lists", in: strings.Repeat("> > 1. ", 6) + "x", want: strings.Repeat("> > 1. ", 2) + strings.Repeat("1. ", 2) + "x"},
		{name: "long line", in: "*[a](" + strings.Repeat("b", maxLine), want: "\\*\\[a\\]\\(" + strings.Repeat("b", maxLine)},
		{name: "long list item", in: "> - `" + strings.Repeat("b", maxLine), want: "> - \\`" + strings.Repeat("b", maxLine)},
		{name: "deep indentation", in: strings.Repeat(" ", 60) + "- x", want: strings.Repeat(" ", 24) + "- x"},
		{name: "references to controls", in: "a&#27;[2J &#x1B b &#0; &#x202e; &rlm; &#65; &amp;", want: "a�[2J � b � � � &#65; &amp;"},
		{name: "references in a link", in: "[x&#27;y](https://x.test)", want: "[x�y](https://x.test)"},
		{name: "alert in indented code", in: "text\n\n    > [!NOTE]", want: "text\n\n    > [!NOTE]"},
		{name: "link item in indented code", in: "    - [x](u)", want: "    - [x](u)"},
		{name: "link item in pre", in: "<pre>\n- [x](u)\n</pre>", want: "<pre>\n- [x](u)\n</pre>"},
		{name: "nested link item in a list", in: "- a\n\n    - [x](u)", want: "- a\n\n    - [\u200bx](u)"},
		{name: "mermaid shows as plain code", in: "```mermaid\ngraph TD\n  A-->B\n```", want: "```text\ngraph TD\n  A-->B\n```"},
		{name: "code keeps its escapes neutralised", in: "```\n\x1b[2J\n```", want: "```text\n�[2J\n```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prepare(tt.in, nil, ""); got != tt.want {
				t.Errorf("prepare(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

// Past maxTableCells, the rows of a table show as text after a blank line
// that ends the table.
func TestTableCellsAreBounded(t *testing.T) {
	row := "|" + strings.Repeat("a|", 15)
	kept := maxTableCells / 16
	got := strings.Split(prepare(lines(kept+2, row), nil, ""), "\n")
	text := strings.ReplaceAll(row, "|", `\|`)
	want := append(strings.Split(lines(kept, row), "\n"), "", text, text)
	if !slices.Equal(got, want) {
		t.Errorf("prepare ends %q, want %q", got[kept-1:], want[kept-1:])
	}
}

func TestOpenFence(t *testing.T) {
	tests := []struct {
		line string
		ok   bool
		lang string
	}{
		{"```", true, ""},
		{"```Go title", true, "go"},
		{"   ~~~mermaid", true, "mermaid"},
		{"``", false, ""},
		{"``` a`b", false, ""},
		{"text ```", false, ""},
	}
	for _, tt := range tests {
		f, ok := openFence(tt.line)
		if ok != tt.ok || f.lang != tt.lang {
			t.Errorf("openFence(%q) = %q, %v; want %q, %v", tt.line, f.lang, ok, tt.lang, tt.ok)
		}
	}
}
