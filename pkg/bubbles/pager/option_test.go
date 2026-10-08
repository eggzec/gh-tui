package pager

import (
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

func TestOptions(t *testing.T) {
	// Lines 2 to 4 and 6 to 7 are blank, and line 8 is only spaces.
	text := "Alpha\n\n\n\nalpha\n\n\n   \nALPHA\n"
	tests := []struct {
		name          string
		keys          []string
		wantWrap      bool
		wantNumbers   bool
		wantShown     []int
		wantCases     caseMode
		wantNote      string
		wantCapturing bool
		wantClose     bool
	}{
		{name: "minus waits for an option", keys: []string{"-"},
			wantNumbers: true, wantCapturing: true},
		{name: "S wraps long lines", keys: []string{"-", "S"},
			wantWrap: true, wantNumbers: true, wantNote: "Wrap long lines"},
		{name: "S again chops them", keys: []string{"-", "S", "-", "S"},
			wantNumbers: true, wantNote: "Chop long lines"},
		{name: "N hides the line numbers", keys: []string{"-", "N"},
			wantNote: "Hide line numbers"},
		{name: "N again shows them", keys: []string{"-", "N", "-", "N"},
			wantNumbers: true, wantNote: "Show line numbers"},
		{name: "s squeezes blank lines", keys: []string{"-", "s"},
			wantNumbers: true, wantShown: []int{1, 2, 5, 6, 8, 9}, wantNote: "Squeeze blank lines"},
		{name: "s again shows them all", keys: []string{"-", "s", "-", "s"},
			wantNumbers: true, wantNote: "Show all blank lines"},
		{name: "i matches case", keys: []string{"-", "i"},
			wantNumbers: true, wantCases: caseSensitive, wantNote: "Case is significant in searches"},
		{name: "i again ignores it unless there are capitals", keys: []string{"-", "i", "-", "i"},
			wantNumbers: true, wantNote: "Ignore case unless the pattern has capitals"},
		{name: "I ignores case", keys: []string{"-", "I"},
			wantNumbers: true, wantCases: caseIgnore, wantNote: "Ignore case in searches"},
		{name: "I again matches it", keys: []string{"-", "I", "-", "I"},
			wantNumbers: true, wantCases: caseSensitive, wantNote: "Case is significant in searches"},
		{name: "i after I ignores it unless there are capitals", keys: []string{"-", "I", "-", "i"},
			wantNumbers: true, wantNote: "Ignore case unless the pattern has capitals"},
		{name: "an unknown option says so", keys: []string{"-", "x"},
			wantNumbers: true, wantNote: "No such option: -x"},
		{name: "w and # are no keys", keys: []string{"w", "#"}, wantNumbers: true},
		{name: "esc cancels", keys: []string{"-", "esc"}, wantNumbers: true},
		{name: "then closes", keys: []string{"-", "esc", "esc"}, wantNumbers: true, wantClose: true},
		{name: "q is no option", keys: []string{"-", "q"}, wantNumbers: true, wantNote: "No such option: -q"},
		{name: "the note goes at the next key", keys: []string{"-", "S", "j"}, wantWrap: true, wantNumbers: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "a.txt", text, WithSize(80, 12))
			m, msg := keys(t, m, tt.keys...)
			if m.Wrap() != tt.wantWrap || m.LineNumbers() != tt.wantNumbers {
				t.Errorf("wrap %v, line numbers %v; want %v, %v", m.Wrap(), m.LineNumbers(), tt.wantWrap, tt.wantNumbers)
			}
			want := tt.wantShown
			if want == nil {
				want = []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
			}
			if got := shownLines(m); !slices.Equal(got, want) {
				t.Errorf("shown %v, want %v", got, want)
			}
			if m.cases != tt.wantCases {
				t.Errorf("case mode %d, want %d", m.cases, tt.wantCases)
			}
			if m.flash != tt.wantNote {
				t.Errorf("note %q, want %q", m.flash, tt.wantNote)
			}
			status := lastLine(plain(m))
			if tt.wantNote != "" && !strings.HasPrefix(status, tt.wantNote) {
				t.Errorf("status %q doesn't start with the note", status)
			}
			if m.Capturing() != tt.wantCapturing {
				t.Errorf("capturing %v, want %v", m.Capturing(), tt.wantCapturing)
			}
			if tt.wantCapturing && !strings.HasPrefix(status, "-") {
				t.Errorf("status %q doesn't show the -", status)
			}
			if _, ok := msg.(CloseMsg); ok != tt.wantClose {
				t.Errorf("sent %#v, want close %v", msg, tt.wantClose)
			}
		})
	}
}

