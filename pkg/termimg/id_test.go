package termimg

import (
	"slices"
	"testing"
)

func TestIDRoundTrip(t *testing.T) {
	for msb := range 256 {
		for _, c := range []uint8{1, 2, 128, 255} {
			id := NewID(uint8(msb), c)
			if id.MSB() != uint8(msb) || id.Color() != c || !id.Valid() {
				t.Fatalf("NewID(%d, %d) = %#x: msb %d, color %d, valid %v", msb, c, uint32(id), id.MSB(), id.Color(), id.Valid())
			}
			if got := NewID(id.MSB(), id.Color()); got != id {
				t.Fatalf("round trip of %#x gave %#x", uint32(id), uint32(got))
			}
		}
	}
	for _, id := range []ID{0, NewID(3, 0), 0x0100_0101} {
		if id.Valid() {
			t.Errorf("%#x is valid", uint32(id))
		}
	}
}

func TestRandomMSB(t *testing.T) {
	for range 1000 {
		if RandomMSB() == 0 {
			t.Fatal("RandomMSB returned 0")
		}
	}
}

func TestPool(t *testing.T) {
	p := NewPool(9)
	first, _, evicted := p.Get("a")
	if evicted || first.MSB() != 9 || !first.Valid() {
		t.Fatalf("first ID %#x, evicted %v", uint32(first), evicted)
	}
	if again, _, _ := p.Get("a"); again != first {
		t.Errorf("a key got a second ID %#x", uint32(again))
	}
	seen := map[ID]bool{first: true}
	for i := range 254 {
		id, _, evicted := p.Get(string(rune('b' + i)))
		if evicted || seen[id] || !id.Valid() {
			t.Fatalf("key %d: ID %#x, evicted %v, seen %v", i, uint32(id), evicted, seen[id])
		}
		seen[id] = true
	}
	if n := len(p.All()); n != 255 {
		t.Fatalf("%d IDs in use, want 255", n)
	}
	// "a" is used again, so "b" is now the least recent.
	p.Lookup("a")
	id, old, evicted := p.Get("new")
	if !evicted || old != "b" {
		t.Fatalf("full pool evicted %q (%v), want b", old, evicted)
	}
	if _, ok := p.Lookup("b"); ok {
		t.Error("the evicted key keeps its ID")
	}
	if got, ok := p.Lookup("new"); !ok || got != id {
		t.Errorf("Lookup(new) = %#x, %v", uint32(got), ok)
	}
	p.Release("new")
	if slices.Contains(p.All(), id) {
		t.Error("a released ID is still in use")
	}
	if again, _, evicted := p.Get("newer"); again != id || evicted {
		t.Errorf("the released ID isn't reused first: %#x, evicted %v", uint32(again), evicted)
	}
}
