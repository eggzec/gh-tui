package markdown

import (
	"strconv"
	"strings"
	"testing"
)

// hyperlinks returns how many hyperlinks the line of raw opens and how many
// it closes, reading the OSC 8 sequences themselves.
func hyperlinks(line string) (opened, closed int) {
	for {
		i := strings.Index(line, "\x1b]8;")
		if i < 0 {
			return opened, closed
		}
		n, uri, ok := osc8(line[i:])
		if !ok {
			return opened, closed
		}
		if uri == "" {
			closed++
		} else {
			opened++
		}
		line = line[i+n:]
	}
}

// An image's address, printed after its text, is not a link of its own: the
// text is the one link, and nothing of the marks is left, wherever the
// lines wrap.
func TestRenderImageWithAddressIsOneLink(t *testing.T) {
	// Serial: links are a process-wide setting.
	const img = "![P2](https://greptile-static-assets.s3.amazonaws.com/badges/p2.svg?v=9)"
	srcs := map[string]string{
		"paragraph": "Reviews (1) · " + img + " end",
		"list":      "- item " + img + " x\n  - nested " + img,
		"quote":     "> > " + img,
	}
	for name, src := range srcs {
		for _, width := range []int{20, 30, 40, 60, 80} {
			t.Run(name+"/"+strconv.Itoa(width), func(t *testing.T) {
				linksOn(t, true)
				raw := New(DefaultStyle(true)).Render(src, width)
				if strings.ContainsFunc(raw, isLinkRune) {
					t.Errorf("a character of the links is left: %q", raw)
				}
				opened, closed := 0, 0
				for l := range strings.SplitSeq(raw, "\n") {
					o, c := hyperlinks(l)
					opened += o
					closed += c
				}
				want := strings.Count(src, "![P2]")
				if opened != want || closed != want {
					t.Errorf("%d hyperlinks opened and %d closed, want %d each: %q", opened, closed, want, raw)
				}
			})
		}
	}
}
