package files

import (
	"fmt"
	"path"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func entry(p string, typ core.EntryType) core.TreeEntry {
	return core.TreeEntry{Path: p, Name: path.Base(p), Type: typ}
}

func TestNewIndex(t *testing.T) {
	blob, tree := core.EntryBlob, core.EntryTree
	listing := core.Tree{SHA: "s", Entries: []core.TreeEntry{
		entry("Makefile", blob),
		entry("a-b.go", blob),
		entry("cmd", tree),
		entry("cmd/main.go", blob),
		entry("internal", tree),
		entry("internal/Core.go", blob),
		entry("internal/app.go", blob),
		entry("internal/tui", tree),
		entry("internal/tui/view.go", blob),
		entry("Docs", tree),
	}}
	x := newIndex(listing)
	paths := func(dir string) []string {
		out := make([]string, 0, len(x.dirs[dir]))
		for _, e := range x.dirs[dir] {
			out = append(out, e.Path)
		}
		return out
	}
	tests := []struct {
		dir  string
		want []string
	}{
		{"", []string{"cmd", "Docs", "internal", "a-b.go", "Makefile"}},
		{"cmd", []string{"cmd/main.go"}},
		{"internal", []string{"internal/tui", "internal/app.go", "internal/Core.go"}},
		{"internal/tui", []string{"internal/tui/view.go"}},
		{"Docs", []string{}},
	}
	for _, tt := range tests {
		if got := paths(tt.dir); !slices.Equal(got, tt.want) {
			t.Errorf("dir %q = %q, want %q", tt.dir, got, tt.want)
		}
	}
	// Each directory is its own slice, so appending to one can't spill
	// into the next.
	if d := x.dirs["cmd"]; cap(d) != len(d) {
		t.Errorf("cap = %d, want %d", cap(d), len(d))
	}
}

// listing50k is a listing of 50,000 entries: 2,500 directories two levels
// deep, holding 19 files each.
func listing50k() core.Tree {
	t := core.Tree{SHA: "big"}
	for a := range 50 {
		top := fmt.Sprintf("pkg%02d", a)
		t.Entries = append(t.Entries, core.TreeEntry{Path: top, Name: top, Type: core.EntryTree})
		for b := range 49 {
			dir := fmt.Sprintf("%s/sub%02d", top, b)
			t.Entries = append(t.Entries, core.TreeEntry{Path: dir, Name: dir[len(top)+1:], Type: core.EntryTree})
			for c := range 19 {
				name := fmt.Sprintf("file%02d.go", c)
				t.Entries = append(t.Entries, core.TreeEntry{Path: dir + "/" + name, Name: name, Type: core.EntryBlob, Size: 1234})
			}
		}
	}
	return t
}

func BenchmarkNewIndex(b *testing.B) {
	t := listing50k()
	if n := len(t.Entries); n < 49_000 || n > 51_000 {
		b.Fatalf("listing has %d entries, want about 50,000", n)
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = newIndex(t)
	}
}
