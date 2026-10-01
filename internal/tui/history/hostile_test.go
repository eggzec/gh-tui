package history

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// Branch names, commit messages, names, emails and paths are text from
// GitHub and git, which the panes draw.
func TestViewCleansHostileHistory(t *testing.T) {
	h := termtexttest.Hostile
	hostile := func() *fake {
		f := newFake()
		f.branches = append(f.branches, core.Branch{Name: h})
		top := &f.histories["main"][0]
		top.Subject, top.Body, top.Author.Name, top.Author.Email = h, h+"\n"+h, h, h
		top.Author.Login = termtexttest.HostileLogin
		d := detail(*top)
		d.Files[0].Path, d.Files[1].Path, d.Files[1].PreviousPath = h, h, "old/"+h
		d.Files[1].Status = core.FileRenamed
		f.details[top.SHA] = d
		return f
	}
	for _, size := range [][2]int{{wideW, wideH}, {narrowW, narrowH}} {
		for _, keys := range [][]string{nil, {"esc", "j"}, {"enter"}, {"enter", "enter"}, {"enter", "enter", "j"}} {
			m, host := newModal(t, hostile(), size[0], size[1])
			host.keys(keys...)
			termtexttest.AssertClean(t, m.View(), size[0])
		}
	}
}
