package tree

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	long := strings.Repeat("a-very-long-directory-name-", 4)
	tests := []struct {
		name  string
		model func(t *testing.T) Model
	}{
		{"loading", func(*testing.T) Model {
			return New(repo().children, WithSize(40, 5), WithFocused(true))
		}},
		{"loaded", func(t *testing.T) Model {
			t.Helper()
			return load(t, repo())
		}},
		{"nested", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, repo(), WithSize(40, 12)), "*", "j", "j", "j", "j", "*", "j", "l")
		}},
		{"blurred", func(t *testing.T) Model {
			t.Helper()
			m := keys(t, load(t, repo()), "l", "l")
			m.Blur()
			return m
		}},
		{"branch loading", func(t *testing.T) Model {
			t.Helper()
			m := keys(t, load(t, repo()), "j", "j")
			m, _ = m.Update(press("+"))
			return m
		}},
		{"branch error", func(t *testing.T) Model {
			t.Helper()
			f := repo()
			f.setFail("internal", errors.New("GET /repos/o/r/contents/internal: 502 Bad Gateway"))
			return keys(t, load(t, f, WithSize(60, 6)), "j", "j", "+")
		}},
		{"root error", func(t *testing.T) Model {
			t.Helper()
			f := repo()
			f.setFail("", errors.New("API rate limit exceeded"))
			return load(t, f, WithSize(60, 3))
		}},
		{"empty", func(t *testing.T) Model {
			t.Helper()
			return load(t, newFiles(), WithSize(40, 3), WithEmptyText("This repository is empty."))
		}},
		{"truncated at 80 columns", func(t *testing.T) Model {
			t.Helper()
			f := newFiles(long+"/"+long+"/"+long+".go", "short.go")
			f.setFail(long+"/"+long, errors.New("dial tcp: i/o timeout"))
			return keys(t, load(t, f, WithSize(80, 4)), "l", "l", "+")
		}},
		{"icons", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, repo(), WithIcons(boxIcons)), "l")
		}},
		{"icons with details", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, sized(), WithSize(26, 6), WithIcons(boxIcons)), "l")
		}},
		{"icons with details dropped", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, sized(), WithSize(18, 6), WithIcons(boxIcons)), "l")
		}},
		{"icons on a branch error", func(t *testing.T) Model {
			t.Helper()
			f := repo()
			f.setFail("internal", errors.New("GET /repos/o/r/contents/internal: 502 Bad Gateway"))
			return keys(t, load(t, f, WithSize(40, 6), WithIcons(boxIcons)), "j", "j", "+")
		}},
		{"details", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, sized(), WithSize(40, 6)), "l")
		}},
		{"details truncate the name", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, sized(), WithSize(24, 6)), "l")
		}},
		{"details dropped when narrow", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, sized(), WithSize(16, 6)), "l")
		}},
		{"scrolled", func(t *testing.T) Model {
			t.Helper()
			m := keys(t, load(t, generated(4), WithSize(40, 8)), "*")
			return keys(t, m, "pgdown", "pgdown", "k")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			golden.RequireEqual(t, v)
		})
	}
}

// boxIcons draws an open or closed box before branches, and a dot before
// leaves.
func boxIcons(n Node, expanded bool) string {
	switch {
	case !n.Branch:
		return "·"
	case expanded:
		return "□"
	default:
		return "■"
	}
}

// sized is a small tree whose files carry their sizes as details.
func sized() *files {
	f := newFiles("docs/a-rather-long-file-name.md", "docs/b.md", "go.mod", "README.md")
	f.setDetail("docs/a-rather-long-file-name.md", "12K")
	f.setDetail("docs/b.md", "512B")
	f.setDetail("go.mod", "1.2K")
	f.setDetail("README.md", "2.1M")
	return f
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
			tb.Fatalf("line %d is %d cells wide, want %d: %q", i, w, width, l)
		}
	}
}

func TestViewFitsAnySize(t *testing.T) {
	f := repo()
	f.setFail("internal", errors.New("boom"))
	m := keys(t, load(t, f, WithIcons(boxIcons)), "*", "j", "j", "+")
	for _, size := range [][2]int{{1, 1}, {2, 3}, {3, 1}, {7, 4}, {80, 40}, {200, 2}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m.SetSize(size[0], size[1])
			assertFits(t, m.View(), size[0], size[1])
		})
	}
	m.SetSize(0, 10)
	if m.View() != "" {
		t.Fatal("zero width should render nothing")
	}
}

func TestIconsAskedOncePerState(t *testing.T) {
	asked := map[string]int{}
	icons := func(n Node, expanded bool) string {
		asked[fmt.Sprintf("%s %v", n.ID, expanded)]++
		return boxIcons(n, expanded)
	}
	m := keys(t, load(t, repo(), WithIcons(icons)), "*")
	for range 3 {
		_ = m.View()
		m = keys(t, m, "enter", "j")
	}
	for k, n := range asked {
		if n != 1 {
			t.Errorf("asked for %q %d times, want once", k, n)
		}
	}
	for _, k := range []string{"cmd true", "cmd false", "go.mod false"} {
		if asked[k] != 1 {
			t.Errorf("never asked for %q", k)
		}
	}
	if n := asked["go.mod true"]; n != 0 {
		t.Errorf("asked for an expanded leaf %d times, want never", n)
	}
}

func TestSetIcons(t *testing.T) {
	m := load(t, repo())
	if v := ansi.Strip(m.View()); strings.Contains(v, "■") {
		t.Fatalf("a tree without icons drew one:\n%s", v)
	}
	m.SetIcons(boxIcons)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "■ cmd") || !strings.Contains(v, "· go.mod") {
		t.Fatalf("SetIcons should draw icons before the nodes known:\n%s", v)
	}
	m.SetIcons(nil)
	if v := ansi.Strip(m.View()); strings.Contains(v, "■") || !strings.Contains(v, "▸ cmd") {
		t.Fatalf("SetIcons(nil) should drop the icons:\n%s", v)
	}
}
