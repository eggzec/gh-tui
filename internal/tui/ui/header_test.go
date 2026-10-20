package ui

import "testing"

func TestHeader(t *testing.T) {
	t.Parallel()
	cols := []Column{
		{Title: "Name", Width: 8},
		{Title: "Description", Width: 0},
		{Title: "Stars", Width: 5, Right: true},
		{Title: "Updated", Width: 4},
	}
	tests := []struct {
		name string
		cols []Column
		want string
	}{
		{"drops zero width, aligns, truncates", cols, "Name      Stars  Upd…"},
		{"none", nil, ""},
		{"blank when only the tail would show", []Column{{Title: "Labels", Width: 1}, {Title: "Ok", Width: 3}}, "   Ok "},
		{"right pads left", []Column{{Title: "N", Width: 3, Right: true}}, "  N"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Header(tt.cols, 2, "…"); got != tt.want {
				t.Errorf("Header = %q, want %q", got, tt.want)
			}
		})
	}
}
