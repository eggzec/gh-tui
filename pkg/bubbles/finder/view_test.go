package finder

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// sized returns the sample with the size of each file as its detail.
func sized() Load {
	sizes := []string{"12K", "31K", "2.1K", "860B", "0.6K", "3.4K", "1.8K", "24K"}
	return func(context.Context) (Listing, error) {
		its := items(sample...)
		for i := range its {
			its[i].Detail = sizes[i]
		}
		return Listing{Items: its}, nil
	}
}

func TestView(t *testing.T) {
	deep := "staging/src/k8s.io/apimachinery/pkg/util/strategicpatch/testdata/swagger-merge-item.json"
	tests := []struct {
		name          string
		width, height int
		load          Load
		opts          []Option
		// skipInit leaves the paths loading.
		skipInit bool
		typed    string
		keys     []string
	}{
		{name: "all", width: 60, height: 12, load: sized()},
		{name: "matches", width: 60, height: 12, load: sized(), typed: "rend"},
		{name: "second selected", width: 60, height: 12, load: sized(), typed: "rend", keys: []string{"down"}},
		{name: "terms", width: 60, height: 8, load: sized(), typed: "rend test"},
		{name: "no match", width: 60, height: 6, load: sized(), typed: "zzz"},
		{name: "loading", width: 60, height: 6, load: sized(), skipInit: true},
		{name: "error", width: 60, height: 6, load: func(context.Context) (Listing, error) {
			return Listing{}, errors.New("403 rate limited")
		}},
		{name: "empty", width: 60, height: 6, load: loader()},
		{name: "note", width: 60, height: 6, load: func(context.Context) (Listing, error) {
			return Listing{Items: items(sample...), Note: "listing cut short"}, nil
		}},
		{name: "cut from the left", width: 40, height: 6, load: loader(deep, "a/"+strings.Repeat("x", 60)+".go"), typed: "swag"},
		{name: "narrow drops detail", width: 26, height: 8, load: sized(), typed: "rend"},
		{name: "scrolled", width: 40, height: 5, load: sized(), keys: []string{"down", "down", "down", "down"}},
		{name: "light", width: 60, height: 8, load: sized(), typed: "rend", opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "one row", width: 40, height: 1, load: sized(), typed: "rend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(tt.load, append([]Option{WithSize(tt.width, tt.height)}, tt.opts...)...)
			m.Focus()
			if !tt.skipInit {
				m = run(t, m, m.Init())
			}
			m = typed(t, m, tt.typed)
			m = keys(t, m, tt.keys...)
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// TestViewMarksMatches checks the characters that the rows mark, without
// the colors of the golden files.
func TestViewMarksMatches(t *testing.T) {
	m := open(t, 60, 6, sample, WithStyles(Styles{Match: DefaultStyles(true).Match.Reverse(true)}))
	m = typed(t, m, "crt")
	row := strings.Split(m.View(), "\n")[1]
	on := newPair(m.styles.Match).on
	var marked strings.Builder
	for _, part := range strings.Split(row, on)[1:] {
		marked.WriteString(ansi.Strip(part)[:1])
	}
	if got := marked.String(); got != "crt" {
		t.Errorf("marked %q in %q, want crt", got, ansi.Strip(row))
	}
}

func TestViewZeroSize(t *testing.T) {
	m := open(t, 0, 0, sample)
	if m.View() != "" {
		t.Error("a finder of no size renders something")
	}
}
