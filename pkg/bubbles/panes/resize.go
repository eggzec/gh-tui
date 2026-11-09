package panes

// Size is how far the user moved one divider from where the layout puts it.
// The zero value is a divider that was not moved. The caller keeps one Size
// per divider, and passes it the default position and the bounds of the
// moment, so that a move is an offset: it scales when the terminal does.
type Size struct{ off int }

// Move shifts the divider by delta cells; a negative delta moves it left or
// up.
func (s *Size) Move(delta int) { s.off += delta }

// Reset puts the divider back where the layout puts it.
func (s *Size) Reset() { s.off = 0 }

// Moved reports whether the divider is away from its default.
func (s Size) Moved() bool { return s.off != 0 }

// Of returns the divider's position: def moved by the offset, held between lo
// and hi. When the bounds leave no room (hi < lo) it returns def.
func (s Size) Of(def, lo, hi int) int {
	if hi < lo {
		return def
	}
	return min(max(def+s.off, lo), hi)
}

// Settle drops the part of the offset that goes past a bound, so that presses
// at a limit build up nothing and the first press back moves at once.
func (s *Size) Settle(def, lo, hi int) {
	if hi < lo {
		return
	}
	s.off = min(max(def+s.off, lo), hi) - def
}
