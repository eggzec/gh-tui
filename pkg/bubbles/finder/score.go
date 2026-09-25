package finder

import (
	"bytes"
	"iter"
	"slices"
)

// Scores of a match, after fzf's: each matched byte scores scoreMatch
// plus its bonus, the first byte of the query twice its bonus, and a gap
// between matched bytes costs scoreGapStart for its first byte and
// scoreGapExtension for each after it. A run of consecutive matches keeps
// the best bonus of the run, at least bonusConsecutive, so that a word
// matched from its start counts as a whole. fzf gives a run less, but
// then a query spelled out inside a file name, such as controller in
// responsecontroller.go, loses to its letters picked from word starts
// all over a long path.
const (
	scoreMatch        = 16
	scoreGapStart     = -3
	scoreGapExtension = -1

	// bonusSegment is the bonus of the first byte of a directory or file
	// name, bonusBoundary that of a word after punctuation, and bonusCamel
	// that of a capital after a lower-case letter or of the first digit.
	bonusSegment  = 10
	bonusBoundary = 8
	bonusCamel    = 7
	// bonusDelim is the bonus of matching punctuation such as a slash.
	bonusDelim       = 8
	bonusConsecutive = bonusBoundary
	// bonusBase is added to every byte of the file name, so that a match
	// in the name beats the same match in a directory.
	bonusBase = 4

	firstMultiplier = 2
)

// none stands for no alignment in the tables of the scorer. Adding the
// penalties of any path to it leaves it far below every real score.
const none = -1 << 24

// scorer scores terms against paths. It reuses its buffers, so each
// goroutine has its own.
type scorer struct {
	// first and last are the earliest and latest offset each byte of the
	// term may match at.
	first, last []int
	// rows hold the offsets where a byte of the term matches, the one
	// before and the one being filled, each with the best score of the
	// term up to that byte matched there.
	rows [2][]cell
}

// cell is an offset where a byte of the term matches, the best score of
// the term up to that byte with it matched there, and the bonus that a run
// through it carries.
type cell struct {
	pos, score, carry int32
}

// score returns the score of the best alignment of term, folded, in path,
// whose bytes have the bonuses, or false if term isn't a subsequence of
// path. It finds the same score as align, but looks only at the offsets
// where each byte of term matches, between the earliest and the latest
// it can: a gap costs the same for every alignment that ends where it
// does, so the best way into a byte after a gap is the best of the
// alignments before it, weighed by where they end.
func (s *scorer) score(term, path []byte, bonus []int8) (int, bool) {
	n, k := len(path), len(term)
	if k == 0 {
		return 0, true
	}
	if k > n {
		return 0, false
	}
	if len(s.first) < k {
		s.first, s.last = make([]int, k), make([]int, k)
	}
	first, last := s.first[:k], s.last[:k]
	pos := 0
	for i, c := range term {
		j := bytes.IndexByte(path[pos:], c)
		if j < 0 {
			return 0, false
		}
		first[i] = pos + j
		pos += j + 1
	}
	end := n
	for i := k - 1; i >= 0; i-- {
		end = bytes.LastIndexByte(path[:end], term[i])
		last[i] = end
	}
	if slices.Equal(first, last) {
		// Each byte can match at one offset only, so there is nothing to
		// choose.
		return fixed(first, bonus), true
	}

	prev := s.rows[0][:0]
	for j := range offsets(path, term[0], first[0], last[0]) {
		b := int32(bonus[j])
		prev = append(prev, cell{pos: int32(j), score: scoreMatch + firstMultiplier*b, carry: b})
	}
	cur := s.rows[1][:0]
	for i := 1; i < k; i++ {
		cur = cur[:0]
		c := term[i]
		// before is the best score of the cells of prev at least two
		// offsets back, less what a gap from each to offset 0 would cost,
		// and p the next cell to weigh in. q finds the cell right before.
		before, p, q := int32(none), 0, 0
		for j := range offsets(path, c, first[i], last[i]) {
			for p < len(prev) && int(prev[p].pos) <= j-2 {
				before = max(before, prev[p].score-prev[p].pos*scoreGapExtension)
				p++
			}
			for q < len(prev) && int(prev[q].pos) < j-1 {
				q++
			}
			b := int32(bonus[j])
			run, cb := int32(none), b
			if q < len(prev) && int(prev[q].pos) == j-1 {
				cb = max(b, prev[q].carry, bonusConsecutive)
				run = prev[q].score + scoreMatch + cb
			}
			gap := int32(none)
			if before > none/2 {
				gap = before + scoreGapStart + int32(j-2)*scoreGapExtension + scoreMatch + b
			}
			switch {
			case run >= gap && run > none/2:
				cur = append(cur, cell{pos: int32(j), score: run, carry: cb})
			case gap > none/2:
				cur = append(cur, cell{pos: int32(j), score: gap, carry: b})
			}
		}
		prev, cur = cur, prev
	}
	// Keep the grown buffers for the next path.
	s.rows[0], s.rows[1] = prev, cur
	best := int32(none)
	for _, x := range prev {
		best = max(best, x.score)
	}
	return int(best), best > none/2
}

