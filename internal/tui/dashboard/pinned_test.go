package dashboard

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestOnePinUsesThePane(t *testing.T) {
	svc := newFake()
	desc := "A GitHub client for the terminal, built with Bubble Tea"
	svc.header.Pinned = []core.Repo{{Ref: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, Description: desc, Language: "Go"}}
	s := newSection(t, svc, &fakeInbox{}, 120, 36, WithHere(core.RepoRef{}, nil))
	if !strings.Contains(screen(s), desc) {
		t.Errorf("the description of the only pin doesn't show on one line:\n%s", screen(s))
	}
}
