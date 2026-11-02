package ui

import "testing"

func TestCount(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1k", 1234: "1.2k", 12_345: "12k", 2_500_000: "2m"} {
		if got := Count(n, false); got != want {
			t.Errorf("Count(%d, false) = %q, want %q", n, got, want)
		}
		if got := Count(n, true); got != want+"+" {
			t.Errorf("Count(%d, true) = %q, want %q", n, got, want+"+")
		}
	}
}
