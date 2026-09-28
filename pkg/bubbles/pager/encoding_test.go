package pager

import (
	"strings"
	"testing"
)

// Text that isn't UTF-8 shows decoded, and searches and filters read it
// decoded.
func TestEncodings(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
		find       string
	}{
		{name: "utf-8 as it is", text: "Grüße 你好\n", want: []string{"Grüße 你好"}, find: "你好"},
		{name: "latin-1", text: "Gr\xfc\xdfe aus K\xf6ln\n", want: []string{"Grüße aus Köln"}, find: "Köln"},
		{name: "windows-1252", text: "\x93quoted\x94 \x96 it\x92s\n", want: []string{"“quoted” – it’s"}, find: "it’s"},
		{name: "bytes left show in hex", text: "你好 \xff\xfe\n", want: []string{"你好 <FF><FE>"}, find: "<FF>"},
		{name: "utf-16", text: "\xff\xfeh\x00i\x00\n\x00`O}Y\n\x00", want: []string{"hi", "你好"}, find: "你好"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "notes.txt", tt.text, WithSize(30, 4))
			if m.state != stateReady {
				t.Fatalf("state %v, want ready", m.state)
			}
			v := plain(m)
			for _, w := range tt.want {
				if !strings.Contains(v, w) {
					t.Errorf("view doesn't show %q:\n%s", w, v)
				}
			}
			assertFits(t, m.View(), 30, 4)
			m, _ = typeSearch(t, m, tt.find)
			if m.Matches() != 1 {
				t.Errorf("%d matches of %q, want 1", m.Matches(), tt.find)
			}
			m, _ = keys(t, m, "esc")
			m, _ = enterAll(t, m, "&", tt.find, "enter")
			if m.Shown() != 1 {
				t.Errorf("filter %q shows %d lines, want 1", tt.find, m.Shown())
			}
		})
	}
}
