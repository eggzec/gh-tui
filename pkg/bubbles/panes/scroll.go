package panes

import "strings"

// Scrolls remembers how far each of several previews is scrolled, by a key
// that names what the preview shows, so a row keeps its place while the
// cursor visits other rows. The zero value is ready to use.
type Scrolls[K comparable] struct {
	offs map[K]int
}

// Get is the offset kept for k, 0 when none was set.
func (s *Scrolls[K]) Get(k K) int { return s.offs[k] }

// Has reports whether an offset was set for k.
func (s *Scrolls[K]) Has(k K) bool {
	_, ok := s.offs[k]
	return ok
}

// Set keeps off as the offset of k.
func (s *Scrolls[K]) Set(k K, off int) {
	if s.offs == nil {
		s.offs = map[K]int{}
	}
	s.offs[k] = off
}

// Reset forgets the offset of k.
func (s *Scrolls[K]) Reset(k K) { delete(s.offs, k) }

// Clamp is the offset kept for k, held to the last one that still fills h
// rows of a body of lines lines. It keeps nothing.
func (s *Scrolls[K]) Clamp(k K, lines, h int) int {
	return min(max(0, s.offs[k]), max(0, lines-h))
}

// Window is body from line off on, with off held to the last offset that
// still fills h rows, and that offset.
func Window(body string, off, h int) (window string, held int) {
	lines := strings.Split(body, "\n")
	off = min(max(0, off), max(0, len(lines)-h))
	return strings.Join(lines[off:], "\n"), off
}
