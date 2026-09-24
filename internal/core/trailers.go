package core

import (
	"strings"
)

// Trailer is one "Key: value" line of the trailer block that ends a commit
// message, such as Signed-off-by or Co-authored-by. Keys keep their case.
type Trailer struct {
	Key   string
	Value string
}

// SplitMessage splits a commit message into its subject, the first
// paragraph joined into one line as git log --format=%s does; its body,
// the rest without the trailers; and the trailers, in order.
//
// The trailers are the last paragraph of the body, if every line of it is
// "Key: value", where the key has letters, digits and hyphens, or a
// continuation of the previous value that starts with a space or a tab.
// A continuation is folded into its value with one space. A paragraph that
// has any other line is part of the body, and so is the subject, however
// it looks.
func SplitMessage(message string) (subject, body string, trailers []Trailer) {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	paragraphs := splitParagraphs(message)
	if len(paragraphs) == 0 {
		return "", "", nil
	}
	subject = strings.Join(strings.Fields(paragraphs[0]), " ")
	rest := paragraphs[1:]
	if len(rest) > 0 {
		if t, ok := parseTrailers(rest[len(rest)-1]); ok {
			trailers, rest = t, rest[:len(rest)-1]
		}
	}
	return subject, strings.Join(rest, "\n\n"), trailers
}

// splitParagraphs returns the paragraphs of s, which blank lines separate,
// without the blank lines around them.
func splitParagraphs(s string) []string {
	var (
		out  []string
		para []string
	)
	flush := func() {
		if len(para) > 0 {
			out = append(out, strings.Join(para, "\n"))
			para = para[:0]
		}
	}
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			flush()
			continue
		}
		para = append(para, line)
	}
	flush()
	return out
}

// parseTrailers returns the trailers of para, if it is a trailer block.
func parseTrailers(para string) ([]Trailer, bool) {
	var out []Trailer
	for line := range strings.SplitSeq(para, "\n") {
		if line[0] == ' ' || line[0] == '\t' {
			if len(out) == 0 {
				return nil, false
			}
			last := &out[len(out)-1]
			last.Value = strings.TrimSpace(last.Value + " " + strings.TrimSpace(line))
			continue
		}
		t, ok := parseTrailer(line)
		if !ok {
			return nil, false
		}
		out = append(out, t)
	}
	return out, len(out) > 0
}

// parseTrailer parses "Key: value". The colon must end the line or be
// followed by a space, so that a URL isn't read as a trailer.
func parseTrailer(line string) (Trailer, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok || key == "" || (value != "" && value[0] != ' ' && value[0] != '\t') {
		return Trailer{}, false
	}
	for _, r := range key {
		if !isTrailerKeyRune(r) {
			return Trailer{}, false
		}
	}
	return Trailer{Key: key, Value: strings.TrimSpace(value)}, true
}

func isTrailerKeyRune(r rune) bool {
	return r == '-' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')
}
