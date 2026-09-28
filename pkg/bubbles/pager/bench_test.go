package pager

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// bigSource is about 50,000 lines of Go.
var bigSource = sync.OnceValue(func() string {
	return strings.Repeat(goSource, 5_000)
})

// bigPager returns a pager over bigSource, highlighted and scrolled to the
// middle, with every "fmt" found.
var bigPager = sync.OnceValue(func() Model {
	m := New(WithSize(120, 40), WithHighlightLimit(math.MaxInt))
	m.Focus()
	msg := m.SetContent("big.go", bigSource())()
	m, _ = m.Update(msg)
	m.top = m.Lines() / 2
	if cmd := m.runSearch("fmt", smartCase("fmt", "fmt"), m.top); cmd != nil {
		m, _ = m.Update(cmd())
	}
	return m
})

func BenchmarkView(b *testing.B) {
	for _, wrap := range []bool{false, true} {
		name := "scroll"
		if wrap {
			name = "wrap"
		}
		b.Run(name, func(b *testing.B) {
			m := bigPager()
			m.SetWrap(wrap)
			b.ReportAllocs()
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}

func BenchmarkUpdate(b *testing.B) {
	for _, tt := range []struct {
		name     string
		fwd, bwd tea.Msg
		wrap     bool
	}{
		{name: "line", fwd: press("j"), bwd: press("k")},
		{name: "page", fwd: press("f"), bwd: press("b")},
		{name: "page/wrap", fwd: press("f"), bwd: press("b"), wrap: true},
		{name: "match", fwd: press("n"), bwd: press("N")},
		{name: "end", fwd: press("G"), bwd: press("g")},
	} {
		b.Run(tt.name, func(b *testing.B) {
			m := bigPager()
			m.SetWrap(tt.wrap)
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				// Go forward and back so the window stays in the middle.
				msg := tt.fwd
				if i%2 == 1 {
					msg = tt.bwd
				}
				i++
				m, _ = m.Update(msg)
			}
		})
	}
}

// BenchmarkSetContent measures what SetContent costs Update, without the
// highlighting, which runs in a command.
func BenchmarkSetContent(b *testing.B) {
	m := New(WithSize(120, 40))
	src := bigSource()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.SetContent("big.go", src)
	}
}

// BenchmarkSearch measures a search of 1 MiB and 16 MiB of Go, as the
// command that runs it does, for a literal and for a pattern.
func BenchmarkSearch(b *testing.B) {
	for _, size := range []int{1 << 20, 16 << 20} {
		text := strings.Repeat(goSource, size/len(goSource)+1)[:size]
		lines := strings.Split(text, "\n")
		for _, tt := range []struct{ name, pattern string }{
			{name: "literal", pattern: "(?i)fmt"},
			{name: "regexp", pattern: `(?i)print\w*\(`},
		} {
			re := regexp.MustCompile(tt.pattern)
			b.Run(strconv.Itoa(size>>20)+"MiB/"+tt.name, func(b *testing.B) {
				b.SetBytes(int64(size))
				b.ReportAllocs()
				for b.Loop() {
					_, _, _ = find(context.Background(), re, lines)
				}
			})
		}
	}
}
