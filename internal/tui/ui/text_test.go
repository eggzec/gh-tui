package ui

import "testing"

func TestCount(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1k", 1234: "1.2k", 12_345: "12k", 2_500_000: "2m"} {
		if got := Count(n); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	t.Parallel()
	if Plural(1, "a", "b") != "a" || Plural(2, "a", "b") != "b" || Plural(0, "a", "b") != "b" {
		t.Error("Plural picks the wrong form")
	}
}

func TestCleanLine(t *testing.T) {
	t.Parallel()
	if got := CleanLine("a\x1b[31m  b\n\tc"); got != "a b c" {
		t.Errorf("CleanLine = %q", got)
	}
}

func TestSpreadDrop(t *testing.T) {
	t.Parallel()
	if got := SpreadDrop("ab", "cd", 8, "…"); got != "ab    cd" {
		t.Errorf("Spread = %q", got)
	}
	if got := SpreadDrop("abcdef", "cd", 5, "…"); got != "abcd…" {
		t.Errorf("Spread without room = %q", got)
	}
}
