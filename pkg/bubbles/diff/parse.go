package diff

import (
	"errors"
	"fmt"
	"strings"
)

// patchLines is the number of lines of a patch, which is also the number of
// rows Parse returns for it. A trailing newline does not start another line.
func patchLines(patch string) int {
	if patch == "" {
		return 0
	}
	n := strings.Count(patch, "\n")
	if patch[len(patch)-1] != '\n' {
		n++
	}
	return n
}

// Parse reads a unified patch of hunks only. It returns one row per line of
// the patch, numbered on both sides, and the hunks they belong to; the rows'
// File is left 0 for the caller to set. A "\ No newline" row carries the
// number of the line it follows, on each side that line has one.
//
// A line ends at "\n", and a "\r" before it is dropped, so CRLF patches read
// as LF ones. Text keeps every other character, control characters too: a
// renderer must sanitize it.
//
// The hunk headers' counts are checked. If the patch is not well formed (text
// before the first hunk header, a header that does not read, a line that
// starts with something other than a space, +, -, \ or @@, more lines in a
// hunk than its header counts, or fewer) the rows are the lines as they are,
// all KindRaw with Err set, and the error says what was found first. One
// exception: when the patch ends in a newline, the last hunk may be one
// blank context line short, since GitHub trims the lone space of such a line
// and what is left looks like the end of the patch.
func Parse(patch string) ([]Hunk, []Row, error) {
	rows := make([]Row, 0, patchLines(patch))
	var hunks []Hunk
	var oldN, newN int       // numbers of the next line on each side
	var remOld, remNew int   // lines the current hunk still owes
	var lastOld, lastNew int // numbers of the last line, for a no-newline marker
	hunk := -1
	lineNo := 0
	bad := func(format string, args ...any) ([]Hunk, []Row, error) {
		msg := fmt.Sprintf("line %d: ", lineNo) + fmt.Sprintf(format, args...)
		return nil, rawRows(patch, msg), errors.New(msg)
	}
	for rest := patch; rest != ""; {
		var line string
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line, rest = rest[:i], rest[i+1:]
		} else {
			line, rest = rest, ""
		}
		line = strings.TrimSuffix(line, "\r")
		lineNo++
		if strings.HasPrefix(line, "@@") {
			if remOld != 0 || remNew != 0 {
				return bad("hunk %d is short of its counts", hunk+1)
			}
			h, ok := parseHunkHeader(line)
			if !ok {
				return bad("bad hunk header %q", line)
			}
			hunks = append(hunks, h)
			hunk = len(hunks) - 1
			oldN, newN = h.OldStart, h.NewStart
			remOld, remNew = h.OldLines, h.NewLines
			// A side with no lines names the line before the hunk.
			if h.OldLines == 0 {
				oldN++
			}
			if h.NewLines == 0 {
				newN++
			}
			lastOld, lastNew = 0, 0
			rows = append(rows, Row{Kind: KindHunkHeader, Hunk: hunk, Text: line})
			continue
		}
		if hunk < 0 {
			return bad("text before the first hunk")
		}
		switch {
		case line == "" || line[0] == ' ': // a blank one has lost its lone space
			if remOld == 0 || remNew == 0 {
				return bad("line past the counts of hunk %d", hunk+1)
			}
			text := ""
			if line != "" {
				text = line[1:]
			}
			rows = append(rows, Row{Kind: KindContext, Hunk: hunk, Old: oldN, New: newN, Text: text})
			lastOld, lastNew = oldN, newN
			oldN++
			newN++
			remOld--
			remNew--
		case line[0] == '+':
			if remNew == 0 {
				return bad("line past the counts of hunk %d", hunk+1)
			}
			rows = append(rows, Row{Kind: KindAdded, Hunk: hunk, New: newN, Text: line[1:]})
			lastOld, lastNew = 0, newN
			newN++
			remNew--
		case line[0] == '-':
			if remOld == 0 {
				return bad("line past the counts of hunk %d", hunk+1)
			}
			rows = append(rows, Row{Kind: KindDeleted, Hunk: hunk, Old: oldN, Text: line[1:]})
			lastOld, lastNew = oldN, 0
			oldN++
			remOld--
		case line[0] == '\\':
			rows = append(rows, Row{Kind: KindNoNewline, Hunk: hunk, Old: lastOld, New: lastNew, Text: line})
		default:
			return bad("unexpected line %q", line)
		}
	}
	if remOld != 0 || remNew != 0 {
		if remOld != 1 || remNew != 1 || patch[len(patch)-1] != '\n' {
			return bad("hunk %d is short of its counts", hunk+1)
		}
	}
	return hunks, rows, nil
}

// rawRows is every line of the patch as a KindRaw row carrying err.
func rawRows(patch, err string) []Row {
	rows := make([]Row, 0, patchLines(patch))
	for rest := patch; rest != ""; {
		var line string
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line, rest = rest[:i], rest[i+1:]
		} else {
			line, rest = rest, ""
		}
		rows = append(rows, Row{Kind: KindRaw, Hunk: -1, Text: strings.TrimSuffix(line, "\r"), Err: err})
	}
	return rows
}

// parseHunkHeader reads "@@ -a[,b] +c[,d] @@[ section]".
func parseHunkHeader(line string) (Hunk, bool) {
	var h Hunk
	s, ok := strings.CutPrefix(line, "@@ -")
	if !ok {
		return h, false
	}
	if h.OldStart, h.OldLines, s, ok = rangeOf(s); !ok {
		return h, false
	}
	if s, ok = strings.CutPrefix(s, " +"); !ok {
		return h, false
	}
	if h.NewStart, h.NewLines, s, ok = rangeOf(s); !ok {
		return h, false
	}
	if s, ok = strings.CutPrefix(s, " @@"); !ok {
		return h, false
	}
	switch {
	case s == "":
	case s[0] == ' ':
		h.Section = s[1:]
	default:
		return h, false
	}
	return h, true
}

// rangeOf reads "start[,count]" off the front of s; a missing count is 1.
func rangeOf(s string) (start, count int, rest string, ok bool) {
	start, s, ok = number(s)
	if !ok {
		return 0, 0, s, false
	}
	count = 1
	if after, found := strings.CutPrefix(s, ","); found {
		count, s, ok = number(after)
		if !ok {
			return 0, 0, s, false
		}
	}
	return start, count, s, true
}

// number reads the digits off the front of s.
func number(s string) (n int, rest string, ok bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' && i < 9 {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return n, s[i:], i > 0
}
