package pulls

import "testing"

// The choice to hide the line numbers outlives the diff being built again, by
// the switch between the file and the whole patch and by a step to the next
// file.
func TestDiffKeepsTheLineNumbersChoice(t *testing.T) {
	t.Parallel()
	h, _, o := diffPanes(t, "docs/c.md")
	if !o.d.LineNumbers() {
		t.Fatal("the line numbers start hidden")
	}
	press(t, h, "-")
	press(t, h, "N")
	if o.d.LineNumbers() || o.numbers {
		t.Fatalf("after -N: view %v, overlay %v, want both hidden", o.d.LineNumbers(), o.numbers)
	}
	for _, k := range []string{"a", "J", "a", "K"} {
		press(t, h, k)
		if o.d.LineNumbers() {
			t.Fatalf("the line numbers came back after %s", k)
		}
	}
	press(t, h, "-")
	press(t, h, "N")
	press(t, h, "a")
	if !o.d.LineNumbers() {
		t.Error("the line numbers did not stay shown after -N again and a")
	}
}
