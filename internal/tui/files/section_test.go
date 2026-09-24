package files

import (
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
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
	if n := f.allCount(); n != 1 || len(f.reads) != 0 {
		t.Fatalf("%d listings and reads %q, want one listing", n, f.readRefs())
	}
	keys(s, "+")
	if got := screen(s); !strings.Contains(got, "gh-tui") || !strings.Contains(got, "go.mod") {
		t.Fatalf("screen = %q, want the files of gh-tui with cmd expanded", got)
	}

	// The same repository, spelled differently, keeps the tree.
	run(s, s.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}}))
	if n := f.allCount(); n != 1 {
		t.Errorf("%d listings after selecting the same repository, want 1", n)
	}

	// A listing still on its way when another repository is selected is
	// cancelled, and its result ignored.
	late := s.Update(ui.RepoMsg{Repo: other})
	run(s, s.Update(ui.RepoMsg{Repo: ghTUI}))
	run(s, late)
	if f.cancelled != 1 {
		t.Errorf("%d loads ran cancelled, want the late one", f.cancelled)
	}
	got := screen(s)
	if strings.Contains(got, "tea.go") || !strings.Contains(got, "cmd") {
		t.Errorf("screen = %q, want only the files of gh-tui", got)
	}
}

func TestExpandMakesNoRequests(t *testing.T) {
	f := sampleFake()
	s := loaded(t, f, 40, 12)
	keys(s, "+", "down", "+")
	if n := f.allCount(); n != 1 || len(f.reads) != 0 {
		t.Errorf("%d listings and reads %q, want the one listing", n, f.readRefs())
	}
	if got := screen(s); !strings.Contains(got, "main.go") || !strings.Contains(got, "2.0K") {
		t.Errorf("screen = %q, want cmd/gh-tui/main.go with its size", got)
	}
	n, _ := s.tree.Selected()
	if e, _ := entryOf(n); e.Path != "cmd/gh-tui" || e.SHA != ghTUISHA {
		t.Errorf("selected %+v, want cmd/gh-tui with its path from the root", e)
	}

	keys(s, "g", "down", "down", "down", "*")
	if got := screen(s); !strings.Contains(got, "core") || !strings.Contains(got, "main.go") {
		t.Errorf("screen = %q, want everything expanded", got)
	}

	// Another tree over the same service takes the listing from the cache.
	s2 := loaded(t, f, 40, 12)
	keys(s2, "+")
	if n := f.allCount(); n != 1 || len(f.reads) != 0 {
		t.Errorf("%d listings and reads %q, want the cached listing", n, f.readRefs())
	}
	if got := screen(s2); !strings.Contains(got, "gh-tui") {
		t.Errorf("screen = %q, want cmd expanded from the cache", got)
	}
}

