package actions

import (
	"slices"
	"testing"
)

func TestCountForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		n    int
		more bool
		want []string
	}{
		{"none", 0, false, nil},
		{"one", 1, false, []string{"1 run", "1"}},
		{"one of more", 1, true, []string{"1+ runs", "1+"}},
		{"few", 30, true, []string{"30+ runs", "30+"}},
		{"thousands loaded with more", 1200, true, []string{"1.2k+ runs", "1.2k+"}},
		{"thousands loaded", 1200, false, []string{"1.2k runs", "1.2k"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := countForms(tt.n, tt.more, "run", "runs"); !slices.Equal(got, tt.want) {
				t.Errorf("countForms(%d, %v) = %q, want %q", tt.n, tt.more, got, tt.want)
			}
		})
	}
}
