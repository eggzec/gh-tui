package thread

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	glamourstyles "charm.land/glamour/v2/styles"
)

type comment struct {
	author string
	body   string
}

// source serves comments in fixed chunks, like a service would, and
// records the cursors it was asked for.
type source struct {
	mu      sync.Mutex
	chunks  [][]comment
	fail    map[string]int // cursor -> failures left
	cursors []string
	ctxs    []context.Context
}

func newSource(chunks, perChunk int) *source {
	s := &source{fail: map[string]int{}}
	for c := range chunks {
		items := make([]comment, 0, perChunk)
		for i := range perChunk {
			n := c*perChunk + i
			// Vary the height, as real comments do.
			body := strings.Repeat("Comment "+strconv.Itoa(n)+" says hello. ", 1+n%3)
			items = append(items, comment{author: "user" + strconv.Itoa(n), body: body})
		}
		s.chunks = append(s.chunks, items)
	}
	return s
}

func (s *source) fetch(ctx context.Context, cursor string) ([]comment, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors = append(s.cursors, cursor)
	s.ctxs = append(s.ctxs, ctx)
	if s.fail[cursor] > 0 {
		s.fail[cursor]--
		return nil, "", errors.New("connection reset")
	}
	i := 0
	if cursor != "" {
		i, _ = strconv.Atoi(cursor)
	}
	if i >= len(s.chunks) {
		return nil, "", nil
	}
	next := ""
	if i+1 < len(s.chunks) {
		next = strconv.Itoa(i + 1)
	}
	return slices.Clone(s.chunks[i]), next, nil
}

// update changes the comments served, as the server's would.
func (s *source) update(f func(chunks [][]comment) [][]comment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks = f(s.chunks)
}

func (s *source) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cursors...)
}

// renders counts calls to renderComment.
type renders struct {
	mu sync.Mutex
	n  int
}

func (r *renders) render(c comment, width int) string {
	r.mu.Lock()
	r.n++
	r.mu.Unlock()
	return renderComment(c, width)
}

func (r *renders) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

// renderComment wraps the body at width, as a caller's renderer would.
func renderComment(c comment, width int) string {
	var b strings.Builder
	b.WriteString("  @" + c.author + "\n")
	line := "   "
	for w := range strings.FieldsSeq(c.body) {
		if len(line)+len(w)+1 > width && line != "   " {
			b.WriteString(line + "\n")
			line = "   "
		}
		line += " " + w
	}
	b.WriteString(line)
	return b.String()
}

const testBody = `## Summary

The cache drops entries **too early** when the TTL is short.

- Reproduce with ` + "`--ttl 1s`" + `
- Watch the hit rate fall

` + "```go\nc := cache.New(cache.WithTTL(time.Second))\n```"

// newTest returns a sized, focused thread over src with a pinned markdown
// style, so the output is the same on every machine.
func newTest(src *source, r *renders, width, height int, opts ...Option) Model[comment] {
	render := renderComment
	if r != nil {
		render = r.render
	}
	opts = append([]Option{
		WithSize(width, height),
		WithFocused(true),
		WithMarkdownStyle(glamourstyles.ASCIIStyleConfig),
	}, opts...)
	return New(src.fetch, render, opts...)
}

// drain runs cmd and every command that follows, feeding the messages back
// into m, but skips spinner ticks, which would never end.
func drain(tb testing.TB, m Model[comment], cmd tea.Cmd) Model[comment] {
	tb.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 1000 {
			tb.Fatal("drain: too many steps")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case spinner.TickMsg:
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			queue = append(queue, next)
		}
	}
	return m
}

// press sends keys and runs what they start.
func press(tb testing.TB, m Model[comment], keys ...string) Model[comment] {
	tb.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = m.Update(keyMsg(k))
		m = drain(tb, m, cmd)
	}
	return m
}

func keyMsg(k string) tea.KeyPressMsg {
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	switch k {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	r := []rune(k)
	if len(r) != 1 {
		panic(fmt.Sprintf("keyMsg: unknown key %q", k))
	}
	return tea.KeyPressMsg{Code: r[0], Text: k}
}

// loaded returns a thread with its document set and the first chunks loaded.
func loaded(tb testing.TB, src *source, r *renders, width, height int, opts ...Option) Model[comment] {
	tb.Helper()
	m := newTest(src, r, width, height, opts...)
	cmd := m.SetDocument(testHeader, testBody)
	return drain(tb, m, cmd)
}

const testHeader = "\x1b[1mCache drops entries early\x1b[m #42\nopen · alice opened 3 days ago"

// toEnd presses G until every chunk is loaded.
func toEnd(tb testing.TB, m Model[comment]) Model[comment] {
	tb.Helper()
	for range 100 {
		if m.done() && m.AtBottom() {
			return m
		}
		m = press(tb, m, "G")
	}
	tb.Fatal("never reached the end")
	return m
}

// resident returns the indexes of the loaded chunks with comments.
func resident(m Model[comment]) []int {
	var out []int
	for i, c := range m.chunks {
		if c.loaded && c.height > 0 {
			out = append(out, i)
		}
	}
	return out
}
