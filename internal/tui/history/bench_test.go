package history

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func BenchmarkView(b *testing.B) {
	for _, size := range []struct {
		name string
		w, h int
		keys []string
	}{
		{"140 columns", wideW, wideH, nil},
		{"140 columns zoomed", wideW, wideH, []string{"z"}},
		{"80 columns", narrowW, narrowH, nil},
	} {
		b.Run(size.name, func(b *testing.B) {
			m, h := newModal(b, newFake(), size.w, size.h)
			h.keys(append([]string{"j", "j", "enter"}, size.keys...)...)
			b.ReportAllocs()
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}

// BenchmarkViewBranches draws the branch pane over five pages of a hundred
// branches, with the cursor in the middle of the list.
func BenchmarkViewBranches(b *testing.B) {
	const pages, perPage = 5, 100
	f := newFake()
	f.branchPages = map[string]core.Page[core.Branch]{}
	for n := 1; n <= pages; n++ {
		items := make([]core.Branch, perPage)
		for i := range items {
			items[i] = core.Branch{Name: fmt.Sprintf("feat/%d-%d", n, i)}
		}
		if n == 1 {
			items[0] = core.Branch{Name: "main"}
		}
		p := core.Page[core.Branch]{Items: items}
		if n < pages {
			p.Next = strconv.Itoa(n + 1)
		}
		cursor := strconv.Itoa(n)
		if n == 1 {
			cursor = ""
		}
		f.branchPages[cursor] = p
	}
	m, h := newModal(b, f, wideW, wideH)
	h.keys("shift+tab")
	for range pages {
		h.keys("end")
	}
	if got := m.branches.feed.Len(); got < pages*perPage {
		b.Fatalf("loaded %d branches, want %d", got, pages*perPage)
	}
	for range pages * perPage / 2 {
		h.keys("up")
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkUpdate measures moving the graph's cursor onto commits whose
// details are cached, as the reads ahead leave them.
func BenchmarkUpdate(b *testing.B) {
	f := newFake()
	for sha := range f.details {
		f.cached[sha] = true
	}
	m, _ := newModal(b, f, wideW, wideH)
	down, up := press("j"), press("k")
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		k := down
		if i/20%2 == 1 {
			k = up
		}
		i++
		_ = m.Update(k)
	}
}
