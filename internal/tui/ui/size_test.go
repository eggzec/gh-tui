package ui

import "testing"

func TestSize(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1025, "1.0K"},
		{1229, "1.2K"},
		{10 * 1024, "10K"},
		{10*1024 - 1, "10K"},
		{34_000, "33K"},
		{2_200_000, "2.1M"},
		{5 << 30, "5.0G"},
	}
	for _, tt := range tests {
		if got := Size(tt.n); got != tt.want {
			t.Errorf("Size(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
