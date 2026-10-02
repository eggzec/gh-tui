package graph

import (
	"cmp"
	"slices"
	"strings"
	"testing"
)

// c returns a commit with the given ID and parents.
func c(id string, parents ...string) Commit {
	return Commit{ID: id, Parents: parents, Title: id}
}

// defaultGlyphs are the glyphs of the default styles.
var defaultGlyphs = func() [glyphCount]string {
	st := DefaultStyles(true)
	return glyphs(st.CommitGlyph, st.Ellipsis, st.Lines)
}()

// plain draws cells without styles.
func plain(cells []cell) string {
	var b strings.Builder
	for j, c := range cells {
		b.WriteString(defaultGlyphs[c.glyph])
		if j == len(cells)-1 {
			break
		}
		if c.gap == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteString("─")
		}
	}
	return b.String()
}

// lay lays out commits in order and returns the graph of each row.
func lay(maxLanes int, commits ...Commit) []string {
	l := newLayout(maxLanes)
	seen := map[string]bool{}
	rows := make([]string, 0, len(commits))
	for i := range commits {
		seen[commits[i].ID] = true
		rows = append(rows, plain(l.add(commits[i], func(id string) bool { return seen[id] }, nil)))
	}
	return rows
}

func TestLayout(t *testing.T) {
	tests := []struct {
		name     string
		maxLanes int
		commits  []Commit
		want     []string
	}{
		{
			name:    "linear",
			commits: []Commit{c("c", "b"), c("b", "a"), c("a")},
			want:    []string{"●", "●", "●"},
		},
		{
			name: "merge",
			commits: []Commit{
				c("m", "a", "f"), c("f", "e"), c("a", "b"), c("e", "b"), c("b"),
			},
			want: []string{"●─╮", "│ ●", "● │", "│ ●", "●─╯"},
		},
		{
			name: "merge into a lane that waits for the parent",
			commits: []Commit{
				c("m2", "m1", "x"), c("m1", "a", "x"), c("x", "a"), c("a"),
			},
			want: []string{"●─╮", "●─┤", "│ ●", "●─╯"},
		},
		{
			name: "fork: two children of one parent",
			commits: []Commit{
				c("feature", "base"), c("main", "base"), c("base"),
			},
			want: []string{"●", "│ ●", "●─╯"},
		},
		{
			name:    "octopus",
			commits: []Commit{c("o", "a", "b", "c", "d"), c("d"), c("c"), c("b"), c("a")},
			want:    []string{"●─┬─┬─╮", "│ │ │ ●", "│ │ ●", "│ ●", "●"},
		},
		{
			name:     "octopus over the cap",
			maxLanes: 2,
			commits:  []Commit{c("o", "a", "b", "c", "d"), c("d"), c("c"), c("b"), c("a")},
			want:     []string{"●─┬─…", "│ │ ●", "│ │ ●", "│ ●", "●"},
		},
		{
			name: "a merge crosses a lane",
			commits: []Commit{
				c("x", "y"), c("z", "w"), c("y", "q", "r"), c("r"), c("w"), c("q"),
			},
			want: []string{"●", "│ ●", "●─┼─╮", "│ │ ●", "│ ●", "●"},
		},
		{
			name: "parents not loaded keep their lanes open",
			commits: []Commit{
				c("a", "missing"), c("b", "gone"), c("c", "b2"),
			},
			want: []string{"●", "│ ●", "│ │ ●"},
		},
		{
			name: "a parent shown above its child has no lane",
			commits: []Commit{
				c("p"), c("child", "p"), c("next"),
			},
			want: []string{"●", "●", "●"},
		},
		{
			name: "a freed lane is taken again",
			commits: []Commit{
				c("m", "a", "f"), c("f", "a"), c("a", "b"), c("n", "o"), c("b"),
			},
			want: []string{"●─╮", "│ ●", "●─╯", "│ ●", "● │"},
		},
		{
			name: "repeated parents",
			commits: []Commit{
				c("m", "a", "a", ""), c("a"),
			},
			want: []string{"●", "●"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lay(cmp.Or(tt.maxLanes, DefaultMaxLanes), tt.commits...)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestLayoutOverflowHidesACommit(t *testing.T) {
	// Three unrelated heads with open lanes push the fourth past a cap of 2.
	got := lay(2, c("a", "x"), c("b", "y"), c("c", "z"), c("d", "w"))
	want := []string{"●", "│ ●", "│ │ ●", "│ │ ●"}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
}

func TestLayoutColors(t *testing.T) {
	l := newLayout(8)
	seen := func(string) bool { return false }
	l.add(c("x", "y"), seen, nil)
	cells := l.add(c("m", "q", "r"), seen, nil)
	// │ in lane 0, ● in lane 1, and a line to the new lane 2 in its color.
	if len(cells) != 3 || cells[0].color != 0 || cells[1].color != 1 || cells[2].color != 2 {
		t.Fatalf("cells = %+v", cells)
	}
	if cells[1].gap != 3 || cells[0].gap != 0 {
		t.Fatalf("gaps = %d %d, want 0 3", cells[0].gap, cells[1].gap)
	}
}
