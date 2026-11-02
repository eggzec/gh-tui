package ui

import "testing"

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
