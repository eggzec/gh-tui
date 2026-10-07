package finder

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// testKeys returns the keys of a finder, as the app sets them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	table := map[string][]string{
		"up":        {"up", "ctrl+p"},
		"down":      {"down", "ctrl+n"},
		"page_up":   {"pgup"},
		"page_down": {"pgdown"},
		"choose":    {"enter"},
		"cancel":    {"esc"},
	}
	return NewKeyMap(keytest.Table(table))
}

// newKeyed returns a finder with the keys of the app.
func newKeyed(tb testing.TB, load Load, opts ...Option) Model {
	tb.Helper()
	return New(load, append([]Option{WithKeyMap(testKeys(tb))}, opts...)...)
}

// items returns an item of each path.
func items(paths ...string) []Item {
	out := make([]Item, len(paths))
	for i, p := range paths {
		out[i] = Item{Path: p}
	}
	return out
}

// corpusOf prepares paths for matching.
func corpusOf(tb testing.TB, paths ...string) *corpus {
	tb.Helper()
	c, err := newCorpus(context.Background(), items(paths...))
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

// ranked returns the paths of c that match query, best first.
func ranked(tb testing.TB, c *corpus, query string, recent map[int32]int) []string {
	tb.Helper()
	res, ok := filter(context.Background(), c, query, nil, recent)
	if !ok {
		tb.Fatal("filter stopped early")
	}
	return pathsOf(c, res)
}

func pathsOf(c *corpus, res *result) []string {
	out := make([]string, len(res.items))
	for i, it := range res.items {
		out[i] = c.items[it].Path
	}
	return out
}

// Words that the paths of a large repository are made of.
var (
	dirWords = []string{
		"pkg", "cmd", "internal", "api", "apis", "core", "v1", "v1beta1", "client", "server",
		"controller", "controllers", "util", "utils", "test", "e2e", "integration", "storage",
		"kubelet", "scheduler", "proxy", "network", "volume", "plugins", "framework", "runtime",
		"generated", "informers", "listers", "clientset", "typed", "fake", "vendor", "github.com",
		"golang.org", "x", "tools", "net", "http", "cache", "metrics", "admission", "auth",
		"docs", "hack", "staging", "src", "k8s.io", "apimachinery", "third_party", "examples",
	}
	nameWords = []string{
		"types", "doc", "register", "zz_generated", "deepcopy", "conversion", "defaults", "helpers",
		"validation", "strategy", "storage", "rest", "handler", "server", "client", "config",
		"options", "flags", "main", "util", "cache", "store", "reflector", "informer", "lister",
		"controller", "manager", "plugin", "volume", "attach", "detach", "mount", "node", "pod",
		"service", "endpoint", "ingress", "policy", "quota", "renderer", "cursed", "model", "view",
		"update", "keys", "styles", "event", "watch", "patch", "merge", "apply", "diff", "table",
	}
	exts = []string{".go", ".go", ".go", "_test.go", ".md", ".yaml", ".json", ".sh", ".proto", ".txt"}
)

// bigPaths returns n distinct paths shaped like those of a large Go
// repository, the same for every run.
func bigPaths(n int) []string {
	r := rand.New(rand.NewPCG(1, 2))
	seen := make(map[string]bool, n)
	out := make([]string, 0, n)
	for len(out) < n {
		var b strings.Builder
		for range 1 + r.IntN(7) {
			b.WriteString(dirWords[r.IntN(len(dirWords))])
			b.WriteByte('/')
		}
		b.WriteString(nameWords[r.IntN(len(nameWords))])
		if r.IntN(3) == 0 {
			b.WriteByte('_')
			b.WriteString(nameWords[r.IntN(len(nameWords))])
		}
		if r.IntN(8) == 0 {
			fmt.Fprintf(&b, "%d", r.IntN(100))
		}
		b.WriteString(exts[r.IntN(len(exts))])
		p := b.String()
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// loader returns a Load of the paths.
func loader(paths ...string) Load {
	return func(context.Context) (Listing, error) { return Listing{Items: items(paths...)}, nil }
}

// open returns a focused finder of width by height over the paths, loaded.
func open(tb testing.TB, width, height int, paths []string, opts ...Option) Model {
	tb.Helper()
	opts = append([]Option{WithSize(width, height)}, opts...)
	m := newKeyed(tb, loader(paths...), opts...)
	m.Focus()
	return run(tb, m, m.Init())
}

// run executes cmd and feeds every resulting message back into m, until
// no commands are left. Spinner ticks are dropped so tests never sleep,
// and the messages for the parent are dropped too.
func run(tb testing.TB, m Model, cmd tea.Cmd) Model {
	tb.Helper()
	m, _ = collect(tb, m, cmd)
	return m
}

// collect runs cmd as run does, and returns the messages for the parent.
func collect(tb testing.TB, m Model, cmd tea.Cmd) (_ Model, out []tea.Msg) {
	tb.Helper()

	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, spinner.TickMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case ChosenMsg, CancelMsg:
			out = append(out, msg)
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			queue = append(queue, next)
		}
	}
	return m, out
}

// typed types text into m and runs what it returns.
func typed(tb testing.TB, m Model, text string) Model {
	tb.Helper()
	for _, r := range text {
		m = keys(tb, m, string(r))
	}
	return m
}

// keys presses each key and runs what it returns.
func keys(tb testing.TB, m Model, ks ...string) Model {
	tb.Helper()
	for _, k := range ks {
		var cmd tea.Cmd
		m, cmd = m.Update(press(k))
		m = run(tb, m, cmd)
	}
	return m
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+n":
		return tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	case "ctrl+p":
		return tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// selected returns the path of the selected item, or "".
func selected(m Model) string {
	it, _ := m.Selected()
	return it.Path
}

// assertFits checks that v is exactly height lines of exactly width cells.
func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Fatalf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("line %d is %d cells wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}
