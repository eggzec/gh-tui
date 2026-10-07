package filterform

import (
	"context"
	"errors"
	"sync"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// testKeys returns the keys of a filter form, as the app sets them.
func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	table := map[string][]string{
		"up":                           {"k", "up"},
		"down":                         {"j", "down"},
		"top":                          {"g", "home"},
		"bottom":                       {"G", "end"},
		"left":                         {"h", "left"},
		"right":                        {"l", "right"},
		"toggle":                       {"space"},
		"insert":                       {"i"},
		"append":                       {"a"},
		"clear":                        {"delete", "backspace"},
		"global.next_tab":              {"]"},
		"global.prev_tab":              {"["},
		"global.select":                {"enter"},
		"global.dismiss":               {"esc"},
		"global.quit":                  {"q"},
		"global.refresh":               {"r"},
		"picker.toggle":                {"space"},
		"filter_query.apply":           {"enter"},
		"filter_query.cancel":          {"esc"},
		"filter_query.up":              {"up", "ctrl+p"},
		"filter_query.down":            {"down", "ctrl+n"},
		"picker.up":                    {"up", "ctrl+p"},
		"picker.down":                  {"down", "ctrl+n"},
		"picker.page_up":               {"pgup"},
		"picker.page_down":             {"pgdown"},
		"picker.choose":                {"enter"},
		"picker.cancel":                {"esc"},
		"picker.next_scope":            {"tab"},
		"picker.prev_scope":            {"shift+tab"},
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

// newKeyed returns a form with the keys of the app.
func newKeyed(tb testing.TB, spec Spec, opts ...Option) Model {
	tb.Helper()
	return New(spec, append([]Option{WithKeyMap(testKeys(tb))}, opts...)...)
}

var (
	enter   = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc     = tea.KeyPressMsg{Code: tea.KeyEscape}
	up      = tea.KeyPressMsg{Code: tea.KeyUp}
	down    = tea.KeyPressMsg{Code: tea.KeyDown}
	left    = tea.KeyPressMsg{Code: tea.KeyLeft}
	right   = tea.KeyPressMsg{Code: tea.KeyRight}
	space   = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	bksp    = tea.KeyPressMsg{Code: tea.KeyBackspace}
	del     = tea.KeyPressMsg{Code: tea.KeyDelete}
	keyX    = tea.KeyPressMsg{Code: 'x', Text: "x"}
	keyR    = tea.KeyPressMsg{Code: 'r', Text: "r"}
	keyF    = tea.KeyPressMsg{Code: 'F', Text: "F"}
	ctrlU   = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	nextTab = tea.KeyPressMsg{Code: ']', Text: "]"}
	prevTab = tea.KeyPressMsg{Code: '[', Text: "["}
	keyI    = tea.KeyPressMsg{Code: 'i', Text: "i"}
	keyA    = tea.KeyPressMsg{Code: 'a', Text: "a"}
	keyJ    = tea.KeyPressMsg{Code: 'j', Text: "j"}
	keyK    = tea.KeyPressMsg{Code: 'k', Text: "k"}
	keyH    = tea.KeyPressMsg{Code: 'h', Text: "h"}
	keyL    = tea.KeyPressMsg{Code: 'l', Text: "l"}
	keyG    = tea.KeyPressMsg{Code: 'g', Text: "g"}
	keyBigG = tea.KeyPressMsg{Code: 'G', Text: "G", Mod: tea.ModShift}
	keyQ    = tea.KeyPressMsg{Code: 'q', Text: "q"}
)

// Rows of prSpec.
const (
	rowState = iota
	rowAuthor
	rowReview
	rowLabels
	rowDrafts
	rowBase
	rowQuery
)

const prDefaults = "is:open author:@me review-requested:@me label:bug,enhancement base:main sort:updated-desc"

var errBoom = errors.New("github: 502 Bad Gateway")

// labels is what fakeLoader loads.
var labels = []Item{
	{Label: "bug", Value: "bug"},
	{Label: "enhancement", Value: "enhancement"},
	{Label: "docs", Value: "docs", Detail: "Documentation"},
	{Label: "good first issue", Value: "good first issue"},
}

// fakeLoader loads labels, or fails with fail. It records the queries it
// gets and whether their context was already cancelled.
type fakeLoader struct {
	mu        sync.Mutex
	queries   []string
	cancelled []bool
	fail      error
	// block, when set, is waited on before returning.
	block chan struct{}
}

func (f *fakeLoader) load(ctx context.Context, query string) ([]Item, error) {
	f.mu.Lock()
	f.queries = append(f.queries, query)
	f.cancelled = append(f.cancelled, ctx.Err() != nil)
	fail, block := f.fail, f.block
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	if fail != nil {
		return nil, fail
	}
	return labels, nil
}

func (f *fakeLoader) setFail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = err
}

func (f *fakeLoader) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queries)
}

// prSpec is the spec of a pull request list, as in the mockup. load, when
// set, loads the labels.
func prSpec(load Loader) Spec {
	return Spec{
		Fields: []Field{
			{
				Key: "state", Label: "State", Kind: Choice, Qualifier: "is",
				Options: []Item{{"Open", "open", ""}, {"Closed", "closed", ""}, {"Merged", "merged", ""}, {"All", "", ""}},
				Default: TextValue("open"),
			},
			{
				Key: "author", Label: "Author", Kind: Person, Qualifier: "author",
				Options: []Item{{"@me", "@me", "you"}, {"octocat", "octocat", ""}},
				Default: TextValue("@me"), Hint: "anyone",
			},
			{
				Key: "review", Label: "Review", Kind: Choice,
				Options: []Item{
					{"Any", "", ""},
					{"Requested from me", "review-requested:@me", ""},
					{"Approved", "review:approved", ""},
					{"Changes requested", "review:changes_requested", ""},
				},
				Default: TextValue("review-requested:@me"),
			},
			{
				Key: "labels", Label: "Labels", Kind: Multi, Qualifier: "label", Load: load,
				Default: ListValue("bug", "enhancement"), Empty: "No labels in this repository.",
			},
			{Key: "drafts", Label: "Drafts", Kind: Toggle, Qualifier: "-is:draft", Hint: "hide drafts"},
			{Key: "base", Label: "Base", Kind: Text, Qualifier: "base", Default: TextValue("main"), Hint: "any branch"},
		},
		Sort: &SortField{
			Options: []SortOption{
				{Label: "Updated", Value: "updated", Desc: "Newest first", Asc: "Oldest first"},
				{Label: "Created", Value: "created", Desc: "Newest first", Asc: "Oldest first"},
				{Label: "Comments", Value: "comments", Desc: "Most first", Asc: "Fewest first"},
			},
			Default: Sort{By: "updated", Desc: true},
		},
	}
}

// run runs cmd and feeds what it sends back into m, as the program would,
// until nothing is left. Spinner ticks are dropped, since they go on for
// as long as a load runs. It returns the messages meant for the parent.
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
		case AppliedMsg, CancelMsg:
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

// typeText types each rune of text.
func typeText(tb testing.TB, m Model, text string) Model {
	tb.Helper()
	for _, r := range text {
		m, _ = press(tb, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// open returns a focused form of spec.
func open(tb testing.TB, spec Spec, opts ...Option) Model {
	tb.Helper()
	m := newKeyed(tb, spec, append([]Option{WithSize(60, 20)}, opts...)...)
	m.Focus()
	return m
}

// keys returns n presses of k.
func keys(k tea.Msg, n int) []tea.Msg {
	out := make([]tea.Msg, n)
	for i := range out {
		out[i] = k
	}
	return out
}
