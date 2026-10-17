package merge

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// frame draws the pane as the modal does, with its styling: the branch line,
// the rows in a tree as high as they are, the rule and the actions line.
func frame(d Data, o Options) string {
	rows, _ := Rows(d, o)
	tree := foldtree.New(foldtree.WithSize(o.Width, max(len(rows), 1)), foldtree.WithRows(rows))
	body := strings.Split(tree.View(), "\n")
	lines := make([]string, 0, len(body)+3)
	lines = append(lines, Branch(d, o))
	lines = append(lines, body...)
	lines = append(lines, Rule(o), Actions(d, o))
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func TestGoldens(t *testing.T) {
	t.Parallel()
	behind := func(n int) Data {
		d := detail()
		d.Merge.Status = core.MergeBehind
		return Data{Detail: d, Behind: n, Method: "merge"}
	}
	long := everything()
	long.Detail.HeadRef = "feature/" + strings.Repeat("a-very-long-branch-name-", 4)
	long.Detail.BaseRef = "release/" + strings.Repeat("long-base-", 5)
	long.Detail.Reviewers.Requested = []core.ReviewRequest{
		{Team: "acme/platform-infrastructure", TeamName: "Platform infrastructure team", CodeOwner: true},
		{Team: "acme/frontend-guild", TeamName: "Frontend guild", CodeOwner: true},
	}
	long.Detail.Reviewers.Verdicts[0].Author = user("carol", "Carol Maria Alexandra Diaz-Fernandez de la Cruz")

	checksOnly := Data{Detail: detail(), Checks: failing(), ChecksLoaded: true, Behind: -1, Method: "squash"}
	zero := Data{Detail: detail(), Behind: -1, Method: "squash"}
	scenarios := []struct {
		name string
		d    Data
	}{
		{"all", everything()},
		{"zero", zero},
		{"checks", checksOnly},
		{"behind unknown", behind(-1)},
		{"behind known", behind(12)},
		{"long names", long},
	}
	for _, theme := range []struct {
		name string
		dark bool
	}{{"dark", true}, {"light", false}} {
		for _, sc := range scenarios {
			t.Run(theme.name+"/"+sc.name, func(t *testing.T) {
				t.Parallel()
				var b strings.Builder
				for _, w := range []int{40, 54, 80, 120} {
					o := opts(t, w)
					o.Styles = styles(t, theme.dark)
					fmt.Fprintf(&b, "== width %d ==\n%s", w, frame(sc.d, o))
				}
				golden.RequireEqual(t, b.String())
			})
		}
	}
}