// The case options change how the next search and filter match.
func TestCaseOptions(t *testing.T) {
	text := "Alpha\nalpha\nALPHA\n"
	tests := []struct {
		name    string
		options []string
		pattern string
		want    int
	}{
		{name: "smart, lower case", pattern: "alpha", want: 3},
		{name: "smart, a capital", pattern: "Alpha", want: 1},
		{name: "sensitive", options: []string{"-", "i"}, pattern: "alpha", want: 1},
		{name: "ignore, a capital", options: []string{"-", "I"}, pattern: "Alpha", want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "a.txt", text, WithSize(40, 6))
			m, _ = keys(t, m, tt.options...)
			searched, _ := enterAll(t, m, "/", tt.pattern, "enter")
			if searched.Matches() != tt.want {
				t.Errorf("search: %d matches, want %d", searched.Matches(), tt.want)
			}
			filtered, _ := enterAll(t, m, "&", tt.pattern, "enter")
			if filtered.Shown() != tt.want {
				t.Errorf("filter: %d lines shown, want %d", filtered.Shown(), tt.want)
			}
		})
	}
}

// Squeeze keeps one blank line of a run among the lines a filter keeps,
// and stays on for new content, as wrap does.
func TestSqueeze(t *testing.T) {
	m := open(t, "a.txt", "a\n\nb\n\n\nc\n\n", WithSize(40, 12))
	m, _ = keys(t, m, "-", "s")
	if got := shownLines(m); !slices.Equal(got, []int{1, 2, 3, 4, 6, 7}) {
		t.Fatalf("squeezed %v", got)
	}
	m, _ = enterAll(t, m, "&", "!b", "enter")
	// The filter drops b, which joins the runs around it.
	if got := shownLines(m); !slices.Equal(got, []int{1, 2, 6, 7}) || m.kept != 6 {
		t.Errorf("filtered and squeezed %v, with %d kept; want 1 2 6 7 of 6", shownLines(m), m.kept)
	}
	m, _ = keys(t, m, "esc")
	if got := shownLines(m); !slices.Equal(got, []int{1, 2, 3, 4, 6, 7}) || m.Filter() != "" {
		t.Errorf("esc left %v, filter %q; want the squeeze only", got, m.Filter())
	}
	_ = m.SetContent("b.txt", "x\n\n\ny\n")
	if got := shownLines(m); !slices.Equal(got, []int{1, 2, 4}) {
		t.Errorf("new content shows %v, want it squeezed", got)
	}
	// Squeeze is no filter: esc closes.
	if _, msg := keys(t, m, "esc"); msg == nil {
		t.Error("esc didn't close with only squeeze on")
	}
}

// A large filtered file squeezes in the background, and says it is
// filtering until it is done.
func TestSqueezeInBackground(t *testing.T) {
	text := strings.Repeat("a\n\n\n", 100_000)
	m := open(t, "big.txt", text, WithSize(40, 11))
	m, run := typeFilter(t, m, "a|^$")
	m, _ = m.Update(run())
	m, _ = keys(t, m, "-")
	m, run = m.Update(press("s"))
	if run == nil || !strings.Contains(plain(m), "filtering…") {
		t.Fatalf("squeezing a large filter ran at once, status %q", lastLine(plain(m)))
	}
	m, _ = m.Update(run())
	if m.Shown() != 200_000 || m.kept != 300_000 {
		t.Errorf("%d lines shown of %d kept, want 200000 of 300000", m.Shown(), m.kept)
	}
}

// A pager takes the keys that name an option from its option key map, so
// a rebound option answers to its new key and not to the old one.
func TestOptionKeysAreRebound(t *testing.T) {
	rebound := func(action string) []string {
		if action == "pager_option.chop" {
			return []string{"W"}
		}
		return lookup.Of(action)
	}
	m := open(t, "a.txt", "a\n", WithSize(20, 4), WithKeyMap(NewKeyMap(keymap.Func(rebound))))
	if m.Wrap() {
		t.Fatal("a pager wraps at first")
	}
	m, _ = keys(t, m, "-", "S")
	if m.Wrap() || !strings.HasPrefix(m.flash, noteNoOption) {
		t.Errorf("-S: wrap %v, note %q, want S to name no option", m.Wrap(), m.flash)
	}
	m, _ = keys(t, m, "-", "W")
	if !m.Wrap() {
		t.Error("-W didn't wrap the lines")
	}
}

