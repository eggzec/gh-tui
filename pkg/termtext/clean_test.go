package termtext

import "testing"

func TestClean(t *testing.T) {
	tests := []struct {
		name, in, want string
		tab            int
	}{
		{name: "tabs to the next stop", in: "\ta\tbc\td", tab: 4, want: "    a   bc  d"},
		{name: "tab width", in: "a\tb", tab: 8, want: "a       b"},
		{name: "tabs after wide runes", in: "你\tb", tab: 4, want: "你  b"},
		{name: "tabs restart on each line", in: "abc\t1\n\t2", tab: 4, want: "abc 1\n    2"},
		{name: "crlf", in: "a\r\nb\r\n", tab: 4, want: "a\nb\n"},
		{name: "lone cr", in: "a\rb", tab: 4, want: "a�b"},
		{name: "escape sequences", in: "\x1b[31mred", tab: 4, want: "�[31mred"},
		{name: "c1 controls", in: "a\u009bb", tab: 4, want: "a�b"},
		{name: "invalid utf-8", in: "a\xffb", tab: 4, want: "a�b"},
		{name: "bidi overrides", in: "a\u202eb\u2066c\u2069d\u200f", tab: 4, want: "a�b�c�d�"},
		{name: "unicode is kept", in: "héllo wörld 👋", tab: 4, want: "héllo wörld 👋"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clean(tt.in, tt.tab); got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
