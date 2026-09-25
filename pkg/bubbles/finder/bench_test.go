package finder

import (
	"context"
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sahilm/fuzzy"
)

// benchSize is the number of paths of a very large repository, such as
// kubernetes/kubernetes.
const benchSize = 100_000

var benchCorpus = sync1(func() *corpus {
	c, _ := newCorpus(context.Background(), items(bigPaths(benchSize)...))
	return c
})

// sync1 returns a function that calls f once, and then returns what it
// returned.
func sync1[T any](f func() T) func() T {
	var v T
	done := false
	return func() T {
		if !done {
			v, done = f(), true
		}
		return v
	}
}

func BenchmarkCorpus(b *testing.B) {
	its := items(bigPaths(benchSize)...)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = newCorpus(context.Background(), its)
	}
}

// BenchmarkFilter scans all the paths for a query, as the first key of a
// query does, on one goroutine and on all of them.
func BenchmarkFilter(b *testing.B) {
	c := benchCorpus()
	for _, q := range []string{"r", "rend", "controller", "pkg/ctl", "zz_gen deep"} {
		for _, procs := range []int{1, runtime.GOMAXPROCS(0)} {
			b.Run(q+"/procs="+itoa(procs), func(b *testing.B) {
				defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
				b.ReportAllocs()
				for b.Loop() {
					_, _ = filter(context.Background(), c, q, nil, nil)
				}
			})
		}
	}
}

// BenchmarkKeystroke types a query a key at a time, each key narrowing
// from the result of the one before, as the finder does, and reports the
// time per key.
func BenchmarkKeystroke(b *testing.B) {
	c := benchCorpus()
	for _, q := range []string{"controller", "pkg/kubelet/pod"} {
		for _, procs := range []int{1, runtime.GOMAXPROCS(0)} {
			b.Run(q+"/procs="+itoa(procs), func(b *testing.B) {
				defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(procs))
				b.ReportAllocs()
				for b.Loop() {
					var prev *result
					for i := 1; i <= len(q); i++ {
						prev, _ = filter(context.Background(), c, q[:i], prev, nil)
					}
				}
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(q)), "ns/key")
			})
		}
	}
}

// BenchmarkSahilm scans all the paths with the matcher of the picker, for
// comparison.
func BenchmarkSahilm(b *testing.B) {
	paths := bigPaths(benchSize)
	for _, q := range []string{"r", "rend", "controller"} {
		b.Run(q, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = fuzzy.Find(q, paths)
			}
		})
	}
}

// benchFinder returns a finder at 60 by 40 over the large corpus with
// "rend" typed and the selection a few rows down.
func benchFinder(b *testing.B) Model {
	b.Helper()
	m := open(b, 60, 40, bigPaths(benchSize), WithSyncLimit(benchSize))
	m = typed(b, m, "rend")
	return keys(b, m, "down", "down", "down")
}

func BenchmarkView(b *testing.B) {
	m := benchFinder(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// BenchmarkUpdate moves the selection down and back up, which renders the
// rows on screen again.
func BenchmarkUpdate(b *testing.B) {
	m := benchFinder(b)
	down, up := tea.Msg(press("down")), tea.Msg(press("up"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		msg := down
		if i/30%2 == 1 {
			msg = up
		}
		i++
		m, _ = m.Update(msg)
	}
}

// BenchmarkUpdateType types a key that narrows the matches of "rend" and
// deletes it again, in Update, as a list below the sync limit does.
func BenchmarkUpdateType(b *testing.B) {
	m := benchFinder(b)
	typeKey, del := tea.Msg(press("e")), tea.Msg(press("backspace"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		msg := typeKey
		if i%2 == 1 {
			msg = del
		}
		i++
		m, _ = m.Update(msg)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
