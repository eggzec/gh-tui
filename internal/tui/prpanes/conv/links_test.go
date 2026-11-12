package conv

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// linked is the age as the terminal gets it when it links to url.
func linked(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// The age of each comment, review, thread and reply links to its own page.
// This test is serial because it sets the process-wide links switch.
func TestAgeLinksToPermalink(t *testing.T) {
	was := termtext.LinksOn()
	termtext.SetLinks(true)
	t.Cleanup(func() { termtext.SetLinks(was) })

	c := comment("1")
	c.URL = "https://ghe.example.com/o/r/pull/1#issuecomment-1"
	bare := comment("2")
	rv := review("R", core.ReviewStateApproved)
	rv.URL = "https://github.com/o/r/pull/1#pullrequestreview-9"
	th := thread("T", "R", 2)
	th.Comments[0].URL = "https://github.com/o/r/pull/1#discussion_r1"
	th.Comments[1].URL = "https://github.com/o/r/pull/1#discussion_r2"
	pend := thread("D", "R", 2)
	pend.Comments[0].URL, pend.Comments[0].Pending = "https://github.com/o/r/pull/1#discussion_r3", true
	pend.Comments[1].URL, pend.Comments[1].Pending = "https://github.com/o/r/pull/1#discussion_r4", true
	insecure := comment("3")
	insecure.URL = "http://other.example.com/o/r/pull/1#issuecomment-3"

	data := Data{Items: []core.TimelineItem{c, rv, bare, review("S", core.ReviewStateApproved), insecure}, Threads: []core.Thread{th, pend}}
	o := opts(config.IconsUnicode, 100)
	rows := byID(Build(data, o))
	age := o.Styles.dim.Render("3d")
	for id, url := range map[string]string{
		"c:1":   c.URL,
		"r:R":   rv.URL,
		"t:T":   th.Comments[0].URL,
		"p:T-b": th.Comments[1].URL,
	} {
		if got := rows[id].Right; !strings.Contains(got, linked(url, age)) {
			t.Errorf("%s: right = %q, want the age linked to %s", id, got, url)
		}
	}
	for _, id := range []string{"c:2", "c:3", "t:D", "p:D-b", "r:pending"} {
		r, ok := rows[id]
		if !ok {
			continue
		}
		if strings.Contains(r.Right, "\x1b]8;") || strings.Contains(strings.Join(r.Lines, "\n"), "\x1b]8;") {
			t.Errorf("%s: drew a link: %q", id, r.Right)
		}
	}

	// The link adds no cells and changes no text.
	termtext.SetLinks(false)
	off := byID(Build(data, o))
	for id, r := range rows {
		if strings.Contains(off[id].Right, "\x1b]8;") {
			t.Errorf("%s: drew a link with links off", id)
		}
		if ansi.Strip(r.Right) != ansi.Strip(off[id].Right) || ansi.StringWidth(r.Lines[0]) != ansi.StringWidth(off[id].Lines[0]) {
			t.Errorf("%s: links changed the row's text or width", id)
		}
	}
}
