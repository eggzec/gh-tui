package ownerui

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
		Styles: &Styles{}, Icons: &icons, Links: &termtext.Links{},
		URL:  func(core.Repo) string { return "" },
		Lang: func(core.Repo) Paint { return Paint{} },
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
	flags := icons.Flags(r)
	if len(flags) == 0 {
		t.Fatal("the repository has no flags")
	}
	name := strings.Index(row, "tea")
	for _, f := range flags {
		if i := strings.Index(row, f); i < 0 || i > name {
			t.Errorf("row %q: flag %q at %d, want it before the name at %d", row, f, i, name)
		}
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
