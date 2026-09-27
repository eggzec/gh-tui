package markdown

import (
	"strings"
	"testing"
)

func TestSafe(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "plain", in: "text", want: "text"},
		{name: "styles stay", in: "\x1b[38;5;39;1mA\x1b[m \x1b[38:2::1:2:3mB\x1b[0m", want: "\x1b[38;5;39;1mA\x1b[m \x1b[38:2::1:2:3mB\x1b[0m"},
		{name: "a 256 color with 5 is not blink", in: "\x1b[38;5;5ma", want: "\x1b[38;5;5ma"},
		{name: "blink goes", in: "\x1b[5ma\x1b[1;6mb", want: "ab"},
		{name: "conceal goes", in: "\x1b[8ma", want: "a"},
		{name: "private forms go", in: "\x1b[>4;2ma\x1b[?25l", want: "[>4;2ma[?25l"},
		{name: "other escapes lose their ESC", in: "\x1b]0;t\x07x\x1b[2J", want: "]0;t�x[2J"},
		{name: "long sequences go", in: "\x1b[" + strings.Repeat("1;", 40) + "1ma", want: "a"},
		{name: "many parameters go", in: "\x1b[" + strings.Repeat("1;", 32) + "1ma", want: "a"},
		{name: "colors past 255 go", in: "\x1b[38;2;256;0;0ma\x1b[38;5;999mb\x1b[38:2::1:2:300mc", want: "abc"},
		{name: "huge numbers go", in: "\x1b[38;2;99999999999;1;1ma", want: "a"},
		{name: "colors up to 255 stay", in: "\x1b[38;2;255;0;0ma", want: "\x1b[38;2;255;0;0ma"},
		{name: "controls", in: "a\x08b\rc\x7fd\u009be", want: "a�b�c�d�e"},
		{name: "tab", in: "a\tb", want: "a b"},
		{name: "bidi", in: "a\u202eb\u2066c", want: "a�b�c"},
		{name: "invalid utf-8", in: "a\xffb", want: "a�b"},
		{name: "unicode", in: "héllo 👋 ▸", want: "héllo 👋 ▸"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safe(tt.in); got != tt.want {
				t.Errorf("safe(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRoom(t *testing.T) {
	for _, tt := range []struct{ width, margin, want int }{
		{80, 4, 76}, {4, 4, 1}, {2, 6, 1}, {0, 4, 0}, {-1, 4, 0},
	} {
		if got := Room(tt.width, tt.margin); got != tt.want {
			t.Errorf("Room(%d, %d) = %d, want %d", tt.width, tt.margin, got, tt.want)
		}
	}
}
