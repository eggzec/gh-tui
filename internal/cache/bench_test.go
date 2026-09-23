package cache

import (
	"strconv"
	"testing"
)

func benchKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "repos/eggzec/gh-tui/pulls/" + strconv.Itoa(i)
	}
	return keys
}

func filled(keys []string) *Cache[int] {
	c := New[int](WithCapacity(len(keys)))
	for i, k := range keys {
		c.Set(k, Entry[int]{Value: i})
	}
	return c
}

func BenchmarkGet(b *testing.B) {
	keys := benchKeys(1024)
	c := filled(keys)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		c.Get(keys[i%len(keys)])
		i++
	}
}

func BenchmarkGetParallel(b *testing.B) {
	keys := benchKeys(1024)
	c := filled(keys)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(keys[i%len(keys)])
			i++
		}
	})
}

func BenchmarkSet(b *testing.B) {
	// Twice the capacity, so half the Sets update and half evict.
	keys := benchKeys(2048)
	c := New[int](WithCapacity(1024))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		c.Set(keys[i%len(keys)], Entry[int]{Value: i})
		i++
	}
}
