package termimg

import "math/rand/v2"

// ID is the number a terminal knows an image by. Its low byte is the
// 256-color index that the placeholder cells' foreground carries, and its
// high byte the third diacritic of each cell; the bytes between are zero.
// 24-bit colors can't carry the low bytes, since a renderer may turn them
// into the nearest of 256 colors, and an index survives that.
type ID uint32

// NewID returns the ID of color index color, 1 to 255, and high byte msb.
func NewID(msb, color uint8) ID {
	return ID(msb)<<24 | ID(color)
}

// Color returns the 256-color index of id, its low byte.
func (id ID) Color() uint8 { return uint8(id) }

// MSB returns the high byte of id.
func (id ID) MSB() uint8 { return uint8(id >> 24) }

// Valid reports whether id can be drawn with placeholders: a color index
// from 1 to 255, and nothing in the middle bytes. Index 0 is left out,
// so an ID never has a low byte of zero, which a placeholder that has
// lost its color names.
func (id ID) Valid() bool {
	return id.Color() != 0 && id&0x00ffff00 == 0
}

// RandomMSB returns a high byte, 1 to 255, for the IDs of one process, so
// two programs drawing in the same terminal, such as in two tmux panes,
// rarely name the same images.
func RandomMSB() uint8 {
	return uint8(rand.IntN(255) + 1)
}

// Pool hands out the 255 IDs of one high byte, one for each key, such as
// an image's address, and reuses the one used least recently when all are
// taken. It is not safe for concurrent use.
type Pool struct {
	msb uint8
	// keys and used are indexed by color index; used counts uses, so the
	// smallest is the least recent, and 0 marks a free ID.
	keys [256]string
	used [256]uint64
	ids  map[string]ID
	tick uint64
}

// NewPool returns a pool of the IDs with high byte msb.
func NewPool(msb uint8) *Pool {
	return &Pool{msb: msb, ids: make(map[string]ID)}
}

// Lookup returns the ID of key, if it has one, and marks it used.
func (p *Pool) Lookup(key string) (ID, bool) {
	id, ok := p.ids[key]
	if ok {
		p.touch(id)
	}
	return id, ok
}

// Get returns the ID of key, giving it one if it has none, and marks it
// used. When it takes the ID of another key, replaced is true and evicted
// is that key, and the caller deletes that image from the terminal before
// it sends key's under the same ID.
func (p *Pool) Get(key string) (id ID, evicted string, replaced bool) {
	if id, ok := p.Lookup(key); ok {
		return id, "", false
	}
	c := 1
	for i := 1; i < len(p.used); i++ {
		if p.used[i] < p.used[c] {
			c = i
		}
	}
	id = NewID(p.msb, uint8(c))
	if p.used[c] != 0 {
		evicted, replaced = p.keys[c], true
		delete(p.ids, evicted)
	}
	p.keys[c] = key
	p.ids[key] = id
	p.touch(id)
	return id, evicted, replaced
}

// Release frees the ID of key, whose image the caller deleted.
func (p *Pool) Release(key string) {
	id, ok := p.ids[key]
	if !ok {
		return
	}
	delete(p.ids, key)
	p.keys[id.Color()], p.used[id.Color()] = "", 0
}

// All returns the IDs in use, such as to delete their images at exit.
func (p *Pool) All() []ID {
	ids := make([]ID, 0, len(p.ids))
	for c := 1; c < len(p.used); c++ {
		if p.used[c] != 0 {
			ids = append(ids, NewID(p.msb, uint8(c)))
		}
	}
	return ids
}

func (p *Pool) touch(id ID) {
	p.tick++
	p.used[id.Color()] = p.tick
}
