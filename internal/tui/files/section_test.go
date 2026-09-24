package files

import (
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestNoRepo(t *testing.T) {
	f := sampleFake()
	s := newSection(t, f, 40, 8)
	if got := screen(s); !strings.Contains(got, "No repository selected") {
		t.Errorf("screen = %q, want the empty state", got)
	}
	if msgs := keys(s, "o", "r", "+", "enter"); len(msgs) != 0 {
		t.Errorf("keys sent %#v, want nothing before a repository is selected", msgs)
	}
	if len(f.reads) != 0 || len(f.invalidated) != 0 {
		t.Errorf("reads %v, invalidated %v; want none", f.reads, f.invalidated)
	}
}

func TestRepoSwitch(t *testing.T) {
	f := sampleFake()
	s := newSection(t, f, 40, 12)
	run(s, s.Update(ui.RepoMsg{Repo: ghTUI}))
	if got := f.readRefs(); !slices.Equal(got, []string{""}) {
		t.Fatalf("reads = %q, want the HEAD tree", got)
	}
	keys(s, "+")
	if got := screen(s); !strings.Contains(got, "gh-tui") || !strings.Contains(got, "go.mod") {
		t.Fatalf("screen = %q, want the files of gh-tui with cmd expanded", got)
	}

	// The same repository, spelled differently, keeps the tree.
	run(s, s.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}}))
	if n := len(f.reads); n != 2 {
		t.Errorf("%d reads after selecting the same repository, want 2", n)
	}

	// A load still on its way when another repository is selected is
	// cancelled, and its result ignored.
	run(s, s.Update(ui.RepoMsg{Repo: other}))
	late := s.Update(ui.RepoMsg{Repo: ghTUI})
	run(s, s.Update(ui.RepoMsg{Repo: other}))
	run(s, late)
	if f.cancelled != 1 {
		t.Errorf("%d loads ran cancelled, want the late one", f.cancelled)
	}
	got := screen(s)
	if !strings.Contains(got, "tea.go") || strings.Contains(got, "cmd") {
		t.Errorf("screen = %q, want only the files of bubbletea", got)
	}
	if last := f.reads[len(f.reads)-2]; last.Repo != other || last.Ref != "" {
		t.Errorf("last read = %+v, want the HEAD tree of bubbletea", last)
	}
}

func TestExpand(t *testing.T) {
	f := sampleFake()
	s := loaded(t, f, 40, 12)
	keys(s, "+", "down", "+")
	if got, want := f.readRefs(), []string{"", cmdSHA, ghTUISHA}; !slices.Equal(got, want) {
		t.Errorf("reads = %q, want %q", got, want)
	}
	if got := screen(s); !strings.Contains(got, "main.go") {
		t.Errorf("screen = %q, want cmd/gh-tui/main.go", got)
	}
	n, _ := s.tree.Selected()
	if e, _ := entryOf(n); e.Path != "cmd/gh-tui" || e.SHA != ghTUISHA {
		t.Errorf("selected %+v, want cmd/gh-tui with its path from the root", e)
	}

	// Directories are read by SHA, so another tree over the same service
	// takes them from the cache.
	s2 := loaded(t, f, 40, 12)
	keys(s2, "+")
	if got := f.readRefs(); len(got) != 4 || got[3] != "" {
		t.Errorf("reads = %q, want only the HEAD tree read again", got)
	}
	if got := screen(s2); !strings.Contains(got, "gh-tui") {
		t.Errorf("screen = %q, want cmd expanded from the cache", got)
	}
}

func TestConfiguredKeys(t *testing.T) {
	cfg := maps.Clone(config.Default().Keys)
	cfg[config.ActionExpand] = []string{"e"}
	cfg[config.ActionCollapse] = []string{"c"}
	s := New(t.Context(), sampleFake(), cfg, WithRepo(ghTUI))
	s.SetSize(40, 12)
	s.Focus()
	run(s, s.Init())

	keys(s, "+")
	if strings.Contains(screen(s), "gh-tui") {
		t.Error("+ expanded, but expand is bound to e")
	}
	keys(s, "e")
	if !strings.Contains(screen(s), "gh-tui") {
		t.Error("e didn't expand")
	}
	keys(s, "c")
	if strings.Contains(screen(s), "gh-tui") {
		t.Error("c didn't collapse")
	}
	keys(s, "e", "left")
	if strings.Contains(screen(s), "gh-tui") {
		t.Error("← no longer collapses alongside the configured keys")
	}
}

func TestRefresh(t *testing.T) {
	f := sampleFake()
	s := loaded(t, f, 40, 12)
	keys(s, "+", "r")
	if !slices.Equal(f.invalidated, []core.RepoRef{ghTUI}) {
		t.Errorf("invalidated %v, want gh-tui", f.invalidated)
	}
	// The expanded directory comes from the cache, the root is read again.
	if got, want := f.readRefs(), []string{"", cmdSHA, ""}; !slices.Equal(got, want) {
		t.Errorf("reads = %q, want %q", got, want)
	}
	if got := screen(s); !strings.Contains(got, "gh-tui") {
		t.Errorf("screen = %q, want cmd still expanded", got)
	}
}

func TestRetry(t *testing.T) {
	f := sampleFake()
	f.errs[treeKey(ghTUI, "")] = errNoTree
	s := loaded(t, f, 60, 5)
	if got := screen(s); !strings.Contains(got, "Couldn't load: 404 Not Found") {
		t.Fatalf("screen = %q, want the error", got)
	}
	delete(f.errs, treeKey(ghTUI, ""))
	keys(s, "r")
	if got := screen(s); !strings.Contains(got, "AGENTS.md") {
		t.Errorf("screen = %q, want the files after a refresh", got)
	}
}

func TestOpenInBrowser(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"directory", nil, "https://github.com/eggzec/gh-tui/tree/HEAD/cmd"},
		{"submodule", []string{"down", "down"}, "https://github.com/eggzec/gh-tui/tree/HEAD/vendor-lib"},
		{"file", []string{"G"}, "https://github.com/eggzec/gh-tui/blob/HEAD/README%20with%20spaces.md"},
		{"nested file", []string{"+", "down", "+", "down"}, "https://github.com/eggzec/gh-tui/blob/HEAD/cmd/gh-tui/main.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := loaded(t, sampleFake(), 40, 12)
			keys(s, tt.keys...)
			got := keys(s, "o")
			if want := []tea.Msg{ui.OpenMsg{URL: tt.want}}; !slices.Equal(got, want) {
				t.Errorf("o sent %#v, want %#v", got, want)
			}
		})
	}
}

func TestBlurred(t *testing.T) {
	f := sampleFake()
	s := loaded(t, f, 40, 12)
	s.Blur()
	if msgs := keys(s, "o", "+", "r"); len(msgs) != 0 || len(f.reads) != 1 {
		t.Errorf("a blurred section sent %#v and read %v", msgs, f.reads)
	}
}
