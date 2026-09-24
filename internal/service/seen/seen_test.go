package seen

import (
	"sync"
	"testing"
	"time"
)

func TestMarks(t *testing.T) {
	var m Marks[int]
	if _, ok := m.Get("a"); ok {
		t.Fatal("Get on the zero Marks found a mark")
	}
	if _, ok := m.Set("repo/a#1", 1); ok {
		t.Error("first Set reported a previous mark")
	}
	if prev, ok := m.Set("repo/a#1", 2); !ok || prev != 1 {
		t.Errorf("Set = %d, %v, want 1, true", prev, ok)
	}
	m.Set("repo/a#2", 3)
	m.Set("repo/b#1", 4)

	m.Delete("repo/a#2")
	if _, ok := m.Get("repo/a#2"); ok {
		t.Error("Get found a deleted mark")
	}
	m.DeletePrefix("repo/a#")
	if _, ok := m.Get("repo/a#1"); ok {
		t.Error("DeletePrefix kept a mark under the prefix")
	}
	if v, ok := m.Get("repo/b#1"); !ok || v != 4 {
		t.Errorf("Get(repo/b#1) = %d, %v, want 4, true", v, ok)
	}
}

func TestMarksConcurrent(*testing.T) {
	var m Marks[int]
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			m.Set("k", i)
			m.Get("k")
			m.DeletePrefix("x")
		})
	}
	wg.Wait()
}

func TestCurrent(t *testing.T) {
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name    string
		v, mark time.Time
		want    bool
	}{
		{"same", t0, t0, true},
		{"newer", t0.Add(time.Second), t0, true},
		{"older", t0, t0.Add(time.Second), false},
		{"no version", time.Time{}, t0, false},
		{"no mark", t0, time.Time{}, false},
	}
	for _, tt := range tests {
		if got := Current(tt.v, tt.mark); got != tt.want {
			t.Errorf("%s: Current = %v, want %v", tt.name, got, tt.want)
		}
	}
}
