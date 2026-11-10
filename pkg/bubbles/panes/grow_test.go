package panes

import "testing"

func TestGrow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ focus, n, div, sign int }{
		{0, 2, 0, 1},
		{1, 2, 0, -1},
		{0, 3, 0, 1},
		{1, 3, 1, 1},
		{2, 3, 1, -1},
	} {
		div, sign := Grow(tc.focus, tc.n)
		if div != tc.div || sign != tc.sign {
			t.Errorf("Grow(%d, %d) = %d, %d; want %d, %d", tc.focus, tc.n, div, sign, tc.div, tc.sign)
		}
	}
}
