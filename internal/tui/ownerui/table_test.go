package ownerui

import "testing"

func TestLayoutCols(t *testing.T) {
	m := Measure{name: 16, flags: 2, stars: 3}
	tests := []struct {
		name  string
		width int
		m     Measure
		want  Cols
	}{
		// The list of the 190, 140 and 80 column dashboards.
		{"190 columns", 100, m, Cols{name: 16, flags: 3, desc: 57, lang: 4, stars: 3, age: 7}},
		{"140 columns", 73, m, Cols{name: 16, flags: 3, desc: 30, lang: 4, stars: 3, age: 7}},
		{"80 columns", 76, m, Cols{name: 16, flags: 3, desc: 33, lang: 4, stars: 3, age: 7}},
		{"a long name is capped", 100, Measure{name: 60, stars: 1}, Cols{name: 30, desc: 50, lang: 4, stars: 1, age: 7}},
		{"no flags", 60, Measure{name: 8, stars: 1}, Cols{name: 8, desc: 32, lang: 4, stars: 1, age: 7}},
		{"the description gives way first", 44, m, Cols{name: 19, flags: 3, lang: 4, stars: 3, age: 7}},
		{"then the language", 32, m, Cols{name: 13, flags: 3, stars: 3, age: 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LayoutCols(tt.width, tt.m, "★", 4)
			if got != tt.want {
				t.Errorf("LayoutCols(%d) = %+v, want %+v", tt.width, got, tt.want)
			}
			if w := got.Width(); w != tt.width {
				t.Errorf("the columns take %d cells, want %d", w, tt.width)
			}
		})
	}
}
