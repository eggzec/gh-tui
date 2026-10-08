package diff

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// testKeys are the keys the tests of the view bind, as an app would. Space
// is not among them: the diff leaves it unbound.
var testKeys = map[string][]string{
	"up":             {"up", "k"},
	"down":           {"down", "j"},
	"left":           {"left", "h"},
	"right":          {"right", "l"},
	"page_up":        {"ctrl+b", "pgup"},
	"page_down":      {"ctrl+f", "pgdown"},
	"half_page_up":   {"ctrl+u"},
	"half_page_down": {"ctrl+d"},
	"top":            {"g", "home"},
	"bottom":         {"G", "end"},
	"next_file":      {"J"},
	"prev_file":      {"K"},
	"next_hunk":      {"}"},
	"prev_hunk":      {"{"},
	"global.select":  {"enter"},
	"global.refresh": {"r"},
}

var testKeyMap = NewKeyMap(keytest.Table(testKeys))

func press(k string) tea.KeyPressMsg {
	switch k {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok {
		r, _ := utf8.DecodeRuneInString(c)
		return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// source serves files in pages of a fixed size.
type source struct {
	mu    sync.Mutex
	files []File
	size  int
	fail  error
	calls []string
}

func (s *source) fetch(_ context.Context, cursor string) ([]File, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, cursor)
	if s.fail != nil {
		return nil, cursor, s.fail
	}
	var from int
	if cursor != "" {
		from, _ = strconv.Atoi(cursor)
	}
	to := min(from+s.size, len(s.files))
	next := ""
	if to < len(s.files) {
		next = strconv.Itoa(to)
	}
	return s.files[from:to], next, nil
}

func (s *source) failWith(err error) {
	s.mu.Lock()
	s.fail = err
	s.mu.Unlock()
}

// patch is a modified-file patch of hunks hunks, each of a context line, a
// deleted line, an added line and a context line.
func patch(hunks int) string {
	var b strings.Builder
	for h := range hunks {
		start := 1 + h*20
		fmt.Fprintf(&b, "@@ -%d,3 +%d,3 @@ func f%d() {\n ctx a%d\n-old b%d\n+new b%d\n ctx c%d\n", start, start, h, h, h, h, h)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// goFile is a modified file of hunks hunks.
func goFile(path string, hunks int) File {
	return File{Path: path, Status: StatusModified, Additions: hunks, Deletions: hunks, Patch: patch(hunks)}
}

// sizedFiles is n modified files of hunks hunks each.
func sizedFiles(n, hunks int) []File {
	files := make([]File, n)
	for i := range files {
		files[i] = goFile(fmt.Sprintf("pkg/f%d.go", i), hunks)
	}
	return files
}

// run runs cmd and feeds what it returns to m, until no command is left.
func run(tb testing.TB, m Model, cmd tea.Cmd) Model {
	tb.Helper()
	for cmd != nil {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				m = run(tb, m, c)
			}
			return m
		}
		m, cmd = m.Update(msg)
	}
	return m
}

// load makes a focused view of size 80x12 over files, and fetches every page
// that it asks for.
func load(tb testing.TB, src *source, opts ...Option) Model {
	tb.Helper()
	opts = append([]Option{WithKeyMap(testKeyMap), WithSize(80, 12), WithFocused(true)}, opts...)
	m := New(src.fetch, opts...)
	return run(tb, m, m.Init())
}

// keys presses each key and runs the commands that result.
func keys(tb testing.TB, m Model, ks ...string) Model {
	tb.Helper()
	for _, k := range ks {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		m = run(tb, m, cmd)
	}
	return m
}

// assertFits checks that v is exactly height lines of exactly width cells.
func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, v)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Fatalf("line %d is %d cells wide, want %d: %q", i, w, width, l)
		}
	}
}

var errBoom = errors.New("GET /repos/o/r/pulls/1/files: 502 Bad Gateway")
