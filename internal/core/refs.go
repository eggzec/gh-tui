package core

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RefGroup is where a link comes from, which is the group a list of them
// shows it in. A lower group is the stronger one.
type RefGroup int

// The groups of references.
const (
	// RefClosing is what the item closes (a pull request) or what closes
	// it (an issue).
	RefClosing RefGroup = iota
	// RefWritten is what the body, a comment or a review of the item links.
	RefWritten
	// RefMentioned is what mentions the item, from its timeline.
	RefMentioned
)

// RefOrigin says exactly where one link was found.
type RefOrigin struct {
	Group RefGroup
	// Where is "body", "comment", "review", "closes", "closed by" or
	// "mentioned".
	Where string
	// By is the login of the comment's, review's or mention's author, "ghost"
	// for a deleted account, and empty for the body and the closing links.
	By string
	// Bot reports that By is an app. Its login has "[bot]" after it.
	Bot bool
}

// Reference is an issue or pull request linked to the item on view.
type Reference struct {
	// Target names it. Its Kind is unknown when it could not be read, and
	// for a link written as owner/name#12.
	Target Target
	Title  string
	State  State
	Draft  bool
	// Reason says why a closed issue was closed.
	Reason StateReason
	URL    string
	// Problem says why the item can't be read, such as "not found, or
	// private", or is empty when it was read.
	Problem string
	// Origins are every place it was found, the strongest first.
	Origins []RefOrigin
}

// References is everything linked to one item that one read finds. The
// items that mention it are read apart, a page at a time.
type References struct {
	Closing []Reference
	Written []Reference
	// Mentioned is GitHub's count of the items that mention this one, before
	// any is read.
	Mentioned int
	// CommentsRead of CommentsTotal comments, and ReviewsRead of
	// ReviewsTotal reviews, were searched for links: GitHub serves the
	// newest 100 of each, since the latest discussion is the likelier to name
	// what is linked now.
	CommentsRead, CommentsTotal int
	ReviewsRead, ReviewsTotal   int
	// Unresolved counts the links written in the text that were left
	// unread because there are more than one read resolves.
	Unresolved int
	// Failed counts those whose read failed, and FailedWhy says why, such as
	// "rate limited". A read with any is not kept as fresh, so the next one
	// asks again.
	Failed    int
	FailedWhy string
	// ItemUpdated is the update time of the item that the read was made
	// for, as its query said, or zero.
	ItemUpdated time.Time
	// ReadAt is when GitHub was asked.
	ReadAt time.Time

	// Stale reports that the value was kept by an earlier session and is
	// served before GitHub was asked whether it changed. Reading it again
	// with the query's Again set asks GitHub.
	Stale bool
	// Offline reports that GitHub couldn't be reached, so this is what
	// was read last.
	Offline bool
	// Limited reports that GitHub rate limited the read, so this is what
	// was read last.
	Limited bool
}

// RefText is a text of the item that may name other items, and where it
// is from.
type RefText struct {
	Origin RefOrigin
	Text   string
}

// RefLink is a Connected or Disconnected event of the item's timeline,
// which the Development sidebar makes, with the end that isn't the item.
type RefLink struct {
	Ref          Reference
	Disconnected bool
}

// RefSources is what one read of an item says of its links, before they
// are merged: the texts to search, GitHub's own links, and the counts.
type RefSources struct {
	// Texts are the body, then the comments, then the reviews.
	Texts []RefText
	// Closing are what GitHub says the item closes, or is closed by.
	Closing []Reference
	// Linked are the events of the sidebar's links, the oldest first.
	Linked []RefLink
	// Closers are the pull requests that closed an issue, the oldest
	// first.
	Closers []Reference
	// Mentioned is the number of items that mention this one.
	Mentioned                   int
	CommentsRead, CommentsTotal int
	ReviewsRead, ReviewsTotal   int
}

// RefKey names the item t is: the same for every case of its repository
// and whatever its Kind, since a number names one thing.
func RefKey(t Target) string {
	return strings.ToLower(t.Repo.String()) + "#" + strconv.Itoa(t.Number)
}

// refAfter ends a number: a word boundary, or the underscore that closes
// emphasis.
const refAfter = `(?:\b|_)`

// refBefore is what may come before a link in text: the start, space,
// punctuation or the mark of emphasis, but not a letter, digit or an
// underscore that is part of a word.
const refBefore = `(?:^|[\s(\[{,;:*~]|(?:^|[\s(\[{,;:*~])_)`

var (
	refURL   = regexp.MustCompile(`https?://[^\s<>()\[\]"']+`)
	refShort = regexp.MustCompile(refBefore + `((?:[A-Za-z0-9_-]+/[A-Za-z0-9._-]+)?#[0-9]+)` + refAfter)
	// refGH is GitHub's autolink GH-123, in capitals, which names a number of the
	// same repository.
	refGH   = regexp.MustCompile(refBefore + `(GH-(\d+))` + refAfter)
	refMark = regexp.MustCompile("^ {0,3}(```+|~~~+)")
	refList = regexp.MustCompile(`^ {0,3}([-*+]|\d+[.)])\s`)
	// refBreak is the end of a paragraph: a blank line.
	refBreak = regexp.MustCompile(`\n[ \t]*\n`)
	// refItem is the start of a list item, which ends a code span.
	refItem = regexp.MustCompile(`\n {0,3}(?:[-*+]|\d+[.)])\s`)
	// refHosts holds the regexp of links without a scheme, by host.
	refHosts sync.Map
)

