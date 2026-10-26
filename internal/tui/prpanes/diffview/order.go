// Package diffview holds the pure helpers of the diff of a pull request: the
// label of a commit range, the order of its files, and the title and status
// note of the diff. Data goes in and strings come out; it has no state and
// draws nothing.
package diffview

import (
	"fmt"
	"slices"

	"github.com/eggzec/gh-tui/internal/core"
)

// shortSHA is the length of an abbreviated commit id.
const shortSHA = 7

// RangeLabel names a range of commits. from and to are 0-based inclusive
// indexes into the list of total commits, and first and last are the ids of
// the commits at those indexes. One commit is its short id alone; every commit
// reads "all N commits · first…last" and a part of them
// "k of N commits · first…last".
func (g Glyphs) RangeLabel(total, from, to int, first, last string) string {
	k := to - from + 1
	if k <= 1 {
		return short(first)
	}
	span := short(first) + g.Ellipsis + short(last)
	if from <= 0 && to >= total-1 {
		return fmt.Sprintf("all %d commits%s%s", total, g.Sep, span)
	}
	return fmt.Sprintf("%d of %d commits%s%s", k, total, g.Sep, span)
}

// short cuts a commit id to its abbreviated form.
func short(oid string) string {
	if len(oid) > shortSHA {
		return oid[:shortSHA]
	}
	return oid
}

// Order lists the paths of the files in the order the diff steps through
// them: sorted by byte order, without duplicates. It is the tree order with
// every folder open.
func Order(fs []core.CommitFile) []string {
	paths := make([]string, 0, len(fs))
	for _, f := range fs {
		paths = append(paths, f.Path)
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

// Step returns the path d places after path in order, with a negative d going
// back. It reports false at either end and when path is not in order, so the
// diff never wraps.
func Step(order []string, path string, d int) (string, bool) {
	i := slices.Index(order, path)
	if i < 0 {
		return "", false
	}
	i += d
	if i < 0 || i >= len(order) {
		return "", false
	}
	return order[i], true
}

// Place gives the 1-based place of path in order and the number of files. When
// path is not in order, i is 0.
func Place(order []string, path string) (i, n int) {
	return slices.Index(order, path) + 1, len(order)
}
