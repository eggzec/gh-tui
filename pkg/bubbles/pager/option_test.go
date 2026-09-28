package pager

import (
	"slices"
	"strings"
	"testing"
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
