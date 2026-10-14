package composer

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

var (
	ctrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	ctrlE = tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
)

const longComment = "This reproduces on main too. The config loader returns before " +
	"it reads the defaults, so every field ends up empty and the app starts " +
	"with no keys at all."

// memDrafts is Drafts in memory, which counts what it was asked.
type memDrafts struct {
	mu    sync.Mutex
	texts map[string]string
	err   error
}

func newMem(texts map[string]string) *memDrafts {
	if texts == nil {
		texts = map[string]string{}
	}
	return &memDrafts{texts: texts}
}

func (d *memDrafts) Load(key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.texts[key], d.err
}

func (d *memDrafts) Save(key, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.TrimSpace(text) == "" {
		delete(d.texts, key)
	} else {
		d.texts[key] = text
	}
	return d.err
}

func (d *memDrafts) Delete(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.texts, key)
	return d.err
}

// get returns the draft under "k", the key most tests use.
func (d *memDrafts) get() (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.texts["k"]
	return t, ok
}

func testKeys(tb testing.TB) KeyMap {
	tb.Helper()
	return NewKeyMap(keytest.Table(map[string][]string{
		"submit": {"ctrl+s"},
		"cancel": {"esc"},
		"edit":   {"ctrl+e"},
	}))
}

// focused returns a focused composer for key with the keys of the app.
func focused(tb testing.TB, key string, opts ...Option) Model {
	tb.Helper()
	m := New(key, append([]Option{WithKeyMap(testKeys(tb)), WithSize(40, 8), WithTitle("Comment")}, opts...)...)
	m.Focus()
	return m
}