func TestTruncatedListing(t *testing.T) {
	f := sampleFake()
	f.truncated["eggzec/gh-tui"] = true
	s := New(t.Context(), f, config.Default().Keys, WithRepo(ghTUI))
	s.SetSize(40, 12)
	s.Focus()
	msgs := run(s, s.Init())
	if len(msgs) != 1 {
		t.Fatalf("Init sent %#v, want one notice", msgs)
	}
	if n, ok := msgs[0].(ui.NotifyMsg); !ok || !strings.Contains(n.Text, "too large to list at once") {
		t.Errorf("Init sent %#v, want a notice that the listing is truncated", msgs[0])
	}
	// Directories are read one by one, as the listing may miss them.
	keys(s, "+", "down", "+")
	if got, want := f.readRefs(), []string{"", cmdSHA, ghTUISHA}; !slices.Equal(got, want) {
		t.Errorf("reads = %q, want %q", got, want)
	}
	if got := screen(s); !strings.Contains(got, "main.go") {
		t.Errorf("screen = %q, want cmd/gh-tui/main.go", got)
	}
	if nodes, depth := s.tree.ExpandAllLimits(); nodes != tree.DefaultExpandAllNodes || depth != tree.DefaultExpandAllDepth {
		t.Errorf("expand-all limits = %d, %d; want the tree's defaults", nodes, depth)
	}
	if msgs := keys(s, "r"); len(msgs) != 0 {
		t.Errorf("refresh sent %#v, want the notice only once", msgs)
	}
}
func TestOfflineListing(t *testing.T) {
	f := sampleFake()
	f.offline["eggzec/gh-tui"] = true
	s := New(t.Context(), f, config.Default().Keys, WithRepo(ghTUI))
	s.SetSize(40, 12)
	s.Focus()
	msgs := run(s, s.Init())
	if len(msgs) != 1 {
		t.Fatalf("Init sent %#v, want one notice", msgs)
	}
	if n, ok := msgs[0].(ui.NotifyMsg); !ok || !strings.Contains(n.Text, "Can't reach GitHub") {
		t.Errorf("Init sent %#v, want a notice that the files are from the disk", msgs[0])
	}
	if got := screen(s); !strings.Contains(got, "cmd") {
		t.Errorf("screen = %q, want the files", got)
	}
	// Back online, and offline again: the notice came once already.
	for _, off := range []bool{false, true} {
		f.mu.Lock()
		f.offline["eggzec/gh-tui"] = off
		f.mu.Unlock()
		if msgs := keys(s, "r"); len(msgs) != 0 {
			t.Errorf("refresh sent %#v, want the notice only once", msgs)
		}
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
	keys(s, "+", "down", "+")
	f.addTree(ghTUI, ghTUISHA, file("main.go", 2_000), file("root.go", 700))
	keys(s, "r")
	if !slices.Equal(f.invalidated, []core.RepoRef{ghTUI}) {
		t.Errorf("invalidated %v, want gh-tui", f.invalidated)
	}
	// The listing is read again, once, and the tree reloads from it.
	if n := f.allCount(); n != 2 || len(f.reads) != 0 {
		t.Errorf("%d listings and reads %q, want the listing read again", n, f.readRefs())
	}
	if got := screen(s); !strings.Contains(got, "root.go") {
		t.Errorf("screen = %q, want cmd/gh-tui still expanded with the new file", got)
	}
}

func TestSyncReloadsMovedRef(t *testing.T) {
	f := sampleFake()
	s := loaded(t, f, 40, 12)
	keys(s, "+", "down", "+")
	// A revalidation found HEAD moved and cached the new listing.
	f.addTree(ghTUI, ghTUISHA, file("main.go", 2_000), file("root.go", 700))
	for _, msg := range []ui.SyncMsg{
		{Key: filesvc.SyncKey(core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"})},
		{Key: filesvc.SyncKey(ghTUI), Err: errNoTree},
	} {
		run(s, s.Update(msg))
		if strings.Contains(screen(s), "root.go") {
			t.Fatalf("%+v reloaded the tree, want it ignored", msg)
		}
	}
	run(s, s.Update(ui.SyncMsg{Key: filesvc.SyncKey(core.RepoRef{Owner: "EggZec", Name: "GH-TUI"})}))
	if got := screen(s); !strings.Contains(got, "root.go") {
		t.Errorf("screen = %q, want cmd/gh-tui still expanded with the new file", got)
	}
	if n := f.allCount(); n != 1 || len(f.invalidated) != 0 {
		t.Errorf("%d listings, invalidated %v; want the cached listing read again", n, f.invalidated)
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
	if msgs := keys(s, "o", "+", "r"); len(msgs) != 0 || f.allCount() != 1 {
		t.Errorf("a blurred section sent %#v and listed %d times", msgs, f.allCount())
	}
}

func TestBase(t *testing.T) {
	const old = "0123456789abcdef0123456789abcdef01234567"
	f := sampleFake()
	f.addTree(ghTUI, old, file("old.go", 100))
	f.addTree(ghTUI, "v2-exp", file("next.go", 100))
	s := loaded(t, f, 60, 12)
	if helpHas(s, "back to head") {
		t.Error("help offers to reset a base that isn't set")
	}
	if msgs := keys(s, "H"); len(msgs) != 0 {
		t.Errorf("H sent %#v without a base, want nothing", msgs)
	}

	run(s, s.Update(ui.BaseMsg{Repo: ghTUI, Ref: old, Label: "main @ 0123456", Branch: "main"}))
	if got := screen(s); !strings.Contains(got, "old.go") || strings.Contains(got, "AGENTS.md") {
		t.Fatalf("screen = %q, want the files of the older commit", got)
	}
	if s.Ref() != old || !helpHas(s, "back to head") {
		t.Errorf("Ref() = %q and help without the reset key, want the base and its key", s.Ref())
	}
	if got := keys(s, "o"); !slices.Equal(got, []tea.Msg{ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/" + old + "/old.go"}}) {
		t.Errorf("o sent %#v, want the file at the base", got)
	}

	// The same base again, or a base of another repository, lists nothing.
	run(s, s.Update(ui.BaseMsg{Repo: ghTUI, Ref: old}))
	run(s, s.Update(ui.BaseMsg{Repo: other, Ref: "main"}))
	if got, want := f.allRefs(), []string{"", old}; !slices.Equal(got, want) {
		t.Fatalf("listed %q, want %q", got, want)
	}

	run(s, s.Update(ui.BaseMsg{Repo: ghTUI, Ref: "v2-exp", Label: "v2-exp", Branch: "v2-exp"}))
	if got := screen(s); !strings.Contains(got, "next.go") {
		t.Fatalf("screen = %q, want the files of the branch", got)
	}

	// Resetting shows the head again, from the cached listing.
	if got := keys(s, "H"); !slices.Equal(got, []tea.Msg{ui.BaseMsg{Repo: ghTUI}}) {
		t.Errorf("H sent %#v, want the base reset", got)
	}
	if got := screen(s); !strings.Contains(got, "AGENTS.md") || s.Ref() != "" {
		t.Errorf("screen = %q at %q, want the head again", got, s.Ref())
	}
	if got, want := f.allRefs(), []string{"", old, "v2-exp"}; !slices.Equal(got, want) {
		t.Errorf("listed %q, want %q", got, want)
	}
}

func TestRepoMsgResetsTheBase(t *testing.T) {
	const old = "0123456789abcdef0123456789abcdef01234567"
	f := sampleFake()
	f.addTree(ghTUI, old, file("old.go", 100))
	s := loaded(t, f, 60, 12)
	run(s, s.Update(ui.BaseMsg{Repo: ghTUI, Ref: old}))
	// Selecting the repository again shows the head of its default branch.
	run(s, s.Update(ui.RepoMsg{Repo: ghTUI}))
	if got := screen(s); !strings.Contains(got, "AGENTS.md") || s.Ref() != "" {
		t.Errorf("screen = %q at %q, want the head", got, s.Ref())
	}
}

// helpHas reports whether the help of s lists a key described as desc.
func helpHas(s *Section, desc string) bool {
	for _, b := range s.Help().ShortHelp() {
		if b.Enabled() && b.Help().Desc == desc {
			return true
		}
	}
	return false
}
