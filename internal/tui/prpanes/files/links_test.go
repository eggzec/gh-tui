package files

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A file's name links to its diff on the page of the pull request's files,
// on the user's host, and a pane that doesn't know the page has no links.
func TestFileLinks(t *testing.T) {
	fs := []core.CommitFile{
		{Path: "cmd/main.go", Additions: 3},
		{Path: uitest.HostileTitle, Additions: 1},
	}
	for _, host := range uitest.Hosts {
		page := ui.WebURL(host, "o/r/pull/7")
		o := opts("unicode", 60)
		o.FileURL = func(path string) string { return page + "/files#" + ui.DiffAnchor(path) }
		rows := Tree(fs, 0, o)
		got := make([]string, 0, len(rows))
		for _, r := range rows {
			got = append(got, uitest.Links(t, strings.Join(r.Lines, "\n"))...)
		}
		// The rows follow the order of the paths.
		want := []string{page + "/files#" + ui.DiffAnchor(fs[1].Path), page + "/files#" + ui.DiffAnchor(fs[0].Path)}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("%s: names link to %q, want %q", host, got, want)
		}
	}
	for _, r := range Tree(fs, 0, opts("unicode", 60)) {
		if strings.Contains(strings.Join(r.Lines, "\n"), "\x1b]8;") {
			t.Errorf("a pane with no page links: %q", r.Lines)
		}
	}
}
