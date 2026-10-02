package images

import "container/list"

// memory keeps the images made most recently, up to a size in bytes of
// their PNGs, their frames' too. It is not safe for concurrent use.
type memory struct {
	max, size int
	order     *list.List // of *memEntry, most recent first
	byKey     map[string]*list.Element
}

type memEntry struct {
	key string
	img Image
}

func newMemory(maxBytes int) *memory {
	return &memory{max: maxBytes, order: list.New(), byKey: make(map[string]*list.Element)}
}

func (m *memory) get(key string) (Image, bool) {
	e, ok := m.byKey[key]
	if !ok {
		return Image{}, false
	}
	m.order.MoveToFront(e)
	return e.Value.(*memEntry).img, true
}

func (m *memory) put(key string, img Image) {
	if img.size() > m.max {
		return
	}
	if e, ok := m.byKey[key]; ok {
		m.size -= e.Value.(*memEntry).img.size()
		m.order.Remove(e)
	}
	m.byKey[key] = m.order.PushFront(&memEntry{key: key, img: img})
	m.size += img.size()
	for m.size > m.max {
		e := m.order.Back()
		old := e.Value.(*memEntry)
		m.order.Remove(e)
		delete(m.byKey, old.key)
		m.size -= old.img.size()
	}
}
