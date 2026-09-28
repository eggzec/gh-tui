package cmdline

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestViewCleansHostileCandidates(t *testing.T) {
	h := termtexttest.Hostile
	complete := func(string, int) []Candidate { return []Candidate{{Text: "x", Label: h, Detail: h}} }
	m := opened(t, "", WithComplete(complete), WithSize(80, MaxHeight))
	termtexttest.AssertClean(t, m.View(), 80)
}
