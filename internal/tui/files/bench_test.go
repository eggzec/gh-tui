package files

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// benchSection returns a section at 40 by 40 over a repository of 50
// directories of 20 files each, all expanded, with the cursor in the
// middle.
func benchSection(b *testing.B) *Section {
	b.Helper()
	f := newFake()
	root := make([]core.TreeEntry, 50)
	for i := range root {
		sha := fmt.Sprintf("%040x", i+1)
		root[i] = dir(fmt.Sprintf("dir-%02d", i), sha)
		files := make([]core.TreeEntry, 20)
		for j := range files {
			files[j] = file(fmt.Sprintf("file-%02d.go", j), 1000)
		}
		f.addTree(ghTUI, sha, files...)
	}
	f.addTree(ghTUI, "", root...)
	s := loaded(b, f, 40, 40)
	for range root {
		keys(s, "+", "down")
	}
	keys(s, "g")
	for range 500 {
		keys(s, "down")
	}
	return s
}

func BenchmarkView(b *testing.B) {
	s := benchSection(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = s.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	s := benchSection(b)
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a window down and back so the tree scrolls.
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		run(s, s.Update(msg))
	}
}

// benchPreview returns a preview at 100 by 40 of a Go file of 2,000
// lines, highlighted and scrolled into the middle.
func benchPreview(b *testing.B) *host {
	b.Helper()
	f := sampleFake()
	f.addBlob(file("main.go", 0), strings.Repeat(mainGo, 250))
	h := newHost(loaded(b, f, 40, 40))
	h.width, h.height = 100, 40
	h.keys("+", "down", "+", "down", "enter")
	h.keys(slices.Repeat([]string{"f"}, 20)...)
	return h
}

func BenchmarkPreviewView(b *testing.B) {
	h := benchPreview(b)
	m := h.top()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkPreviewUpdate(b *testing.B) {
	h := benchPreview(b)
	m := h.top()
	down, up := tea.Msg(press("j")), tea.Msg(press("k"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		_ = m.Update(msg)
	}
}
