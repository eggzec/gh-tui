package panes

import "testing"

func TestSizeOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		off         int
		def, lo, hi int
		want        int
	}{
		{"not moved", 0, 40, 24, 96, 40},
		{"grown", 6, 40, 24, 96, 46},
		{"shrunk", -6, 40, 24, 96, 34},
		{"past the top", 100, 40, 24, 96, 96},
		{"past the bottom", -100, 40, 24, 96, 24},
		{"no room", 10, 40, 24, 20, 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := Size{off: tt.off}
			if got := s.Of(tt.def, tt.lo, tt.hi); got != tt.want {
				t.Errorf("Of(%d, %d, %d) = %d, want %d", tt.def, tt.lo, tt.hi, got, tt.want)
			}
		})
	}
}

func TestSizeMoveResetSettle(t *testing.T) {
	t.Parallel()
	var s Size
	if s.Moved() {
		t.Fatal("zero Size is moved")
	}
	s.Move(2)
	s.Move(-5)
	if !s.Moved() || s.Of(40, 24, 96) != 37 {
		t.Fatalf("after moves Of = %d", s.Of(40, 24, 96))
	}
	s.Reset()
	if s.Moved() {
		t.Error("Reset left the Size moved")
	}
	for range 20 {
		s.Move(-2)
		s.Settle(40, 24, 96)
	}
	if got := s.Of(40, 24, 96); got != 24 {
		t.Fatalf("at the limit Of = %d, want 24", got)
	}
	s.Move(2)
	s.Settle(40, 24, 96)
	if got := s.Of(40, 24, 96); got != 26 {
		t.Errorf("one press back = %d, want 26", got)
	}
	// A terminal that shrinks drops the part past the new bound.
	s = Size{off: 30}
	s.Settle(40, 24, 60)
	if got := s.Of(40, 24, 60); got != 60 || s.Of(40, 24, 96) != 60 {
		t.Errorf("Settle kept overshoot: %d", got)
	}
	s.Settle(40, 24, 20)
	if s.Of(40, 24, 96) != 60 {
		t.Error("Settle with no room changed the offset")
	}
}
