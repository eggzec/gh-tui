package termtexttest

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// recorder notes whether a check failed, without failing the test.
type recorder struct {
	testing.TB
	failed bool
}

func (r *recorder) Helper()               {}
func (r *recorder) Errorf(string, ...any) { r.failed = true }

func TestAssertClean(t *testing.T) {
	tests := []struct {
		name, view string
		width      int
		fails      bool
	}{
		{name: "hostile", view: Hostile, width: 200, fails: true},
		{name: "cleaned", view: termtext.OneLine(Hostile), width: 200},
		{name: "colors", view: "\x1b[1;31mred\x1b[m\nplain", width: 5},
		{name: "link", view: "\x1b]8;;https://github.com/\x1b\\gh\x1b]8;;\x1b\\", width: 2},
		{name: "link elsewhere", view: "\x1b]8;;https://evil.test\x1b\\gh\x1b]8;;\x1b\\", width: 2, fails: true},
		{name: "c1 byte", view: "a\x9b2J", width: 5, fails: true},
		{name: "bidi", view: "a\u2066b", width: 5, fails: true},
		{name: "too wide", view: "abc", width: 2, fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{TB: t}
			AssertClean(r, tt.view, tt.width)
			if r.failed != tt.fails {
				t.Errorf("AssertClean(%q) failed = %v, want %v", tt.view, r.failed, tt.fails)
			}
		})
	}
}
