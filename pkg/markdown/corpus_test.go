package markdown

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

// corpus holds bodies as GitHub has them.
var corpus = map[string]string{
	"template": `<!--
Thanks for opening a PR! Please read CONTRIBUTING.md.
-->

## Summary

<!-- Describe your change -->
Adds markdown rendering to comments.

## Checklist
<!-- Put an x in the boxes -->
- [x] Tests pass
- [ ] Docs updated <!-- if needed -->

Fixes #123`,
	"coderabbit": "<!-- This is an auto-generated comment: summarize by coderabbit.ai -->\n" +
		"<!-- walkthrough_start -->\n\n" +
		"## Walkthrough\n\n" +
		"The change adds a markdown package and uses it for comments.\n\n" +
		"## Changes\n\n" +
		"| Cohort / File(s) | Summary |\n" +
		"|---|---|\n" +
		"| **Markdown** <br> `pkg/markdown/markdown.go`, `pkg/markdown/prepare.go` | New renderer with a cache keyed by source, width and open blocks. |\n" +
		"| **Thread** <br> `pkg/bubbles/thread/model.go` | Uses the renderer. |\n\n" +
		"## Sequence Diagram(s)\n\n" +
		"```mermaid\nsequenceDiagram\n    participant U as User\n    participant T as Thread\n    U->>T: open\n    T-->>U: rendered\n```\n\n" +
		"## Estimated code review effort\n\n🎯 3 (Moderate) | ⏱️ ~25 minutes\n\n" +
		"<!-- walkthrough_end -->\n\n" +
		"<details>\n<summary>📜 Recent review details</summary>\n\n" +
		"**Configuration used**: CodeRabbit UI\n\n" +
		"<details>\n<summary>📥 Commits</summary>\n\nReviewing files that changed from the base of the PR and between abc123 and def456.\n\n</details>\n\n" +
		"<details>\n<summary>📒 Files selected for processing (2)</summary>\n\n* `pkg/markdown/markdown.go` (1 hunks)\n* `pkg/markdown/prepare.go` (1 hunks)\n\n</details>\n\n" +
		"</details>\n\n" +
		"<!-- tips_start -->\n\n---\n\nThanks for using [CodeRabbit](https://coderabbit.ai?utm_source=oss&utm_medium=github&utm_campaign=eggzec/gh-tui&utm_content=91)! It's free for OSS.\n\n" +
		"<details>\n<summary>❤️ Share</summary>\n\n- [X](https://twitter.com/intent/tweet?text=I%20just%20used%20%40coderabbitai)\n- [Mastodon](https://mastodon.social/share?text=I%20just%20used)\n\n</details>\n\n<!-- tips_end -->",
	"dependabot": `Bumps [golang.org/x/net](https://github.com/golang/net) from 0.33.0 to 0.36.0.
<details>
<summary>Commits</summary>
<ul>
<li><a href="https://github.com/golang/net/commit/85d1d54551b68719346cb9fec24b911da4e452a1"><code>85d1d54</code></a> go.mod: update golang.org/x dependencies</li>
<li>See full diff in <a href="https://github.com/golang/net/compare/v0.33.0...v0.36.0">compare view</a></li>
</ul>
</details>
<br />


[![Dependabot compatibility score](https://dependabot-badges.githubapp.com/badges/compatibility_score?dependency-name=golang.org/x/net&package-manager=go_modules&previous-version=0.33.0&new-version=0.36.0)](https://docs.github.com/en/github/managing-security-vulnerabilities/about-dependabot-security-updates#about-compatibility-scores)

Dependabot will resolve any conflicts with this PR as long as you don't alter it yourself.

---

<details>
<summary>Dependabot commands and options</summary>
<br />

You can trigger Dependabot actions by commenting on this PR:
- ` + "`@dependabot rebase`" + ` will rebase this PR
- ` + "`@dependabot ignore this major version`" + ` will close this PR

</details>`,
	"alerts":   "> [!NOTE]\n> Useful information.\n\n> [!WARNING]\n> Critical content.\n\n> [!CAUTION]\n> Negative outcomes.",
	"lists":    "- [x] done\n- [ ] todo\n  - nested one\n    - nested two with a very long line of text that should wrap nicely at narrow widths without breaking\n1. first\n2. second\n\nA footnote[^1] and :tada: :+1: emoji.\n\n[^1]: The note.\n\nhttps://github.com/eggzec/gh-tui/blob/main/pkg/markdown/some/really/long/path/that/goes/on/and/on/forever/file.go#L10-L20",
	"cjk":      "これは日本語のテキストです。幅の広い文字が正しく折り返されるかを確認します。中文文本也应该正确换行，不应超过宽度。한국어 텍스트도 마찬가지입니다.",
	"tabs":     "```go\nfunc main() {\n\tif x {\n\t\treturn\n\t}\n}\n```\n\nText\twith\ttabs.",
	"odd":      "<details>\n<summary>Unclosed\n\nbody text\n\n<details><summary>inner</summary>inner body</details>\n\n<summary></summary>\n\n</details></details></details>\n\n<details open><summary><b>Bold</b> summary</summary>\n\nx\n</details>\n\n<p align=\"center\"><img src=\"a.png\" alt=\"logo\" width=100></p>\n\n<!-- unclosed comment\n\nstill hidden?",
	"codehtml": "```html\n<!-- keep me -->\n<details><summary>keep</summary></details>\n<img src=x alt=y>\n```\n\n`<!-- inline -->` and ``<img src=x>``",
}

// TestCorpus renders bodies as GitHub has them, from templates and bots,
// at several widths in both themes: no line is wider than the width, the
// render only colors text, and it shows what GitHub shows.
func TestCorpus(t *testing.T) {
	for _, dark := range []bool{true, false} {
		r := New(DefaultStyle(dark))
		for name, src := range corpus {
			for _, w := range []int{36, 76, 116} {
				out := r.Render(src, w)
				for i, l := range strings.Split(out, "\n") {
					if lw := xansi.StringWidth(l); lw > w {
						t.Errorf("%s dark=%v w=%d: line %d is %d wide: %q", name, dark, w, i+1, lw, xansi.Strip(l))
					}
				}
				if why := unsafe(out); why != "" {
					t.Errorf("%s dark=%v w=%d: %s", name, dark, w, why)
				}
			}
		}
	}
	r := New(DefaultStyle(true))
	for name, shows := range map[string][]string{
		"template":   {"Summary\n", "[✓] Tests pass", "Fixes #123"},
		"coderabbit": {"▸ 📜 Recent review details", "• \u200bX https://twitter.com", "◆ sequence diagram · 5 lines · View diagram ↗"},
		"dependabot": {"▸ Commits", "🖼 Dependabot compatibility score https://docs.github.com"},
		"alerts":     {"│ ℹ Note\n│ Useful information.", "│ ⚠ Warning", "│ ✖ Caution"},
		"lists":      {"🎉 👍 emoji"},
		"odd":        {"▸ inner\n\ninner body", "🖼 logo"},
		"codehtml":   {"<!-- keep me -->", "<img src=x alt=y>"},
	} {
		out := xansi.Strip(r.Render(corpus[name], 116))
		for _, want := range shows {
			if !strings.Contains(out, want) {
				t.Errorf("%s lacks %q:\n%s", name, want, out)
			}
		}
		for _, hidden := range []string{"walkthrough_start", "Thanks for opening", "[!NOTE]", "<details>", "<summary>"} {
			if name != "codehtml" && strings.Contains(out, hidden) {
				t.Errorf("%s shows %q:\n%s", name, hidden, out)
			}
		}
	}
}
