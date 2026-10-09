package refs

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// The titles, logins and reasons of links are text from GitHub, which the
// rows draw.
func TestViewCleansHostileLinks(t *testing.T) {
	h := termtexttest.Hostile
	hostile := func() *fake {
		f := newFake()
		r := testRefs()
		r.Closing[0].Title = h
		r.Written[0].Title = h
		r.Written[0].Origins = []core.RefOrigin{{Group: core.RefWritten, Where: "comment", By: termtexttest.HostileLogin}}
		r.Written[4].Problem = h
		f.set(r)
		f.pages[""].Items[0].Title = h
		f.pages[""].Items[0].Origins[0].By = termtexttest.HostileLogin
		return f
	}
	for _, size := range [][2]int{{wideW, wideH}, {narrowW, narrowH}} {
		for _, steps := range [][]string{nil, {"G", "enter"}, {"&"}} {
			s, host := newStep(t, hostile(), size[0], size[1], WithItem(func() Item { return Item{Title: h} }))
			host.keys(steps...)
			host.typed("x")
			termtexttest.AssertClean(t, s.View(), size[0])
		}
	}
}
