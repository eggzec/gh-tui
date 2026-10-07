package picker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

var (
	enter    = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc      = tea.KeyPressMsg{Code: tea.KeyEscape}
	up       = tea.KeyPressMsg{Code: tea.KeyUp}
	down     = tea.KeyPressMsg{Code: tea.KeyDown}
	pgDown   = tea.KeyPressMsg{Code: tea.KeyPgDown}
	pgUp     = tea.KeyPressMsg{Code: tea.KeyPgUp}
	ctrlN    = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
	ctrlP    = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	tab      = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	bksp     = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

const (
	kindRepos  = "Repositories"
	kindIssues = "Issues"
	kindPulls  = "Pull requests"
)

var (
	ghTUI    = Item{Kind: kindRepos, Title: "eggzec/gh-tui", Detail: "A GitHub client for the terminal", Value: "repo:eggzec/gh-tui"}
	dotfiles = Item{Kind: kindRepos, Title: "octo-org/dotfiles", Value: "repo:octo-org/dotfiles"}
	crash    = Item{Kind: kindIssues, Title: "Crash when the config file is empty", Detail: "octo-org/hello#42", Value: "issue:42"}
	fix      = Item{Kind: kindPulls, Title: "Fix the crash on an empty config", Detail: "eggzec/gh-tui#7", Value: "pull:7"}
	catalog  = []Item{ghTUI, dotfiles, crash, fix}
)

// testKeys returns the keys of a picker, as the app sets them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	table := map[string][]string{
		"up":                           {"up", "ctrl+p"},
		"down":                         {"down", "ctrl+n"},
		"page_up":                      {"pgup"},
		"page_down":                    {"pgdown"},
		"choose":                       {"enter"},
		"cancel":                       {"esc"},
		"next_scope":                   {"tab"},
		"prev_scope":                   {"shift+tab"},
		"picker_normal.up":             {"k", "up"},
		"picker_normal.down":           {"j", "down"},
		"picker_normal.page_up":        {"ctrl+b", "pgup"},
		"picker_normal.page_down":      {"ctrl+f", "pgdown"},
		"picker_normal.half_page_up":   {"ctrl+u"},
		"picker_normal.half_page_down": {"ctrl+d"},
		"picker_normal.top":            {"g", "home"},
		"picker_normal.bottom":         {"G", "end"},
		"picker_normal.insert":         {"i"},
		"picker_normal.append":         {"a"},
	}
	return NewKeyMap(keytest.Table(table))
}

var errBoom = errors.New("github: 502 Bad Gateway")

// fakeSearch searches catalog for titles that contain the text, and lists
// the repositories for an empty text. It records the queries it gets, and
// whether their context was already cancelled.
type fakeSearch struct {
	mu        sync.Mutex
	queries   []Query
	cancelled []bool
	// fail, when set, is returned instead of results.
	fail error
}

func (f *fakeSearch) search(ctx context.Context, q Query) ([]Item, error) {
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.cancelled = append(f.cancelled, ctx.Err() != nil)
	fail := f.fail
	f.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	var out []Item
	for _, it := range catalog {
		if q.Scope != "" && it.Kind != q.Scope {
			continue
		}
		if q.Text == "" && it.Kind != kindRepos {
			continue
		}
		if strings.Contains(strings.ToLower(it.Title), strings.ToLower(q.Text)) {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeSearch) Queries() []Query {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.queries)
}

// run runs cmd and feeds what it sends back into m, as the program would,
// until nothing is left. Spinner ticks are dropped, since they go on for as
// long as a search runs. It returns the messages meant for the parent.
func run(tb testing.TB, m Model, cmd tea.Cmd) (after Model, sent []tea.Msg) {
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
			sent = append(sent, msg)
		default:
			var next tea.Cmd
			m, next = m.Update(msg)
			queue = append(queue, next)
		}
	}
	return m, sent
}

// press sends msgs in order and runs what each returns.
func press(tb testing.TB, m Model, msgs ...tea.Msg) (after Model, sent []tea.Msg) {
	tb.Helper()
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		var more []tea.Msg
		m, more = run(tb, m, cmd)
		sent = append(sent, more...)
	}
	return m, sent
}

// typeText types each rune of text and runs what each key returns.
func typeText(tb testing.TB, m Model, text string) Model {
	tb.Helper()
	for _, r := range text {
		m, _ = press(tb, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// open returns a focused picker that has run its first search, and never
// waits to search.
func open(tb testing.TB, search Search, opts ...Option) Model {
	tb.Helper()
	m := New(search, append([]Option{WithKeyMap(testKeys(tb)), WithDebounce(0), WithSize(60, 12)}, opts...)...)
	m.Focus()
	m, _ = run(tb, m, m.Init())
	return m
}

func titlesOf(m Model) []string {
	out := make([]string, len(m.results))
	for i := range m.results {
		out[i] = m.results[i].Title
	}
	return out
}
