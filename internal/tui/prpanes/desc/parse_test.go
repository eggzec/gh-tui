package desc

import (
	"fmt"
	"strings"
	"testing"
)

// shape draws nodes as text: [text], {id summary: nodes}.
func shape(nodes []Node) string {
	var b strings.Builder
	for _, n := range nodes {
		if n.Block == nil {
			fmt.Fprintf(&b, "[%s]", strings.TrimSpace(n.Text))
			continue
		}
		fmt.Fprintf(&b, "{%s %s: %s}", n.Block.ID, n.Block.Summary, shape(n.Block.Nodes))
	}
	return b.String()
}

func TestParse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, body, want string }{
		{"plain", "hello\n\nworld", "[hello\n\nworld]"},
		{"empty", "", ""},
		{"blank", " \n\n ", ""},
		{"block", "a\n<details>\n<summary>Sum</summary>\nb\n</details>\nc", "[a]{d0 Sum: [b]}[c]"},
		{"no summary", "<details>\nb\n</details>", "{d0 : [b]}"},
		{"upper case", "<DETAILS><SUMMARY>S</SUMMARY>b</DETAILS>", "{d0 S: [b]}"},
		{"attributes", "<details open><summary class=\"x\">S</summary>b</details>", "{d0 S: [b]}"},
		{"summary tags", "<details><summary><b>Big</b> &amp; <i>small</i>\n  one </summary>b</details>", "{d0 Big & small one: [b]}"},
		{"same line", "x<details><summary>S</summary>y</details>z", "[x]{d0 S: [y]}[z]"},
		{"two", "<details>a</details><details>b</details>", "{d0 : [a]}{d1 : [b]}"},
		{
			"nested", "<details><summary>A</summary>x<details><summary>B</summary>y</details><details>z</details></details>",
			"{d0 A: [x]{d0.0 B: [y]}{d0.1 : [z]}}",
		},
		{"unclosed", "a\n<details><summary>S</summary>b\nc", "[a]{d0 S: [b\nc]}"},
		{"unclosed nested", "<details>a<details>b", "{d0 : [a]{d0.0 : [b]}}"},
		{"stray close", "a</details>b", "[a][b]"},
		{"summary not first", "<details>x<summary>S</summary></details>", "{d0 : [x<summary>S</summary>]}"},
		{"unclosed summary", "<details><summary>S\nb</details>", "{d0 : [<summary>S\nb]}"},
		{
			"in a fence", "```\n<details>\n<summary>S</summary>\n```\ntext",
			"[```\n<details>\n<summary>S</summary>\n```\ntext]",
		},
		{"in a tilde fence", "~~~md\n<details>\n~~~\n<details>a</details>", "[~~~md\n<details>\n~~~]{d0 : [a]}"},
		{"long fence", "````\n```\n<details>\n````\n<details>a", "[````\n```\n<details>\n````]{d0 : [a]}"},
		{"fence never closed", "<details>a\n```\n</details>\nb", "{d0 : [a\n```\n</details>\nb]}"},
		{"fence in block", "<details>\n```\n</details>\n```\n</details>c", "{d0 : [```\n</details>\n```]}[c]"},
		{"backtick info", "```a`b\n<details>a</details>", "[```a`b]{d0 : [a]}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shape(Parse(tc.body)); got != tc.want {
				t.Errorf("Parse(%q)\n got %q\nwant %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestParseDeep(t *testing.T) {
	t.Parallel()
	n := 3 * maxDepth
	body := strings.Repeat("<details><summary>s</summary>", n) + "text" + strings.Repeat("</details>", n) + "\nafter"
	nodes := Parse(body)
	if len(nodes) != 2 || nodes[1].Block != nil || !strings.Contains(nodes[1].Text, "after") {
		t.Fatalf("the text after the blocks is lost: %q", shape(nodes))
	}
	depth := 0
	for b := nodes[0].Block; b != nil; depth++ {
		var next *Block
		for _, c := range b.Nodes {
			if c.Block != nil {
				next = c.Block
			}
		}
		b = next
	}
	if depth != maxDepth {
		t.Errorf("nesting is %d deep, want it cut at %d", depth, maxDepth)
	}
}

func TestParseManyTags(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("<details><summary>", 20000)
	if got := len(Parse(body)); got == 0 {
		t.Error("a body of tags parsed to nothing")
	}
}
