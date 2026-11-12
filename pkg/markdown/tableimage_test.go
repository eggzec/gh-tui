package markdown

import (
	"strings"
	"testing"
)

// A table of pictures lists no addresses under it, as GitHub shows none, and
// each cell keeps the alt text of its image. A link in a cell is still listed.
func TestTableImagesListNoAddresses(t *testing.T) {
	const a = "https://github.com/user-attachments/assets/aaaa"
	const b = "https://github.com/user-attachments/assets/bbbb"
	src := "| Before | After |\n| --- | --- |\n" +
		"| <img width=\"10\" alt=\"ayu-old\" src=\"" + a + "\" /> | <img alt=\"ayu-new\" src=\"" + b + "\"> |\n" +
		"| ![md-img](" + a + ") | [docs](https://example.com/docs) |\n"
	out := New(DefaultStyle(true)).Render(src, 80)
	for _, want := range []string{"ayu-old", "ayu-new", "md-img", "docs"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from %q", want, out)
		}
	}
	for _, bad := range []string{"user-attachments", "ayu-old[", "ayu-new[", "md-img["} {
		if strings.Contains(out, bad) {
			t.Errorf("%q should not be in %q", bad, out)
		}
	}
	if !strings.Contains(out, "example.com") {
		t.Errorf("the link of a cell is no longer listed: %q", out)
	}
}
