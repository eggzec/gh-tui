package merge

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
)

// matrix is a commit whose build job runs on three systems, one failing.
func matrix() Data {
	return Data{Detail: detail(), ChecksLoaded: true, Behind: -1, Checks: commit([]core.CommitCheck{
		job(10, "build (linux)", core.ConclusionFailure),
		job(11, "build (mac)", core.ConclusionSuccess),
		job(12, "build (windows)", core.ConclusionSuccess),
	})}
}

// lacking is everything, behind its base by five commits of which three are
// listed.
func lacking() Data {
	d := everything()
	d.Behind = 5
	d.BaseCommits = core.BaseCommits{AheadBy: 5, Commits: []core.Commit{
		{SHA: "aaaaaaa1111111", Subject: "feat: add the parser"},
		{SHA: "bbbbbbb2222222", Subject: "fix: a very long subject that goes on and on to be cut by a narrow preview"},
		{SHA: "ccccccc3333333", Subject: "chore: bump deps"},
	}}
	return d
}

func TestPreviewGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		d    Data
		id   string
	}{
		{"checks", lacking(), "checks"},
		{"run", lacking(), "run/1"},
		{"matrix", matrix(), "grp/1/build ("},
		{"app", lacking(), "app/codecov"},
		{"status", lacking(), "status/ci/jenkins"},
		{"conflicts", lacking(), idConflict},
		{"behind", lacking(), idBehind},
		{"behind unknown", func() Data { d := lacking(); d.Behind, d.BaseCommits = -1, core.BaseCommits{}; return d }(), idBehind},
		{"draft", lacking(), idDraft},
	}
	for _, theme := range []struct {
		name string
		dark bool
	}{{"dark", true}, {"light", false}} {
		for _, c := range cases {
			t.Run(theme.name+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				var b strings.Builder
				for _, w := range []int{30, 60, 100} {
					o := opts(t, w)
					o.Styles = styles(t, theme.dark)
					p, ok := PreviewOf(c.d, c.id, o)
					if !ok || p.Body == "" {
						t.Fatalf("no preview of %q: %+v, %v", c.id, p, ok)
					}
					fmt.Fprintf(&b, "== width %d: %s | %s ==\n%s\n", w, p.Title, p.Info, p.Body)
				}
				golden.RequireEqual(t, b.String())
			})
		}
	}
}

func TestPreviewTitlesOfRowsWithoutOne(t *testing.T) {
	t.Parallel()
	d := lacking()
	for _, c := range []struct{ id, title string }{
		{idReview, "Required review"},
		{idChanges + "carol", "Changes requested: Carol Diaz"},
		{"job/1", "test"},
		// The frame of a check run of an App names the App and tells how it stands.
		{"check/4", "Codecov · coverage"},
	} {
		p, ok := PreviewOf(d, c.id, opts(t, 60))
		if !ok || p.Title != c.title || p.Body != "" {
			t.Errorf("preview of %q = %+v, %v; want only the title %q", c.id, p, ok, c.title)
		}
	}
	if _, ok := PreviewOf(d, "nope", opts(t, 60)); ok {
		t.Error("an ID no row has got a preview")
	}
	d.ChecksLoaded = false
	if _, ok := PreviewOf(d, "checks", opts(t, 60)); ok {
		t.Error("the checks got a preview before they were read")
	}
}
