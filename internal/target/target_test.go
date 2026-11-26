package target

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want Target
		err  string
	}{
		{in: "laraibg786@github.com", want: Target{"laraibg786", "github.com"}},
		{in: "laraibg786@corp.ghe.com", want: Target{"laraibg786", "corp.ghe.com"}},
		{in: "hubot@www.github.com", want: Target{"hubot", "github.com"}},
		{in: "user hubot", want: Target{"hubot", "github.com"}},
		{in: "  user   hubot ", want: Target{"hubot", "github.com"}},
		{in: "host corp.ghe.com", want: Target{"me", "corp.ghe.com"}},
		{in: "host localhost:8080", want: Target{"me", "localhost:8080"}},
		{in: "host https://Corp.GHE.com/", want: Target{"me", "corp.ghe.com"}},
		{in: "", err: "usage: :switch"},
		{in: "  ", err: "usage: :switch"},
		{in: "hubot", err: "usage: :switch"},
		{in: "corp.ghe.com", err: "usage: :switch"},
		{in: "@corp.ghe.com", err: "usage: :switch"},
		{in: "hubot@", err: "usage: :switch"},
		{in: "@", err: "usage: :switch"},
		{in: "a@b@c.com", err: "usage: :switch"},
		{in: "user", err: "usage: :switch"},
		{in: "host", err: "usage: :switch"},
		{in: "user a b", err: "usage: :switch"},
		{in: "user a@b.com", err: "not a GitHub login"},
		{in: "host a@b.com", err: "names no host"},
		{in: "team hubot", err: "usage: :switch"},
		{in: "a@b c.com", err: "usage: :switch"},
		{in: "me@corp.com/api/v3", err: "names no host"},
		{in: "bad/name@github.com", err: "not a GitHub login"},
		{in: "user bad!", err: "not a GitHub login"},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in, "me", "github.com")
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("Parse(%q) error = %v, want one with %q", tt.in, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("Parse(%q) = %v, %v, want %v", tt.in, got, err, tt.want)
		}
	}
}

func TestValidHost(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"github.com", "ghe.example.com:8443", "https://ghe.example.com/"} {
		if !ValidHost(h) {
			t.Errorf("ValidHost(%q) = false", h)
		}
	}
	for _, h := range []string{"", "a/b", "u@host", "host/api/v3", "host?x=1"} {
		if ValidHost(h) {
			t.Errorf("ValidHost(%q) = true", h)
		}
	}
}
