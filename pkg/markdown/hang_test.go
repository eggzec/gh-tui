package markdown

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestHang(t *testing.T) {
	t.Parallel()
	const long = "this is starting here and continuing here on a long line"
	cases := map[string]string{
		"bullet":   "* " + long + "\n* second item that also wraps around the width limit\n",
		"numbered": "1. " + long + "\n2. second item that also wraps around the width limit\n",
		"ten":      "10. " + long + "\n11. second item that also wraps around the width limit\n",
		"nested": "* " + long + "\n  * nested item that is long enough to wrap around\n" +
			"    * and one deeper that wraps around the width\n* back out again\n",
		"quote":  "> * " + long + "\n> * second item that wraps around the width limit\n",
		"fence":  "* item before the code that is long enough to wrap\n\n  ```go\n  x := \"a long line of code that is never wrapped\"\n  ```\n\n* " + long + "\n",
		"breaks": "* short\n  lines\n  stay apart\n",
		"table":  "* item\n\n  | a | b |\n  |---|---|\n  | 1 | 2 |\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out := New(DefaultStyle(true)).Render(src, 30)
			var plain []string
			for l := range strings.SplitSeq(out, "\n") {
				if w := xansi.StringWidth(l); w > 30 && name != "fence" {
					t.Errorf("line %q is %d cells, past the width", xansi.Strip(l), w)
				}
				plain = append(plain, xansi.Strip(l))
			}
			golden.RequireEqual(t, strings.Join(plain, "\n"))
		})
	}
}
