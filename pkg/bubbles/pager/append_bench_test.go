package pager

import (
	"strconv"
	"testing"
)

// benchAppend measures appending batch rows onto 100k rows that the filter
// keeps all of, if there is one. The pager is put back after each append so
// every run starts from the same length.
func benchAppend(b *testing.B, batch int, filter bool) {
	b.Helper()
	l := liveOf(100_000, "entry")
	m := fresh(b, WithSize(80, 24), WithRows(l))
	m.Focus()
	if filter {
		_ = m.filterFor("entry")
	}
	extra := make([]string, batch)
	for i := range extra {
		extra[i] = "entry " + strconv.Itoa(i)
	}
	base := l.Len()
	b.ReportAllocs()
	for b.Loop() {
		l.lines = append(l.lines[:base], extra...)
		m.Append(base)
		m.seen, m.kept, m.size = base, base, m.size-batch*(len("entry 0")+1)
		if m.vis != nil {
			m.vis = m.vis[:base]
		}
	}
}

func BenchmarkAppendOne(b *testing.B)           { benchAppend(b, 1, false) }
func BenchmarkAppendBatch(b *testing.B)         { benchAppend(b, 1000, false) }
func BenchmarkAppendOneFiltered(b *testing.B)   { benchAppend(b, 1, true) }
func BenchmarkAppendBatchFiltered(b *testing.B) { benchAppend(b, 1000, true) }
