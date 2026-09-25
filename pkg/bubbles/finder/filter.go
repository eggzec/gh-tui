package finder

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// terms splits a query into its terms, folded, each of which a path must
// contain in order, though not necessarily together.
func terms(query string) []string {
	return strings.Fields(fold(query))
}

// narrows reports whether every path that matches the terms of next also
// matches those of prev, so that next need only look through the matches
// of prev: when each term of prev is a subsequence of a term of next.
func narrows(prev, next []string) bool {
	for _, p := range prev {
		if !slices.ContainsFunc(next, func(n string) bool { return subsequence(p, n) }) {
			return false
		}
	}
	return true
}

// subsequence reports whether a's bytes appear in b in order.
func subsequence(a, b string) bool {
	for i := range len(a) {
		j := strings.IndexByte(b, a[i])
		if j < 0 {
			return false
		}
		b = b[j+1:]
	}
	return true
}

// result is what a query found: the indexes of the matching items, best
// first.
type result struct {
	query string
	terms []string
	// all reports whether the query matched every item, as the empty one
	// does, so that narrowing from it looks through all of them.
	all   bool
	items []int32
}

// Recent paths rank above others that match as well: the most recent by
// bonusRecent plus maxRecent, the least by bonusRecent plus one.
const (
	maxRecent   = 16
	bonusRecent = 24
)

// parallelMin is the fewest candidates worth splitting among goroutines.
const parallelMin = 16384

// filter returns the items of c that match the terms of query, best first:
// by score, then by the length of the path, then in the order of the
// items. It looks only through from, if it is set. recent holds the rank
// of each recent item, 0 for the most recent. It stops early, returning
// false, when ctx is done.
func filter(ctx context.Context, c *corpus, query string, from *result, recent map[int32]int) (*result, bool) {
	ts := terms(query)
	if len(ts) == 0 {
		return everything(c, query, recent), true
	}
	// The matches of from are looked through in the order of the items,
	// not of their rank, so that reading the paths stays in cache.
	var set []uint64
	n := c.len()
	if from != nil && !from.all {
		set, n = bitset(from.items, c.len()), len(from.items)
	}
	workers := 1
	if n >= parallelMin {
		workers = min(runtime.GOMAXPROCS(0), n/(parallelMin/2))
	}
	parts := make([][]uint64, workers)
	done := make([]bool, workers)
	bts := make([][]byte, len(ts))
	for i, t := range ts {
		bts[i] = []byte(t)
	}
	run := func(w int) {
		lo, hi := w*c.len()/workers, (w+1)*c.len()/workers
		parts[w], done[w] = rank(ctx, c, bts, set, lo, hi, recent)
	}
	if workers == 1 {
		run(0)
	} else {
		var wg sync.WaitGroup
		for w := range workers {
			wg.Go(func() { run(w) })
		}
		wg.Wait()
	}
	if slices.Contains(done, false) {
		return nil, false
	}
	keys := slices.Concat(parts...)
	sortKeys(keys)
	items := make([]int32, len(keys))
	for i, k := range keys {
		items[i] = int32(k & idxMask)
	}
	return &result{query: query, terms: ts, items: items}, true
}

// bitset returns the set of items, of n in all.
func bitset(items []int32, n int) []uint64 {
	set := make([]uint64, (n+63)/64)
	for _, i := range items {
		set[i/64] |= 1 << (i % 64)
	}
	return set
}

// rank scores the items lo to hi that are in set, or all of them if it is
// nil, and returns the keys of those that match.
func rank(ctx context.Context, c *corpus, ts [][]byte, set []uint64, lo, hi int, recent map[int32]int) ([]uint64, bool) {
	var s scorer
	keys := make([]uint64, 0, (hi-lo)/4)
	for k := lo; k < hi; k++ {
		if (k-lo)%4096 == 0 && ctx.Err() != nil {
			return nil, false
		}
		if set != nil {
			if w := set[k/64]; w>>(k%64) == 0 {
				// No more items of this word are in the set.
				k |= 63
				continue
			} else if w&(1<<(k%64)) == 0 {
				continue
			}
		}
		i := int32(k)
		path, bonus := c.path(i)
		total, ok := 0, true
		for _, t := range ts {
			var v int
			if v, ok = s.score(t, path, bonus); !ok {
				break
			}
			total += v
		}
		if !ok {
			continue
		}
		if r, ok := recent[i]; ok {
			total += bonusRecent + maxRecent - r
		}
		keys = append(keys, rankKey(total, len(path), i))
	}
	return keys, true
}

// Keys pack what ranks an item into one number that sorts ascending: the
// score, negated, then the length of the path, then the index.
const (
	idxBits  = 21
	lenBits  = 11
	idxMask  = 1<<idxBits - 1
	lenMask  = 1<<lenBits - 1
	maxScore = 1<<(64-idxBits-lenBits-1) - 1
)

func rankKey(score, length int, i int32) uint64 {
	score = min(max(score, -maxScore), maxScore)
	return uint64(maxScore-score)<<(idxBits+lenBits) | uint64(min(length, lenMask))<<idxBits | uint64(i)
}

// sortKeys sorts keys with a radix sort on the bits they use, which is
// several times faster than comparing them for the many matches of a short
// query.
func sortKeys(keys []uint64) {
	if len(keys) < 256 {
		slices.Sort(keys)
		return
	}
	const bits = 11
	const buckets = 1 << bits
	var hi uint64
	for _, k := range keys {
		hi |= k
	}
	tmp := make([]uint64, len(keys))
	src, dst := keys, tmp
	for shift := uint(0); shift < 64 && hi>>shift != 0; shift += bits {
		var count [buckets]int
		for _, k := range src {
			count[k>>shift&(buckets-1)]++
		}
		if count[src[0]>>shift&(buckets-1)] == len(src) {
			// Every key has the same digit here.
			continue
		}
		sum := 0
		for b, n := range &count {
			count[b] = sum
			sum += n
		}
		for _, k := range src {
			b := k >> shift & (buckets - 1)
			dst[count[b]] = k
			count[b]++
		}
		src, dst = dst, src
	}
	if &src[0] != &keys[0] {
		copy(keys, src)
	}
}

// everything returns every item, the recent ones first, most recent
// first, then the rest in order.
func everything(c *corpus, query string, recent map[int32]int) *result {
	items := make([]int32, 0, c.len())
	if len(recent) > 0 {
		first := make([]int32, 0, len(recent))
		for i := range recent {
			first = append(first, i)
		}
		slices.SortFunc(first, func(a, b int32) int { return recent[a] - recent[b] })
		items = append(items, first...)
	}
	for i := range int32(c.len()) {
		if _, ok := recent[i]; !ok {
			items = append(items, i)
		}
	}
	return &result{query: query, all: true, items: items}
}
