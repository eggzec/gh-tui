package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// errorStyles returns the error styles of the default palette on a dark or
// light terminal, marked with the Unicode set.
func errorStyles(tb testing.TB, dark bool) ErrorStyles {
	tb.Helper()
	p, err := config.Default().Palette(dark)
	if err != nil {
		tb.Fatal(err)
	}
	return NewTheme(p, dark).Errors(NewIcons(config.IconsUnicode))
}

// longReason is a reason from GitHub too long for two lines at 80 columns.
const longReason = "At least 2 approving reviews are required by reviewers with write access. " +
	"Required status check \"build (ubuntu-latest, 1.26)\" is expected. " +
	"Changes must be made through a pull request, and the branch must be up to date before merging."

func TestErrorLine(t *testing.T) {
	cases := append(slices.Clone(sayCases), sayCase{
		name: "rejected with a long reason", p: &core.Problem{Kind: core.Rejected, Action: "merge #5", Reason: longReason},
	})
	for _, theme := range []struct {
		name string
		dark bool
	}{{"dark", true}, {"light", false}} {
		for _, tt := range cases {
			t.Run(tt.name+"/"+theme.name, func(t *testing.T) {
				text, hint := Say(tt.p, testVoice())
				lines := ErrorLine(errorStyles(t, theme.dark), text, hint, 80)
				for _, l := range lines {
					if w := ansi.StringWidth(l); w > 80 {
						t.Errorf("line %q is %d cells wide, want at most 80", ansi.Strip(l), w)
					}
				}
				golden.RequireEqual(t, strings.Join(lines, "\n"))
			})
		}
	}
}

func TestErrorLineLayout(t *testing.T) {
	st := errorStyles(t, true)
	tests := []struct {
		name, text, hint string
		width            int
		want             []string
	}{
		{"one line", "Can't reach GitHub", "r to retry", 40, []string{"✗ Can't reach GitHub · r to retry"}},
		{"hint on its own line", "Can't reach GitHub", "r to retry", 26, []string{"✗ Can't reach GitHub", "  r to retry"}},
		{
			"text wraps with a hanging indent", "GitHub rejected the token. Run gh auth login.", "r to retry", 30,
			[]string{"✗ GitHub rejected the token.", "  Run gh auth login.", "  r to retry"},
		},
		{
			"text cut on the second line", "one two three four five six seven eight nine", "o to open on GitHub", 16,
			[]string{"✗ one two three", "  four five six…", "  o to open on", "  GitHub"},
		},
		{"no hint", "eggzec/x#5 doesn't exist or is private.", "", 80, []string{"✗ eggzec/x#5 doesn't exist or is private."}},
		{"nothing", "", "", 80, nil},
		{"too narrow for the mark", "Can't", "r to retry", 2, []string{"Ca", "n…", "r", "to", "re", "tr", "y"}},
		{"a path stays whole", "see ~/.local/state/gh-tui/gh-tui.log", "", 36, []string{"✗ see", "  ~/.local/state/gh-tui/gh-tui.log"}},
		{"wide characters", "変更できません", "", 4, []string{"✗ 変", "  …"}},
		{"a wide character wider than the row", "変更", "", 3, []string{"✗ …", "  …"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ErrorLine(st, tt.text, tt.hint, tt.width)
			for i := range got {
				got[i] = ansi.Strip(got[i])
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ErrorLine = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestErrorLineFits checks every problem at every width from 1 to 200: no
// line is wider than the width, the text takes at most two lines and only
// the last may end in "…", and the hint is all there.
func TestErrorLineFits(t *testing.T) {
	st := errorStyles(t, true)
	problems := make([]*core.Problem, 0, len(sayCases)+1)
	problems = append(problems, &core.Problem{Kind: core.Rejected, Reason: longReason})
	for _, tt := range sayCases {
		problems = append(problems, tt.p)
	}
	for _, p := range problems {
		text, hint := Say(p, testVoice())
		for width := 1; width <= 200; width++ {
			lines := ErrorLine(st, text, hint, width)
			var hintText strings.Builder
			textLines := 0
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > width {
					t.Fatalf("%s at %d: line %d %q is %d cells wide", p.Kind, width, i, ansi.Strip(l), w)
				}
				plain := ansi.Strip(l)
				if isHintOnly(st, l) {
					hintText.WriteString(plain)
					continue
				}
				textLines++
				if head, tail, ok := strings.Cut(plain, " · "); ok && hint != "" {
					plain = head
					hintText.WriteString(tail)
				}
				if strings.HasSuffix(plain, "…") && i != errorTextLines-1 {
					t.Errorf("%s at %d: line %d %q ends in … but isn't the last line of the text", p.Kind, width, i, plain)
				}
			}
			if textLines > errorTextLines {
				t.Errorf("%s at %d: the text takes %d lines, want at most %d", p.Kind, width, textLines, errorTextLines)
			}
			if got := hintText.String(); squeeze(got) != squeeze(hint) {
				t.Errorf("%s at %d: hint %q, want %q", p.Kind, width, got, hint)
			}
		}
	}
}

// isHintOnly reports whether line holds only the hint, on a line of its
// own, which starts in the hint's style after the indent.
func isHintOnly(st ErrorStyles, line string) bool {
	start, _, _ := strings.Cut(st.Hint.Render("x"), "x")
	return strings.HasPrefix(strings.TrimLeft(line, " "), start)
}

// squeeze drops the spaces of s, which wrapping moves.
func squeeze(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

// TestErrorLineIsClean checks that GitHub's reason reaches the screen
// without the escape sequences and invisible characters it came with.
func TestErrorLineIsClean(t *testing.T) {
	p := &core.Problem{Kind: core.Rejected, Reason: "\x1b[2JHead \u202eelif.exe\u202c is\r\nout of date" + invisible}
	text, hint := Say(p, testVoice())
	for width := 1; width <= 80; width++ {
		for _, l := range ErrorLine(errorStyles(t, true), text, hint, width) {
			if plain := ansi.Strip(l); strings.ContainsFunc(plain, isFormat) || strings.ContainsAny(plain, "\x1b\r\n") {
				t.Fatalf("at %d: line %q isn't clean", width, plain)
			}
		}
	}
}

func BenchmarkErrorLine(b *testing.B) {
	st := errorStyles(b, true)
	text, hint := Say(&core.Problem{Kind: core.Rejected, Reason: longReason}, testVoice())
	b.ReportAllocs()
	for b.Loop() {
		ErrorLine(st, text, hint, 80)
	}
}

// The ASCII set draws error lines in ASCII alone: its mark, the separator
// before the hint and the ellipsis of a cut text, at any width.
func TestErrorLineASCII(t *testing.T) {
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	st := NewTheme(p, true).Errors(NewIcons(config.IconsASCII))
	cut := false
	for width := 12; width <= 120; width++ {
		lines := ErrorLine(st, longReason, "r to retry", width)
		for _, l := range lines {
			plain := ansi.Strip(l)
			for i := range len(plain) {
				if plain[i] >= 0x80 {
					t.Fatalf("width %d: line %q has a byte beyond ASCII", width, plain)
				}
			}
			if w := ansi.StringWidth(plain); w > width {
				t.Errorf("width %d: line %q is %d cells", width, plain, w)
			}
			cut = cut || strings.Contains(plain, "...")
		}
	}
	if !cut {
		t.Error("the text was never cut with ...")
	}
	if got := ansi.Strip(ErrorLine(st, "Can't reach GitHub", "r to retry", 80)[0]); got != "x Can't reach GitHub - r to retry" {
		t.Errorf("line = %q, want the ASCII mark and separator", got)
	}
}
