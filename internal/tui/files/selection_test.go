package files

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSelected(t *testing.T) {
	if _, ok := newSection(t, newFake(), 60, 12).Selected(); ok {
		t.Error("a section without a repository selected something")
	}
	f := sampleFake()
	const old = "0123456789abcdef0123456789abcdef01234567"
	f.addTree(ghTUI, old, file("old.go", 100))
	f.addTree(ghTUI, "v2-exp", file("next.go", 100))
	s := loaded(t, f, 60, 12)
	e := s.selected()
	got, ok := s.Selected()
	if !ok || got.Path != e.Path || got.Repo != ghTUI || got.SHA != "" || got.URL != webURL(s.host, ghTUI, "", e) {
		t.Errorf("head: Selected() = %+v, %v, want %s without a SHA", got, ok, e.Path)
	}
	run(s, s.Update(ui.BaseMsg{Repo: ghTUI, Ref: old, Label: "main @ 0123456", Branch: "main"}))
	if got, _ := s.Selected(); got.SHA != old || got.What != "file" || got.Path != "old.go" {
		t.Errorf("commit: Selected() = %+v, want old.go at %s", got, old)
	}
	run(s, s.Update(ui.BaseMsg{Repo: ghTUI, Ref: "v2-exp", Label: "v2-exp", Branch: "v2-exp"}))
	if got, _ := s.Selected(); got.SHA != "" {
		t.Errorf("branch: Selected() = %+v, want no SHA", got)
	}
}