// A pager whose config binds no option key names no option.
func TestNoOptionKeys(t *testing.T) {
	bare := func(action string) []string {
		if strings.HasPrefix(action, "pager_option.") {
			return nil
		}
		return lookup.Of(action)
	}
	m := New(WithKeyMap(NewKeyMap(keymap.Func(bare))), WithSize(20, 4))
	m.Focus()
	m, _ = keys(t, m, "-", "S", "esc")
	if m.Wrap() || m.Capturing() {
		t.Errorf("wrap %v, capturing %v, want the option key to take one key and change nothing", m.Wrap(), m.Capturing())
	}
}

// The option key's help lists the keys that name an option as the config
// sets them, and option mode's help lists every option action with its key.
func TestOptionHelpFollowsKeys(t *testing.T) {
	if got := testKeys(t).Option.Help().Desc; got != "option: S N s i I" {
		t.Errorf("option help is %q, want the default keys", got)
	}
	rebound := func(action string) []string {
		switch action {
		case "pager_option.chop":
			return []string{"W"}
		case "pager_option.squeeze":
			return nil
		}
		return lookup.Of(action)
	}
	m := open(t, "a.txt", "a\n", WithSize(20, 4), WithKeyMap(NewKeyMap(keymap.Func(rebound))))
	if got := m.keys.Option.Help().Desc; got != "option: W N i I" {
		t.Errorf("option help is %q, want W N i I", got)
	}
	m, _ = keys(t, m, "-")
	want := map[string]string{
		"chop or wrap long lines": "W", "line numbers": "N", "squeeze blank lines": "",
		"smart case": "i", "ignore case": "I", "cancel": "esc",
	}
	got := map[string]string{}
	if first := m.ShortHelp()[0]; first.Help().Desc != "cancel" {
		t.Errorf("option mode's help starts with %q, want cancel first", first.Help().Desc)
	}
	for _, b := range m.ShortHelp() {
		if b.Enabled() {
			got[b.Help().Desc] = b.Help().Key
		} else {
			got[b.Help().Desc] = ""
		}
	}
	for desc, k := range want {
		if g, ok := got[desc]; !ok || g != k {
			t.Errorf("option mode help lists %q with key %q (listed %v), want %q", desc, g, ok, k)
		}
	}
}

// With no option key bound, the option key's help names no keys.
func TestOptionHelpWithoutOptionKeys(t *testing.T) {
	bare := func(action string) []string {
		if strings.HasPrefix(action, "pager_option.") {
			return nil
		}
		return lookup.Of(action)
	}
	if got := NewKeyMap(keymap.Func(bare)).Option.Help().Desc; got != "option" {
		t.Errorf("option help is %q, want %q", got, "option")
	}
}

// Rendered content explains the line numbers and chopping in the words
// of its parent, and other content doesn't.
func TestRenderedNotes(t *testing.T) {
	r := &wrapWords{src: words(40)}
	m := fresh(t, WithRenderedNotes("Rows numbered", "code only"), WithSize(30, 6))
	m.Focus()
	m.SetRendered("a.md", r.src, r.render)
	m, _ = keys(t, m, "-", "N")
	if m.flash != "Hide line numbers" || m.LineNumbers() {
		t.Errorf("-N says %q with numbers %v", m.flash, m.LineNumbers())
	}
	m, _ = keys(t, m, "-", "N")
	if m.flash != "Rows numbered" || !m.LineNumbers() {
		t.Errorf("-N back on says %q with numbers %v", m.flash, m.LineNumbers())
	}
	m, _ = keys(t, m, "-", "S")
	if m.flash != "Wrap long lines (code only)" || !m.Wrap() {
		t.Errorf("-S says %q with wrap %v", m.flash, m.Wrap())
	}
	_ = m.SetContent("a.txt", "plain")
	m, _ = keys(t, m, "-", "N")
	m, _ = keys(t, m, "-", "N")
	if m.flash != "Show line numbers" {
		t.Errorf("-N on plain content says %q", m.flash)
	}
	m, _ = keys(t, m, "-", "S")
	if m.flash != "Chop long lines" {
		t.Errorf("-S on plain content says %q", m.flash)
	}
}
