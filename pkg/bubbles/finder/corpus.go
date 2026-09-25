package finder

import (
	"context"
	"strings"
)

// maxItems is the most items the finder searches. Ranking packs an item's
// index into 21 bits; GitHub cuts a listing short well before that.
const maxItems = 1 << 21

// corpus holds the items prepared for matching, once per load: every path
// folded to lower case in one buffer, so that scanning them stays in
// cache, and the bonus of every byte for a match there.
type corpus struct {
	items []Item
	// text holds the folded paths back to back; the path of item i is
	// text[start[i]:start[i+1]], and bonus holds the bonus of each byte.
	text  []byte
	bonus []int8
	start []int32
	// index finds an item by its path, for the recent paths.
	index map[string]int32
}

// newCorpus prepares items for matching. It stops early when ctx is done.
func newCorpus(ctx context.Context, items []Item) (*corpus, error) {
	items = items[:min(len(items), maxItems)]
	n := 0
	for i := range items {
		n += len(items[i].Path)
	}
	c := &corpus{
		items: items,
		text:  make([]byte, 0, n),
		bonus: make([]int8, 0, n),
		start: make([]int32, 0, len(items)+1),
		index: make(map[string]int32, len(items)),
	}
	for i := range items {
		if i%4096 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		p := items[i].Path
		c.start = append(c.start, int32(len(c.text)))
		c.text = appendFolded(c.text, p)
		c.bonus = appendBonus(c.bonus, p)
		if _, ok := c.index[p]; !ok {
			c.index[p] = int32(i)
		}
	}
	c.start = append(c.start, int32(len(c.text)))
	return c, nil
}

// len returns the number of items.
func (c *corpus) len() int {
	return len(c.items)
}

// path returns the folded path of item i and the bonuses of its bytes.
func (c *corpus) path(i int32) (path []byte, bonus []int8) {
	a, b := c.start[i], c.start[i+1]
	return c.text[a:b], c.bonus[a:b]
}

// appendFolded appends s folded to lower case. Only ASCII letters fold,
// so the offsets of the folded path are those of s.
func appendFolded(dst []byte, s string) []byte {
	for i := range len(s) {
		dst = append(dst, lower(s[i]))
	}
	return dst
}

func lower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// fold folds s to lower case as appendFolded does.
func fold(s string) string {
	return string(appendFolded(make([]byte, 0, len(s)), s))
}

// Classes of bytes, for the bonuses of word boundaries.
type class uint8

const (
	classLower class = iota
	classUpper
	classDigit
	classSlash
	// classDelim is other punctuation that separates words, such as _, -,
	// . and spaces.
	classDelim
)

func classOf(c byte) class {
	switch {
	case 'a' <= c && c <= 'z', c >= 0x80:
		// Bytes of other scripts count as letters without case.
		return classLower
	case 'A' <= c && c <= 'Z':
		return classUpper
	case '0' <= c && c <= '9':
		return classDigit
	case c == '/':
		return classSlash
	}
	return classDelim
}

// bonusAt returns the bonus of matching a byte of class cur after one of
// class prev: most at the start of a directory or file name, then at the
// start of a word, then at a change of case or to digits.
func bonusAt(prev, cur class) int8 {
	switch {
	case cur == classSlash || cur == classDelim:
		return bonusDelim
	case prev == classSlash:
		return bonusSegment
	case prev == classDelim:
		return bonusBoundary
	case prev == classLower && cur == classUpper, prev != classDigit && cur == classDigit:
		return bonusCamel
	}
	return 0
}

// appendBonus appends the bonus of each byte of path p. The bytes of the
// file name get bonusBase on top, so that a match there ranks above the
// same match in a directory.
func appendBonus(dst []int8, p string) []int8 {
	base := strings.LastIndexByte(p, '/') + 1
	prev := classSlash
	for i := range len(p) {
		cur := classOf(p[i])
		b := bonusAt(prev, cur)
		if i >= base {
			b += bonusBase
		}
		dst = append(dst, b)
		prev = cur
	}
	return dst
}
