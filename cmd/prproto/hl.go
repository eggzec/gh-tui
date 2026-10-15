package main

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

const tabStop = 4

// expandTabs turns tabs into spaces up to the next stop, per line, so the
// width code and the terminal agree on how wide a row is.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			n := tabStop - col%tabStop
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case '\n':
			b.WriteRune(r)
			col = 0
		default:
			b.WriteRune(r)
			col += ansi.StringWidth(string(r))
		}
	}
	return b.String()
}

// highlight colours a code block by its language tag; untagged or unknown
// languages come back unchanged. The result has one string per input line.
func highlight(lang string, lines []string) []string {
	lang = strings.ToLower(strings.TrimSpace(strings.Fields(lang + " x")[0]))
	if lang == "" || lang == "x" {
		return lines
	}
	lx := lexers.Get(lang)
	if lx == nil {
		return lines
	}
	lx = chroma.Coalesce(lx)
	it, err := lx.Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return lines
	}
	var b strings.Builder
	if err := formatters.TTY16m.Format(&b, styles.Get("monokai"), it); err != nil {
		return lines
	}
	out := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(out) != len(lines) {
		return lines
	}
	return out
}
