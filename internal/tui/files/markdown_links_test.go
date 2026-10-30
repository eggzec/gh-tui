package files

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A relative link of a markdown file goes to its page on the host of the
// session, as GitHub resolves it from the file's directory.
func TestMarkdownLinkResolves(t *testing.T) {
	t.Parallel()
	for _, host := range uitest.Hosts {
		v := &markdownView{host: host, repo: core.RepoRef{Owner: "o", Name: "r"}, ref: "feat/x y", file: "docs/Guide.md"}
		base := "https://" + host + "/o/r/"
		for addr, want := range map[string]string{
			"other.md":         base + "blob/feat/x%20y/docs/other.md",
			"../README.md#top": base + "blob/feat/x%20y/README.md#top",
			"/LICENSE":         base + "blob/feat/x%20y/LICENSE",
			"sub/":             base + "tree/feat/x%20y/docs/sub",
			"a b.md":           base + "blob/feat/x%20y/docs/a%20b.md",
			"../../../escape":  "",
			"#section":         "",
		} {
			if got := v.link(addr); got != want {
				t.Errorf("%s: link(%q) = %q, want %q", host, addr, got, want)
			}
		}
	}
}
