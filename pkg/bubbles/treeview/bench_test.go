package treeview

import (
	"fmt"
	"testing"
)

// benchRows returns 10000 rows: 200 sections of 5 folds of 9 rows, the folds
// open, each with a marker and a right-aligned part, and every third row
// with a body.
func benchRows() []Node {
	rows := make([]Node, 0, 10000)
	for s := range 200 {
		rows = append(rows, Node{ID: fmt.Sprintf("s%d", s), Branch: true, Lines: []string{fmt.Sprintf("section %d", s)}, Marker: "⋯"})
		for f := range 5 {
			id := fmt.Sprintf("s%d/f%d", s, f)
			rows = append(rows, Node{ID: id, Depth: 1, Branch: true, Lines: []string{"folder " + id}, Right: "3d"})
			for i := range 9 {
				r := Node{ID: fmt.Sprintf("%s/r%d", id, i), Depth: 2, Kind: "file", Lines: []string{fmt.Sprintf("a row of text %d in %s", i, id)}, Right: "+5 −2"}
				if i%3 == 0 {
					r.Lines = append(r.Lines, "a body line", "and another one")
				}
				rows = append(rows, r)
			}
		}
	}
	return rows
}

func benchTree(b *testing.B) Model {
	b.Helper()
	m := newTree(benchRows(), WithSize(100, 40))
	if m.Len() < 10000 {
		b.Fatalf("Len() = %d, want 10000 or more", m.Len())
	}
	m.setCur(m.Len() / 2)
	m.scroll()
	return m
}

func BenchmarkView(b *testing.B) {
	m := benchTree(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := benchTree(b)
	down, up := press("j"), press("k")
	b.ReportAllocs()
	for b.Loop() {
		m, _ = m.Update(down)
		m, _ = m.Update(up)
	}
}

// BenchmarkUpdateFold folds and unfolds a section, which lays every row out
// again.
func BenchmarkUpdateFold(b *testing.B) {
	m := benchTree(b)
	m.setCur(0)
	enter := press("enter")
	b.ReportAllocs()
	for b.Loop() {
		m, _ = m.Update(enter)
		m, _ = m.Update(enter)
	}
}

func BenchmarkSetRows(b *testing.B) {
	rows := benchRows()
	m := benchTree(b)
	b.ReportAllocs()
	for b.Loop() {
		m.SetRows(rows)
	}
}