func typeText(m Model, text string) Model {
	for _, r := range text {
		if r == '\n' {
			m, _ = m.Update(enter)
			continue
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// run returns the message of cmd, or nil.
func run(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys(t))
	keytest.Tagged(t, testKeys(t))
	keytest.HelpTags(t, testKeys(t))
	keytest.NoConflicts(t, testKeys(t))
}

// Enter adds a line, the text wraps to the width, and the view fits.
func TestEditAndWrap(t *testing.T) {
	m := focused(t, "k")
	m = typeText(m, "one\ntwo "+longComment)
	if !strings.HasPrefix(m.Value(), "one\ntwo This") {
		t.Errorf("value %q", m.Value())
	}
	for l := range strings.SplitSeq(m.View(), "\n") {
		if w := ansi.StringWidth(l); w != 40 {
			t.Errorf("row %q is %d wide, want 40", ansi.Strip(l), w)
		}
	}
	if got := len(strings.Split(m.View(), "\n")); got != 8 {
		t.Errorf("view has %d rows, want 8", got)
	}
}

func TestSubmitRefusedWhenBlank(t *testing.T) {
	for _, text := range []string{"", "   ", "\n \n"} {
		m := typeText(focused(t, "k"), text)
		m, cmd := m.Update(ctrlS)
		if cmd != nil {
			t.Errorf("%q submitted: %v", text, run(cmd))
		}
		if !strings.Contains(ansi.Strip(m.View()), noteEmpty) {
			t.Errorf("%q: the view doesn't say there is nothing to submit", text)
		}
		// The note goes with the next key.
		m = typeText(m, "a")
		if strings.Contains(ansi.Strip(m.View()), noteEmpty) {
			t.Errorf("%q: the note stayed after a key", text)
		}
	}
}

func TestSubmitKeepsDraftUntilDiscarded(t *testing.T) {
	d := newMem(nil)
	m := typeText(focused(t, "k", WithDrafts(d)), "LGTM")
	m, cmd := m.Update(ctrlS)
	got, ok := run(cmd).(SubmitMsg)
	if !ok || got.Text != "LGTM" || got.Key != "k" || got.ID != m.ID() || got.Err != nil {
		t.Fatalf("got %+v", got)
	}
	// The send may fail, so the draft is there until the parent says it went.
	if text, _ := d.get(); text != "LGTM" {
		t.Errorf("draft after submit %q, want LGTM", text)
	}
	msg, ok := run(m.Discard()).(DiscardedMsg)
	if !ok || msg.Err != nil {
		t.Fatalf("discard gave %+v", msg)
	}
	if _, ok := d.get(); ok {
		t.Error("the draft is still there after Discard")
	}
}

func TestEscKeepsDraft(t *testing.T) {
	d := newMem(nil)
	m := typeText(focused(t, "k", WithDrafts(d)), "half a thought")
	_, cmd := m.Update(esc)
	got, ok := run(cmd).(CancelMsg)
	if !ok || got.Text != "half a thought" || got.ID != m.ID() {
		t.Fatalf("got %+v", got)
	}
	if text, _ := d.get(); text != "half a thought" {
		t.Errorf("draft %q, want the text", text)
	}
	// Closing it emptied drops the draft: load "old", clear it, press esc.
	d2 := newMem(map[string]string{"k2": "old"})
	m = focused(t, "k2", WithDrafts(d2))
	m, _ = m.Update(run(m.Init()))
	if m.Value() != "old" {
		t.Fatalf("draft not loaded: %q", m.Value())
	}
	m.SetValue("")
	_, cmd = m.Update(esc)
	got, ok = run(cmd).(CancelMsg)
	if !ok || got.Text != "" {
		t.Fatalf("esc gave %+v", got)
	}
	if text, ok := d2.texts["k2"]; ok {
		t.Errorf("the draft %q is still kept after closing it empty", text)
	}
}

func TestRestoreByKey(t *testing.T) {
	d := newMem(map[string]string{"a": "draft of a", "b": "draft of b"})
	m := focused(t, "b", WithDrafts(d))
	m, _ = m.Update(run(m.Init()))
	if m.Value() != "draft of b" || !m.Restored() {
		t.Errorf("value %q, restored %v", m.Value(), m.Restored())
	}
	// A draft that arrives after typing doesn't replace it.
	m = focused(t, "a", WithDrafts(d))
	msg := run(m.Init())
	m = typeText(m, "new")
	m, _ = m.Update(msg)
	if m.Value() != "new" || m.Restored() {
		t.Errorf("value %q, restored %v; want the typed text", m.Value(), m.Restored())
	}
	// Another composer's message is not this one's.
	m = focused(t, "a", WithDrafts(d))
	other := focused(t, "a", WithDrafts(d))
	m, _ = m.Update(run(other.Init()))
	if m.Value() != "" {
		t.Errorf("took another composer's draft: %q", m.Value())
	}
	// A draft that fails to load is none.
	bad := newMem(map[string]string{"a": "x"})
	bad.err = errors.New("damaged")
	m = focused(t, "a", WithDrafts(bad))
	m, _ = m.Update(run(m.Init()))
	// The fake returns the text along with the error, as a damaged file may.
	if m.Value() != "" || m.Restored() {
		t.Errorf("value %q, restored %v; want none", m.Value(), m.Restored())
	}
}

// ctrl+e opens the editor, and the text area's own use of it, the end of the
// line, stays on the End key.
func TestEditKeyAndLineEnd(t *testing.T) {
	m := focused(t, "k", WithEditor("vim"))
	m = typeText(m, "ab")
	m.exec, m.tempDir = fakeEditor("edited", nil), t.TempDir()
	m, cmd := m.Update(ctrlE)
	if cmd == nil {
		t.Fatal("ctrl+e opened no editor")
	}
	if m.Value() != "ab" {
		t.Errorf("ctrl+e changed the text to %q", m.Value())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = typeText(m, "!")
	if m.Value() != "ab!" {
		t.Errorf("home, end, ! gave %q, want ab!", m.Value())
	}
}

// Wide characters and tabs keep every row at the width.
func TestWideAndTabs(t *testing.T) {
	m := focused(t, "k", WithSize(20, 8))
	text := "日本語のコメント 😀 tab\there"
	m = typeText(m, text)
	for l := range strings.SplitSeq(m.View(), "\n") {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("row %q is %d wide, want 20", ansi.Strip(l), w)
		}
	}
	if !strings.HasPrefix(m.Value(), "日本語のコメント 😀 tab") {
		t.Errorf("value %q lost characters", m.Value())
	}
	// A tab key neither moves the focus nor inserts a tab.
	before := m.Value()
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !m.Focused() || strings.Contains(strings.TrimPrefix(m.Value(), before), "\t") {
		t.Errorf("tab key: focused %v, value %q", m.Focused(), m.Value())
	}
}

// ctrl+c is the parent's to handle: Save writes the text at once.
func TestSaveBeforeQuit(t *testing.T) {
	d := newMem(nil)
	m := typeText(focused(t, "k", WithDrafts(d)), "unsent")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if text, _ := d.get(); text != "unsent" {
		t.Errorf("draft %q, want unsent", text)
	}
	// Without drafts there is nothing to save, and no error.
	if err := focused(t, "k").Save(); err != nil {
		t.Errorf("Save without drafts: %v", err)
	}
}

// fakeEditor makes the editor leave text in the file, or fail.
func fakeEditor(text string, err error) func(*exec.Cmd, tea.ExecCallback) tea.Cmd {
	return func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
		return func() tea.Msg {
			if err == nil {
				if werr := os.WriteFile(c.Args[len(c.Args)-1], []byte(text), 0o600); werr != nil {
					return done(werr)
				}
			}
			return done(err)
		}
	}
}

func TestEditor(t *testing.T) {
	d := newMem(nil)
	m := typeText(focused(t, "k", WithDrafts(d), WithEditor("vim")), "short")
	m.exec, m.tempDir = fakeEditor("from the editor\nline two\n", nil), t.TempDir()
	m, cmd := m.Update(ctrlE)
	m, save := m.Update(run(cmd))
	if m.Value() != "from the editor\nline two" {
		t.Errorf("value %q", m.Value())
	}
	run(save)
	if text, _ := d.get(); text != "from the editor\nline two" {
		t.Errorf("draft %q", text)
	}

	// A failing editor leaves the text and says so.
	m.exec = fakeEditor("", errors.New("exit status 1\nmore"))
	m, cmd = m.Update(ctrlE)
	m, _ = m.Update(run(cmd))
	if m.Value() != "from the editor\nline two" || !strings.Contains(ansi.Strip(m.View()), "editor: exit status 1") {
		t.Errorf("value %q after a failing editor", m.Value())
	}
}

func TestNoEditor(t *testing.T) {
	m := focused(t, "k")
	m.getenv = func(string) string { return "" }
	m, cmd := m.Update(ctrlE)
	if cmd != nil || !strings.Contains(ansi.Strip(m.View()), noteNoEditor) {
		t.Errorf("cmd %v; the view doesn't say there is no editor", cmd != nil)
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := focused(t, "k")
	m.Blur()
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if cmd != nil || m.Value() != "" {
		t.Errorf("a blurred composer took a key: %q", m.Value())
	}
}

func BenchmarkView(b *testing.B) {
	m := typeText(focused(b, "k", WithSize(80, 12)), longComment)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := typeText(focused(b, "k", WithSize(80, 12)), longComment)
	key := tea.KeyPressMsg{Code: 'x', Text: "x"}
	b.ReportAllocs()
	for b.Loop() {
		m, _ = m.Update(key)
	}
}

func TestView(t *testing.T) {
	for _, width := range []int{80, 40} {
		for _, tt := range []struct{ name, text string }{
			{"empty", ""},
			{"typed", "Thanks, **fixed** on main.\n\nSee #12."},
			{"long", longComment},
		} {
			t.Run(tt.name+"-"+strconv.Itoa(width), func(t *testing.T) {
				m := focused(t, "k", WithSize(width, 7), WithPlaceholder("Write a comment…"))
				m = typeText(m, tt.text)
				v := m.View()
				for l := range strings.SplitSeq(v, "\n") {
					if w := ansi.StringWidth(l); w != width {
						t.Errorf("row %q is %d wide, want %d", ansi.Strip(l), w, width)
					}
				}
				golden.RequireEqual(t, v)
			})
		}
	}
}
