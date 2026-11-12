package markdown

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

var sgrSeq = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// padStyled returns what is drawn styled on the padding of line: the
// styles that are on at its trailing spaces.
func padStyled(line string) []string {
	plain := strings.TrimRight(xansi.Strip(line), " ")
	if plain == xansi.Strip(line) {
		return nil
	}
	// Find where the last visible character ends in the raw line.
	end, seen := 0, 0
	for i := 0; i < len(line); {
		if loc := sgrSeq.FindStringIndex(line[i:]); len(loc) > 0 && loc[0] == 0 {
			i += loc[1]
			continue
		}
		if line[i] == '\x1b' && strings.HasPrefix(line[i:], "\x1b]8;") {
			j := strings.IndexAny(line[i:], "\a\x1b")
			if j < 0 {
				break
			}
			if line[i+j] == '\x1b' {
				j++
			}
			i += j + 1
			continue
		}
		if seen == len(plain) {
			break
		}
		seen++
		i++
		end = i
	}
	var on []string
	for _, s := range sgrSeq.FindAllString(line[end:], -1) {
		if s != "\x1b[m" && s != "\x1b[0m" {
			on = append(on, strings.ReplaceAll(s, "\x1b", "ESC"))
		}
	}
	return on
}

func checkNoStyledPad(t *testing.T, raw string) {
	t.Helper()
	drawn, _ := underlinedCells(raw)
	for i, l := range strings.Split(raw, "\n") {
		if on := padStyled(l); len(on) > 0 {
			t.Errorf("line %d leaves %v on its padding: %q", i, on, l)
		}
		if strings.Contains(drawn[i], " ") {
			t.Errorf("line %d draws a space underlined: %q", i, l)
		}
	}
}

// The address glamour prints after a link wraps, and is gone with its
// style, so the padding of the line is not drawn in the style of a link.
func TestRenderListItemEndingInLinkLeavesNoStyledPadding(t *testing.T) {
	const src = "* Defer checks by @b in [#2](https://x.io/2)"
	for _, width := range []int{30, 36, 40, 44, 50, 60, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			linksOn(t, true)
			checkNoStyledPad(t, New(DefaultStyle(true)).Render(src, width))
		})
	}
}

// An image alone on its line shows its text as a link, and the style of the
// address glamour printed after it is not left on the padding.
func TestRenderImageLineLeavesNoStyledPadding(t *testing.T) {
	const src = "![Screenshot](https://user-images.githubusercontent.com/4655775/12345-abcdef.png)"
	for _, width := range []int{40, 60, 80, 100, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			linksOn(t, true)
			checkNoStyledPad(t, New(DefaultStyle(true)).Render(src, width))
		})
	}
}

// The notes of a release: bullets with mentions, references and code that
// end in a link.
func TestRenderReleaseNotesLeavesNoStyledPadding(t *testing.T) {
	const src = "## What's Changed\n\n" +
		"* Defer `--attach` credential checks to the upload endpoint by @BagToad in [#14805](https://github.com/cli/cli/pull/14805)\n" +
		"* Fix the `gh pr checks` exit status for pending runs by @someone in [#14790](https://github.com/cli/cli/pull/14790)\n\n" +
		"**Full Changelog**: https://github.com/cli/cli/compare/v2.103.1...v2.103.2\n"
	for _, width := range []int{80, 100, 108, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			linksOn(t, true)
			checkNoStyledPad(t, New(DefaultStyle(true)).Render(src, width))
		})
	}
}
