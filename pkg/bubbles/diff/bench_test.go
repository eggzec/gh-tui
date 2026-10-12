package diff

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchView returns a view of 3000 files in one page, with a 40-row window
// scrolled into the middle of a file.
func benchView(b *testing.B) Model {
	b.Helper()
	files := make([]File, 3000)
	for i := range files {
		files[i] = goFile(fmt.Sprintf("pkg/dir%d/file%d.go", i/50, i), 8)
	}
	src := &source{size: len(files), files: files}
	m := load(b, src, WithSize(120, 40))
	row, _ := m.layout.FileRow(1500)
	m.cursor, m.top = row+10, row+8
	return run(b, m, m.sync())
}

// benchBlocks returns about 500 blocks of three rows spread over the files of
// benchView, one in every sixth file, on its second line.
func benchBlocks() []Block {
	var blocks []Block
	for i := 0; i < 3000; i += 6 {
		path := fmt.Sprintf("pkg/dir%d/file%d.go", i/50, i)
		blocks = append(blocks, Block{
			ID:     fmt.Sprintf("b%d", i),
			Anchor: Pos{Path: path, Side: NewSide, Line: 2},
			Render: func(_ int, folded bool) []string {
				if folded {
					return []string{"> thread"}
				}
				return []string{"+ thread", "| a remark on the line", "'"}
			},
		})
	}
	return blocks
}

// benchBlockView is benchView with the blocks set, and a block under the cursor
// when onBlock is set.
func benchBlockView(b *testing.B) Model {
	b.Helper()
	m := benchView(b)
	m.SetBlocks(benchBlocks())
	m = run(b, m, m.sync())
	row, _ := m.layout.FileRow(1500)
	m.cursor, m.top = row+10, row+8
	return run(b, m, m.sync())
}

func BenchmarkViewBlocks(b *testing.B) {
	m := benchBlockView(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdateBlocks(b *testing.B) {
	m := benchBlockView(b)
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		m, _ = m.Update(msg)
	}
}

// BenchmarkSetBlocks replaces ~500 blocks over 3000 files by blocks that
// differ in every file, as a reload of the threads would.
func BenchmarkSetBlocks(b *testing.B) {
	m := benchBlockView(b)
	blocks := benchBlocks()
	b.ReportAllocs()
	for b.Loop() {
		m.SetBlocks(blocks)
		m.SetBlocks(blocks[:len(blocks)/2])
	}
}

// BenchmarkFoldBlock folds and unfolds one block in the middle of the diff.
func BenchmarkFoldBlock(b *testing.B) {
	m := benchBlockView(b)
	b.ReportAllocs()
	for b.Loop() {
		m.SetBlockFolded("b1500", true)
		m.SetBlockFolded("b1500", false)
	}
}

func BenchmarkView(b *testing.B) {
	m := benchView(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := benchView(b)
	// Box the keys once so the benchmark measures the view, not the boxing.
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a full window down and back so the window scrolls.
		msg := down
		if i/40%2 == 1 {
			msg = up
		}
		i++
		m, _ = m.Update(msg)
	}
}
