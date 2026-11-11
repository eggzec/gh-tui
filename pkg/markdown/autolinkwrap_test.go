package markdown

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

// Release notes list a bullet per commit with a full hash and an author. Once
// the references link, a bullet that wraps must still fill the width: the
// address that a link carries is not text on the screen.
func TestRenderAutolinkWrapsAtFullWidth(t *testing.T) {
	linksOn(t, true)
	const width = 186
	words := strings.Repeat("persist the deactivated flag of users when the workspace admin syncs them and ", 4)
	body := "## Changelog\n\n" +
		"- a3af47c1b2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7 " + words + "(@wlecki)\n" +
		"- 0123abcdef0123abcdef0123abcdef0123abcdef " + words + "(@gammons)\n"
	r := New(DefaultStyle(true))
	r.SetAutolink(func(kind RefKind, ref string) (string, bool) {
		if kind == RefCommit {
			return "https://github.com/gammons/slk/commit/" + ref, true
		}
		return "https://github.com/" + ref, true
	})
	var bullets, long int
	lines := strings.Split(xansi.Strip(r.Render(body, width)), "\n")
	for i, l := range lines {
		// A line that another line of its bullet follows is full. It may
		// fall short by the width of the longest word, about 20 cells,
		// since a word that does not fit goes to the next line.
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") && strings.TrimSpace(lines[i+1]) != "" {
			bullets++
			if xansi.StringWidth(l) >= width-20 {
				long++
			}
		}
	}
	if bullets == 0 || long != bullets {
		t.Errorf("%d of %d wrapped lines fill the width:\n%s", long, bullets, strings.Join(lines, "\n"))
	}
}