// refBare returns the regexp of a link to host written without its
// scheme, such as github.com/owner/name/issues/8.
func refBare(host string) *regexp.Regexp {
	if re, ok := refHosts.Load(host); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(refBefore + `((?:www\.)?` + regexp.QuoteMeta(host) + `/[^\s<>()\[\]"']+)`)
	refHosts.Store(host, re)
	return re
}

// ScanRefs returns the issues and pull requests that text links, in the
// order they appear, as ParseTarget on host reads them: a link to one,
// with or without its scheme, or #12, GH-12 or owner/name#12, whose
// repository is here when it names none. Links to another host are left
// out, and so is what GitHub doesn't link: code, which is fenced,
// indented or in backticks, and HTML comments. A target may come back
// more than once.
func ScanRefs(text, host string, here RepoRef) []Target {
	text = refStripInline(refStripBlocks(text))
	if host == "" {
		host = DefaultHost
	}
	type found struct {
		at int
		t  Target
	}
	var all []found
	add := func(at int, s string, short bool) {
		t, err := ParseTarget(s, host)
		if err != nil || !t.HasNumber() || !t.HasRepo() && !short {
			return
		}
		if !t.HasRepo() {
			t.Repo = here
		}
		all = append(all, found{at, t})
	}
	// What a match is found in is blanked, so no other pattern finds it
	// again.
	blank := []byte(text)
	blankOut := func(from, to int) {
		for i := from; i < to; i++ {
			blank[i] = ' '
		}
	}
	for _, m := range refURL.FindAllStringIndex(text, -1) {
		add(m[0], strings.TrimRight(text[m[0]:m[1]], ".,;:!?*_~"), false)
		blankOut(m[0], m[1])
	}
	rest := string(blank)
	for _, m := range refBare(host).FindAllStringSubmatchIndex(rest, -1) {
		add(m[2], strings.TrimRight(rest[m[2]:m[3]], ".,;:!?*_~"), false)
		blankOut(m[2], m[3])
	}
	rest = string(blank)
	for _, m := range refShort.FindAllStringSubmatchIndex(rest, -1) {
		add(m[2], rest[m[2]:m[3]], true)
	}
	for _, m := range refGH.FindAllStringSubmatchIndex(rest, -1) {
		add(m[2], "#"+rest[m[4]:m[5]], true)
	}
	slices.SortStableFunc(all, func(a, b found) int { return cmp.Compare(a.at, b.at) })
	out := make([]Target, len(all))
	for i := range all {
		out[i] = all[i].t
	}
	return out
}

// refStripBlocks blanks the fenced and the indented code blocks of text.
// An indented block starts after a blank line or at the start, and the
// paragraph of a list item isn't one.
func refStripBlocks(text string) string {
	lines := strings.Split(text, "\n")
	// fence is the opening run of an open fenced block.
	var fence string
	// prev is the kind of the last line that wasn't blank, and gap says that
	// a blank line came since.
	const (
		start = iota
		code
		list
		prose
	)
	prev, gap := start, true
	for i, line := range lines {
		if fence != "" {
			trimmed := strings.TrimSpace(line)
			if strings.Trim(trimmed, fence[:1]) == "" && len(trimmed) >= len(fence) {
				fence = ""
			}
			lines[i], prev, gap = "", code, false
			continue
		}
		if m := refMark.FindStringSubmatch(line); m != nil {
			fence = m[1]
			lines[i], prev, gap = "", code, false
			continue
		}
		if strings.TrimSpace(line) == "" {
			gap = true
			continue
		}
		indented := strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
		switch {
		case indented && (prev == code || gap && prev != list):
			lines[i], prev = "", code
		case refList.MatchString(line):
			prev = list
		default:
			prev = prose
		}
		gap = false
	}
	return strings.Join(lines, "\n")
}

// refStripInline blanks the code spans and the HTML comments of text, in
// one pass from the left, so that what one holds can't open or close the
// other. A span is a run of backticks up to the next run of as many, in
// the same paragraph and before the next comment. A comment, which pull
// request templates use for hints, runs to its end. One that has none runs
// to the end of the text if it starts a line, as GitHub reads it, and is
// otherwise text.
func refStripInline(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], "<!--"):
			end := strings.Index(text[i+len("<!--"):], "-->")
			switch {
			case end >= 0:
				b.WriteByte(' ')
				i += len("<!--") + end + len("-->")
			case strings.TrimSpace(text[strings.LastIndex(text[:i], "\n")+1:i]) == "":
				// One that starts a line runs to the end of the text.
				b.WriteByte(' ')
				return b.String()
			default:
				// Mid-sentence, it is text.
				b.WriteString("<!--")
				i += len("<!--")
			}
		case text[i] == '`':
			n := 0
			for i+n < len(text) && text[i+n] == '`' {
				n++
			}
			limit := len(text)
			if m := refBreak.FindStringIndex(text[i+n:]); m != nil {
				limit = i + n + m[0]
			}
			if m := refItem.FindStringIndex(text[i+n : limit]); m != nil {
				limit = i + n + m[0]
			}
			end := refClosing(text[:limit], i+n, n)
			if end < 0 {
				b.WriteString(text[i : i+n])
				i += n
				continue
			}
			b.WriteByte(' ')
			i = end
		default:
			b.WriteByte(text[i])
			i++
		}
	}
	return b.String()
}

// refClosing returns the index after the first run of exactly n backticks
// in text at or after from, or -1.
func refClosing(text string, from, n int) int {
	for i := from; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}
		run := 0
		for i+run < len(text) && text[i+run] == '`' {
			run++
		}
		if run == n {
			return i + run
		}
		i += run
	}
	return -1
}
