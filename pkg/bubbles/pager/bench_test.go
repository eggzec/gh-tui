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

// bigColored is about 50,000 lines of a program's output with its colors.
var bigColored = sync.OnceValue(func() string {
	return strings.Repeat(colored, 12_500)
})

// coloredPager returns a pager over bigColored scrolled to the middle,
// with every "view" found.
var coloredPager = sync.OnceValue(func() Model {
	m := New(WithSize(120, 40))
	m.Focus()
	_ = m.SetContent("test.log", bigColored())
	m.top = m.Lines() / 2
	re, _ := compile("view", caseSmart)
	if cmd := m.runSearch("view", re, false, m.top, false); cmd != nil {
		m, _ = m.Update(cmd())
	}
	return m
})

// hostilePager returns a pager over one long line that changes its style
// at every cell and never resets, wrapped and at its end.
var hostilePager = sync.OnceValue(func() Model {
	m := New(WithSize(120, 40), WithWrap(true))
	m.Focus()
	_ = m.SetContent("hostile.log", strings.Repeat("\x1b[1mx\x1b[3my\x1b[22;23mz\x1b[38;5;208m", 70_000))
	m, _ = m.Update(press("G"))
	return m
})

// bigPager returns a pager over bigSource, highlighted and scrolled to the
// middle, with every "fmt" found.
var bigPager = sync.OnceValue(func() Model {
	m := New(WithSize(120, 40), WithHighlightLimit(math.MaxInt))
	m.Focus()
	msg := m.SetContent("big.go", bigSource())()
	m, _ = m.Update(msg)
	m.top = m.Lines() / 2
	re, _ := compile("fmt", caseSmart)
	if cmd := m.runSearch("fmt", re, false, m.top, false); cmd != nil {
		m, _ = m.Update(cmd())
	}
	return m
})

// filteredPager returns bigPager showing only the lines with "Println"
// or "func", so that about half the matches of "fmt" are shown.
var filteredPager = sync.OnceValue(func() Model {
	m := bigPager()
	re, _ := compile("println|func", caseSmart)
	if cmd := m.project(projection{filter: filter{query: "println|func", re: re}}); cmd != nil {
		m, _ = deliver(m, cmd())
	}
	return m
})

func BenchmarkView(b *testing.B) {
	for _, tt := range []struct {
		name                             string
		wrap, filtered, colored, hostile bool
	}{
		{name: "scroll"},
		{name: "wrap", wrap: true},
		{name: "filtered", filtered: true},
		{name: "colored", colored: true},
		{name: "colored/wrap", colored: true, wrap: true},
		{name: "colored/hostile", hostile: true, wrap: true},
	} {
		b.Run(tt.name, func(b *testing.B) {
			m := bigPager()
			switch {
			case tt.filtered:
				m = filteredPager()
			case tt.colored:
				m = coloredPager()
			case tt.hostile:
				m = hostilePager()
			}
			m.SetWrap(tt.wrap)
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
		filtered bool
	}{
		{name: "line", fwd: press("j"), bwd: press("k")},
		{name: "page", fwd: press("f"), bwd: press("b")},
		{name: "page/wrap", fwd: press("f"), bwd: press("b"), wrap: true},
		{name: "match", fwd: press("n"), bwd: press("N")},
		{name: "end", fwd: press("G"), bwd: press("g")},
		{name: "page/filtered", fwd: press("f"), bwd: press("b"), filtered: true},
		{name: "match/filtered", fwd: press("n"), bwd: press("N"), filtered: true},
	} {
		b.Run(tt.name, func(b *testing.B) {
			m := bigPager()
			if tt.filtered {
				m = filteredPager()
			}
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
// highlighting, which runs in a command, for plain and colored content.
func BenchmarkSetContent(b *testing.B) {
	for _, tt := range []struct {
		name string
		src  func() string
	}{
		{name: "plain", src: bigSource},
		{name: "colored", src: bigColored},
	} {
		b.Run(tt.name, func(b *testing.B) {
			m := New(WithSize(120, 40))
			src := tt.src()
			b.ReportAllocs()
			for b.Loop() {
				_ = m.SetContent("big", src)
			}
		})
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
					_, _, _ = find(context.Background(), re, false, lines, nil)
				}
			})
		}
	}
}

// BenchmarkFilter measures a filter of 100,000 lines of Go, as the command
// that runs it does, for a literal, a pattern and the lines a pattern
// doesn't match, and what confirming it costs Update, which only starts
// that command.
func BenchmarkFilter(b *testing.B) {
	const n = 100_000
	text := strings.Repeat(goSource, n/strings.Count(goSource, "\n")+1)
	lines := strings.Split(text, "\n")[:n]
	for _, tt := range []struct {
		name, pattern string
		invert        bool
	}{
		{name: "literal", pattern: "(?i)fmt"},
		{name: "regexp", pattern: `(?i)print\w*\(`},
		{name: "inverted", pattern: "(?i)fmt", invert: true},
	} {
		p := projection{filter: filter{re: regexp.MustCompile(tt.pattern), invert: tt.invert}}
		b.Run("100k/"+tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _, _ = pick(context.Background(), lines, p, nil)
			}
		})
	}
	b.Run("100k/update", func(b *testing.B) {
		m := New(WithSize(120, 40))
		m.Focus()
		_ = m.SetContent("big.go", strings.Join(lines, "\n"))
		m, _ = m.Update(press("&"))
		for _, r := range "fmt" {
			m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		b.ReportAllocs()
		for b.Loop() {
			_, cmd := m.Update(enter)
			if cmd == nil {
				b.Fatal("the filter ran in Update")
			}
		}
	})
}
