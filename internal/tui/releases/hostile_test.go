package releases

import (
	"context"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// hostile serves v3 with name, and a tag and a file named in text that
// tries to take over the terminal.
type hostile struct {
	fakeService
	name string
}

func (f *hostile) Get(ctx context.Context, r core.RepoRef, id int64) (core.Release, error) {
	rel, err := f.fakeService.Get(ctx, r, id)
	h := termtexttest.Hostile
	rel.Name, rel.Tag = f.name, h
	rel.Author.Login = termtexttest.HostileLogin
	rel.Assets = []core.ReleaseAsset{{Name: h, Size: 5300, Downloads: 12}}
	return rel, err
}

// A release's name, tag and files are text from GitHub, which the head of
// the release, its files and its title draw.
func TestViewCleansHostileReleases(t *testing.T) {
	for _, name := range []string{termtexttest.Hostile, ""} {
		for _, w := range []int{40, 148} {
			m := newModal(&hostile{name: name}, w, 18)
			run(t, m, m.Init())
			v := m.View()
			if !strings.Contains(v, "moved") {
				t.Fatalf("the release doesn't show its name: %q", v)
			}
			termtexttest.AssertClean(t, v, w)
			termtexttest.AssertClean(t, m.Title(), 1000)
			if w == 148 && !strings.Contains(v, termtexttest.CleanLogin) {
				t.Errorf("the release doesn't show its author cleaned: %q", v)
			}
		}
	}
}
