package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const label = "all 3 commits · abc1234…def5678"

func TestTitle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		mode  Mode
		path  string
		width int
		want  string
	}{
		{
			"this file", ThisFile, "internal/tui/model.go", 160,
			"Diff · " + label + " · internal/tui/model.go (2/7) · hunks only · 40%",
		},
		{
			"whole patch", WholePatch, "internal/tui/model.go", 160,
			"Whole patch · " + label + " · internal/tui/model.go (2/7) · hunks only · 40%",
		},
		{
			"a path cut in the middle", ThisFile, "internal/tui/prpanes/diffview/title_test.go", 100,
			"Diff · " + label + " · internal/tui/prpa…ew/title_test.go (2/7) · hunks only · 40%",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Title(tc.mode, label, tc.path, 2, 7, "hunks only", 40, tc.width)
			if got != tc.want {
				t.Errorf("Title = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTitleFitsTheWidth(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("directory/", 20) + "file.go"
	for _, path := range []string{"a.go", long, "dir/界界界界界界界界界界界界界界界界界界界界.go"} {
		for _, m := range []Mode{ThisFile, WholePatch} {
			for _, w := range []int{0, 1, 10, 40, 80, 160} {
				got := Title(m, label, path, 12, 345, "hunks only", 100, w)
				if sw := ansi.StringWidth(got); sw > w {
					t.Errorf("mode %d, width %d: title is %d wide: %q", m, w, sw, got)
				}
			}
		}
	}
}

func TestTitleKeepsTheEndOfALongPath(t *testing.T) {
	t.Parallel()
	got := Title(ThisFile, label, strings.Repeat("d/", 50)+"main.go", 1, 2, "hunks only", 0, 80)
	if !strings.Contains(got, "…") || !strings.Contains(got, "main.go (1/2) · hunks only · 0%") {
		t.Errorf("Title = %q, want the cut path to keep its end and the title its tail", got)
	}
}

func TestTitleClampsThePercent(t *testing.T) {
	t.Parallel()
	for pct, want := range map[int]string{-5: "0%", 250: "100%"} {
		if got := Title(ThisFile, label, "a.go", 1, 1, "hunks only", pct, 160); !strings.HasSuffix(got, want) {
			t.Errorf("pct %d: Title = %q, want suffix %q", pct, got, want)
		}
	}
}

func TestTitleHostileInput(t *testing.T) {
	t.Parallel()
	for name, path := range map[string]string{
		"ansi escapes":   "\x1b[31mred\x1b[0m/\x1b]8;;http://x\x07link\x1b]8;;\x07/\x1b[2Jfile.go",
		"control bytes":  "a\r\nb\tc\x00d\x7f.go",
		"huge name":      strings.Repeat("x", 10000),
		"huge wide name": strings.Repeat("界", 10000),
	} {
		for _, w := range []int{40, 80, 160} {
			got := Title(ThisFile, label, path, 1, 2, "hunks only", 50, w)
			if sw := ansi.StringWidth(got); sw > w {
				t.Errorf("%s, width %d: title is %d wide", name, w, sw)
			}
			if got != ansi.Strip(got) || strings.ContainsAny(got, "\x1b\x07\r\n\t\x00\x7f") {
				t.Errorf("%s, width %d: title carries control characters: %q", name, w, got)
			}
		}
	}
	got := Title(ThisFile, label, "\x1b[31mred\x1b[0m/file.go", 1, 2, "hunks only", 50, 160)
	if !strings.Contains(got, " · red/file.go (1/2)") {
		t.Errorf("Title = %q, want the escapes dropped and the text kept", got)
	}
}

func TestNote(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cur   int
		total int
		hints []string
		want  string
	}{
		{"hints", 12, 340, []string{"J/K file", "a whole patch", "/ search"}, "L12/340 · J/K file · a whole patch · / search"},
		{"one hint", 1, 1, []string{"/ search"}, "L1/1 · / search"},
		{"no hints", 0, 0, nil, "L0/0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Note(tc.cur, tc.total, tc.hints); got != tc.want {
				t.Errorf("Note = %q, want %q", got, tc.want)
			}
		})
	}
}
