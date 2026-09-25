package finder

import (
	"context"
	"math/rand/v2"
	"slices"
	"testing"
)

// TestRanking checks the order of matches: file names beat directories,
// word starts and runs beat scattered letters, shorter paths win ties, and
// recent paths float up.
func TestRanking(t *testing.T) {
	tests := []struct {
		name   string
		paths  []string
		query  string
		recent []string
		want   []string
	}{
		{
			"name beats directory",
			[]string{"render/model.go", "cursed_renderer.go"},
			"rend",
			nil,
			[]string{"cursed_renderer.go", "render/model.go"},
		},
		{
			"name start beats name middle",
			[]string{"cursed_renderer.go", "renderer.go"},
			"rend",
			nil,
			[]string{"renderer.go", "cursed_renderer.go"},
		},
		{
			"word starts beat scattered letters",
			[]string{"pkg/gamepad.go", "pkg/go_model.go"},
			"gm",
			nil,
			[]string{"pkg/go_model.go", "pkg/gamepad.go"},
		},
		{
			"directory starts beat a run in a word",
			[]string{"internal/tui/model.go", "static/utility.js"},
			"tui",
			nil,
			[]string{"internal/tui/model.go", "static/utility.js"},
		},
		{
			"camel case",
			[]string{"src/fooBar.ts", "src/foobar.ts"},
			"fb",
			nil,
			[]string{"src/fooBar.ts", "src/foobar.ts"},
		},
		{
			"consecutive beats spread",
			[]string{"a/view_extra.go", "a/viewx.go"},
			"viewx",
			nil,
			[]string{"a/viewx.go", "a/view_extra.go"},
		},
		{
			"shorter wins a tie",
			[]string{"deep/nested/dir/main.go", "cmd/main.go", "main.go"},
			"main",
			nil,
			[]string{"main.go", "cmd/main.go", "deep/nested/dir/main.go"},
		},
		{
			"order of the items breaks the last tie",
			[]string{"b/x.go", "a/x.go"},
			"x",
			nil,
			[]string{"b/x.go", "a/x.go"},
		},
		{
			"case folds",
			[]string{"README.md", "docs/readme.txt"},
			"ReadMe",
			nil,
			[]string{"README.md", "docs/readme.txt"},
		},
		{
			"terms match anywhere, all of them",
			[]string{"tea/renderer.go", "tea/renderer_test.go", "other/test.go"},
			"test rend",
			nil,
			[]string{"tea/renderer_test.go"},
		},
		{
			"recent floats up",
			[]string{"renderer.go", "cursed_renderer.go", "nil_renderer.go"},
			"rend",
			[]string{"nil_renderer.go"},
			[]string{"nil_renderer.go", "renderer.go", "cursed_renderer.go"},
		},
		{
			"more recent first",
			[]string{"a.go", "b.go", "c.go"},
			"go",
			[]string{"c.go", "b.go"},
			[]string{"c.go", "b.go", "a.go"},
		},
		{
			"recent first while empty",
			[]string{"a.go", "b.go", "c.go"},
			"",
			[]string{"c.go", "nope.go"},
			[]string{"c.go", "a.go", "b.go"},
		},
		{
			"no match",
			[]string{"a.go"},
			"zz",
			nil,
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := corpusOf(t, tt.paths...)
			got := ranked(t, c, tt.query, c.ranks(tt.recent))
			if !slices.Equal(got, tt.want) {
				t.Errorf("ranked %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAlign checks the matches that the rows mark.
func TestAlign(t *testing.T) {
	tests := []struct {
		path, term string
		want       []int
	}{
		{"cursed_renderer.go", "rend", []int{7, 8, 9, 10}},
		{"cursed_renderer_test.go", "crt", []int{0, 7, 16}},
		{"examples/render/render.go", "render", []int{16, 17, 18, 19, 20, 21}},
		{"internal/tui/model.go", "tuim", []int{9, 10, 11, 13}},
		{"a/b", "a/b", []int{0, 1, 2}},
	}
	for _, tt := range tests {
		c := corpusOf(t, tt.path)
		path, bonus := c.path(0)
		_, got, ok := align([]byte(tt.term), path, bonus)
		if !ok || !slices.Equal(got, tt.want) {
			t.Errorf("align(%q, %q) = %v, %v, want %v", tt.term, tt.path, got, ok, tt.want)
		}
	}
}

// TestScoreMatchesAlign checks that the banded scorer finds the score of
// the full tables, over many random terms and paths.
func TestScoreMatchesAlign(t *testing.T) {
	paths := bigPaths(2000)
	c := corpusOf(t, paths...)
	r := rand.New(rand.NewPCG(3, 4))
	var s scorer
	const alphabet = "abcdegilmnoprstuv_/.e"
	for range 3000 {
		i := int32(r.IntN(len(paths)))
		path, bonus := c.path(i)
		var term []byte
		if r.IntN(2) == 0 {
			// A subsequence of the path, so that it matches.
			for j := range path {
				if r.IntN(len(path)/3+1) == 0 {
					term = append(term, path[j])
				}
			}
		} else {
			for range 1 + r.IntN(6) {
				term = append(term, alphabet[r.IntN(len(alphabet))])
			}
		}
		if len(term) == 0 {
			continue
		}
		got, ok := s.score(term, path, bonus)
		want, pos, wantOK := align(term, path, bonus)
		if ok != wantOK || ok && got != want {
			t.Fatalf("score(%q, %q) = %d, %v; align found %d, %v", term, path, got, ok, want, wantOK)
		}
		if ok && !isAlignment(term, path, pos) {
			t.Fatalf("align(%q, %q) = %v, not an alignment", term, path, pos)
		}
	}
}

func isAlignment(term, path []byte, pos []int) bool {
	if len(pos) != len(term) {
		return false
	}
	for i, p := range pos {
		if path[p] != term[i] || i > 0 && p <= pos[i-1] {
			return false
		}
	}
	return true
}

func TestNarrows(t *testing.T) {
	tests := []struct {
		prev, next string
		want       bool
	}{
		{"", "a", true},
		{"re", "rend", true},
		{"rnd", "rend", true},
		{"rend", "ren", false},
		{"rend", "rend test", true},
		{"rend test", "rend", false},
		{"ab", "a b", false},
		{"a b", "ab", true},
	}
	for _, tt := range tests {
		if got := narrows(terms(tt.prev), terms(tt.next)); got != tt.want {
			t.Errorf("narrows(%q, %q) = %v, want %v", tt.prev, tt.next, got, tt.want)
		}
	}
}

// TestNarrowingMatchesFullScan checks that narrowing from the last result
// finds what a scan of every item finds, for a query typed a key at a time.
func TestNarrowingMatchesFullScan(t *testing.T) {
	c := corpusOf(t, bigPaths(5000)...)
	for _, q := range []string{"pkg/ctl", "zz_gen deep", "rest_test", "vol attach"} {
		var prev *result
		for i := range len(q) + 1 {
			query := q[:i]
			from := prev
			if prev != nil && !narrows(prev.terms, terms(query)) {
				from = nil
			}
			got, _ := filter(t.Context(), c, query, from, nil)
			want, _ := filter(t.Context(), c, query, nil, nil)
			if !slices.Equal(got.items, want.items) {
				t.Fatalf("%q: narrowing found %d items, a full scan %d", query, len(got.items), len(want.items))
			}
			prev = got
		}
	}
}

func TestFilterStopsWhenCancelled(t *testing.T) {
	c := corpusOf(t, bigPaths(parallelMin*2)...)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if res, ok := filter(ctx, c, "ab", nil, nil); ok || res != nil {
		t.Errorf("filter = %v, %v after its context was cancelled", res, ok)
	}
}

func TestSortKeys(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for _, n := range []int{0, 1, 255, 256, 5000} {
		keys := make([]uint64, n)
		for i := range keys {
			keys[i] = rankKey(r.IntN(400)-100, r.IntN(300), int32(r.IntN(idxMask)))
		}
		want := slices.Sorted(slices.Values(keys))
		sortKeys(keys)
		if !slices.Equal(keys, want) {
			t.Errorf("sortKeys of %d keys isn't sorted", n)
		}
	}
}

func TestRankingRunBeatsScatteredStarts(t *testing.T) {
	c := corpusOf(t,
		"src/crypto/x509/testdata/nist-pkits/certs/RolloverfromPrintableStringtoUTF8StringCACert.crt",
		"src/net/http/responsecontroller.go",
	)
	if got := ranked(t, c, "controller", nil); got[0] != "src/net/http/responsecontroller.go" {
		t.Errorf("ranked %v, want the run in the file name first", got)
	}
}
