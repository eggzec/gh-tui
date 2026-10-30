package files

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A relative link in a markdown file names the page of the file it points to
// on the user's host, from the folder of the file; images and addresses that
// aren't relative stay as they are.
func TestMarkdownRelativeLinks(t *testing.T) {
	t.Parallel()
	for _, host := range uitest.Hosts {
		v := &markdownView{host: host, repo: core.RepoRef{Owner: "o", Name: "r"}, ref: "main", file: "docs/guide.md"}
		for _, c := range []struct {
			addr  string
			image bool
			want  string
		}{
			{"setup.md#top", false, ui.WebURL(host, "o/r/blob/main/docs/setup.md#top")},
			{"/LICENSE", false, ui.WebURL(host, "o/r/blob/main/LICENSE")},
			{"../a b.md", false, ui.WebURL(host, "o/r/blob/main/a%20b.md")},
			{"pic.png", true, "pic.png"},
			{"https://example.com/x", false, "https://example.com/x"},
			{"../../x.md", false, "../../x.md"},
		} {
			if got := v.linkTo(c.addr, c.image); got != c.want {
				t.Errorf("%s: %q links to %q, want %q", host, c.addr, got, c.want)
			}
		}
	}
}
