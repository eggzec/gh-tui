package markdown

import (
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	// fn marks each address it is given, an image's with img: and a
	// link's with link:, and leaves those on the web.
	fn := func(addr string, image bool) string {
		if strings.Contains(addr, "://") {
			return addr
		}
		if image {
			return "img:" + addr
		}
		return "link:" + addr
	}
	tests := []struct{ name, src, want string }{
		{"link", "See [the docs](docs/a.md).", "See [the docs](link:docs/a.md)."},
		{"image", "![logo](img/logo.png)", "![logo](img:img/logo.png)"},
		{"title", `[a](b.md "B") and ![c](d.png 'D')`, `[a](link:b.md "B") and ![c](img:d.png 'D')`},
		{"angle", "[a](<my file.md>)", "[a](<link:my file.md>)"},
		{"parentheses", "[a](a_(b).md)", "[a](link:a_(b).md)"},
		{"badge", "[![build](badge.svg)](actions)", "[![build](img:badge.svg)](link:actions)"},
		{"web", "[a](https://example.com/x)", "[a](https://example.com/x)"},
		{"reference", "[home]: ./home.md \"Home\"", "[home]: link:./home.md \"Home\""},
		{"html", `<p align="center"><img src="logo.png" width="80"> <a href='about.md'>x</a></p>`,
			`<p align="center"><img src="img:logo.png" width="80"> <a href='link:about.md'>x</a></p>`},
		{"code span", "`[a](b.md)` and [c](d.md)", "`[a](b.md)` and [c](link:d.md)"},
		{"double code span", "`` a ` [a](b.md) `` [c](d.md)", "`` a ` [a](b.md) `` [c](link:d.md)"},
		{"lone backtick", "a ` [c](d.md)", "a ` [c](link:d.md)"},
		{"fence", "```\n[a](b.md)\n```\n[c](d.md)", "```\n[a](b.md)\n```\n[c](link:d.md)"},
		{"tilde fence", "~~~~md\n<img src=\"x.png\">\n~~~\n~~~~\n![c](d.png)", "~~~~md\n<img src=\"x.png\">\n~~~\n~~~~\n![c](img:d.png)"},
		{"empty", "[a]() and text", "[a]() and text"},
		{"footnote", "[^1]: See [the docs](docs.md).", "[^1]: See [the docs](link:docs.md)."},
		{"prose", "[Note]: this is a sentence", "[Note]: this is a sentence"},
		{"reference titled", "[a]: <b c.md> (B)", "[a]: <link:b c.md> (B)"},
		{"quoted fence", "> ```\n> [a](b.md)\n> ```\n> [c](d.md)", "> ```\n> [a](b.md)\n> ```\n> [c](link:d.md)"},
		{"indented code", "text\n\n    [a](b.md)\n\n[c](d.md)", "text\n\n    [a](b.md)\n\n[c](link:d.md)"},
		{"list text", "- item\n\n    [a](b.md)", "- item\n\n    [a](link:b.md)"},
		{"indented continues", "text\n    [a](b.md)", "text\n    [a](link:b.md)"},
		{"pre", "<pre>\n<img src=\"x.png\">\n</pre>\n<img src=\"y.png\">", "<pre>\n<img src=\"x.png\">\n</pre>\n<img src=\"img:y.png\">"},
		{"code tag", "<code>[a](b.md)</code> [c](d.md)", "<code>[a](b.md)</code> [c](link:d.md)"},
		{"comment", "<!-- [a](b.md)\n<img src=\"x.png\"> -->[c](d.md)", "<!-- [a](b.md)\n<img src=\"x.png\"> -->[c](link:d.md)"},
		{"data attributes", `<img data-src="x.png" src="y.png"> <a data-href="z">`, `<img data-src="x.png" src="img:y.png"> <a data-href="z">`},
		{"unquoted", `<img src=logo.png width=80>`, `<img src=img:logo.png width=80>`},
		{"multi-line tag", "<img\n  alt=\"logo\"\n  src=\"logo.png\">", "<img\n  alt=\"logo\"\n  src=\"img:logo.png\">"},
		{"plain", "nothing to do here", "nothing to do here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.src, fn); got != tt.want {
				t.Errorf("Resolve(%q)\n got %q\nwant %q", tt.src, got, tt.want)
			}
		})
	}
}