// offsets yields the offsets from lo to hi, inclusive, where path has c.
func offsets(path []byte, c byte, lo, hi int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for j := lo; j <= hi; j++ {
			x := bytes.IndexByte(path[j:hi+1], c)
			if x < 0 || !yield(j+x) {
				return
			}
			j += x
		}
	}
}

// fixed returns the score of the alignment at the offsets pos, with the
// bonuses of the path.
func fixed(pos []int, bonus []int8) int {
	score := scoreMatch + firstMultiplier*int(bonus[pos[0]])
	carry := int(bonus[pos[0]])
	for i := 1; i < len(pos); i++ {
		b := int(bonus[pos[i]])
		if gap := pos[i] - pos[i-1] - 1; gap > 0 {
			score += scoreGapStart + (gap-1)*scoreGapExtension
			carry = b
		} else {
			carry = max(b, carry, bonusConsecutive)
		}
		score += scoreMatch + carry
	}
	return score
}

// align returns the score of the best alignment of term in path, as score
// does, and the offsets of the bytes it matches, or false. It fills the
// whole tables and keeps them, so it is only for the few rows on screen.
func align(term, path []byte, bonus []int8) (score int, pos []int, ok bool) {
	n, k := len(path), len(term)
	if k == 0 {
		return 0, nil, true
	}
	m, e := table(k, n), table(k, n)
	// run records whether the best score at m[i][j] continues a run from
	// m[i-1][j-1], and open whether e[i][j] opens its gap after a match at
	// j-2 rather than extending one.
	run, open := make([][]bool, k), make([][]bool, k)
	carry := table(k, n)
	for i := range k {
		run[i], open[i] = make([]bool, n), make([]bool, n)
		for j := range n {
			e[i][j] = none
			if j >= 2 {
				start, ext := m[i][j-2]+scoreGapStart, e[i][j-1]+scoreGapExtension
				e[i][j], open[i][j] = max(start, ext), start >= ext
			}
			m[i][j] = none
			if path[j] != term[i] {
				continue
			}
			b := int32(bonus[j])
			if i == 0 {
				m[i][j], carry[i][j] = scoreMatch+firstMultiplier*b, b
				continue
			}
			if j == 0 {
				continue
			}
			r, cb := int32(none), max(b, carry[i-1][j-1], bonusConsecutive)
			if m[i-1][j-1] > none/2 {
				r = m[i-1][j-1] + scoreMatch + cb
			}
			g := int32(none)
			if e[i-1][j] > none/2 {
				g = e[i-1][j] + scoreMatch + b
			}
			if r >= g {
				m[i][j], carry[i][j], run[i][j] = r, cb, true
			} else {
				m[i][j], carry[i][j] = g, b
			}
		}
	}
	best, at := int32(none), -1
	for j, v := range m[k-1] {
		if v > best {
			best, at = v, j
		}
	}
	if best <= none/2 {
		return 0, nil, false
	}
	pos = make([]int, k)
	for i := k - 1; i >= 0; i-- {
		pos[i] = at
		if i == 0 {
			break
		}
		if run[i][at] {
			at--
			continue
		}
		// The gap before at ends in a match of the row above, two or more
		// offsets before it.
		g := at
		for !open[i-1][g] {
			g--
		}
		at = g - 2
	}
	return int(best), pos, true
}

func table(k, n int) [][]int32 {
	t := make([][]int32, k)
	for i := range t {
		t[i] = make([]int32, n)
	}
	return t
}
