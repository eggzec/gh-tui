package repolist

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

func rowDrawer(icons ui.Icons) Drawer {
	return Drawer{
		Styles: &ui.ListStyles{}, Icons: &icons, Links: &termtext.Links{},
		URL:  func(core.Repo) string { return "" },
		Lang: func(core.Repo) ui.Paint { return ui.Paint{} },
	}
}

func TestRowFlagsBeforeName(t *testing.T) {
	icons := ui.NewIcons(config.IconsASCII)
	d := rowDrawer(icons)
	r := core.Repo{
		Ref:         core.RepoRef{Owner: "octo", Name: "tea"},
		Description: "Every mark at once, described at such length that the row has to cut the description in the middle",
		Archived:    true,
		Fork:        true,
		Stars:       12,
	}
	var m Measure
	m.Add(r, icons)
	plain := core.Repo{Ref: core.RepoRef{Owner: "octo", Name: "plain"}, Description: "short"}
	m.Add(plain, icons)
	c := LayoutCols(80, m, "*", 2)
	row := ansi.Strip(d.Row(c, r, ui.LookPlain, ui.Dates{}, time.Now))
	if w := ansi.StringWidth(row); w != c.Width() {
		t.Errorf("the row is %d cells, want the %d of its columns", w, c.Width())
	}
	if c.flags != 1 {
		t.Errorf("the flags column is %d cells, want 1", c.flags)
	}
	name := strings.Index(row, "tea")
	if f := icons.Flag(r); f == "" || strings.Index(row, f) != 0 || name != 1+colGap {
		t.Errorf("row %q: want flag %q in the one cell before the name at %d", row, f, name)
	}
	if icons.Flag(r) != icons.Fork || strings.Contains(row, icons.Archived) {
		t.Errorf("row %q, want only the fork flag of a fork that is archived", row)
	}
	if !strings.Contains(row, icons.Ellipsis) || !strings.Contains(row, "Every mark") || !strings.Contains(row, "middle") {
		t.Errorf("row %q, want the description cut in the middle", row)
	}
	// The name column starts where the header's does.
	header := c.Header("*", icons.Ellipsis)
	if h, n := strings.Index(header, nameHeader), name; h != n {
		t.Errorf("the name header is at %d and the name at %d", h, n)
	}
	if w := ansi.StringWidth(header); w != c.Width() {
		t.Errorf("the header is %d cells, want %d", w, c.Width())
	}
}
