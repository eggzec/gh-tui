package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestKeys(t *testing.T) {
	keys := Keys()
	for _, k := range []string{"repos", "theme", "ui.icons", "sync.interval", "details.prefetch.rows", "files.prefetch.max_size", "history.row", "cache.disk.dir", "log.level"} {
		if !slices.Contains(keys, k) {
			t.Errorf("Keys() lacks %s", k)
		}
	}
	for _, k := range []string{"keys", "themes", "cache", "cache.disk", "keys.quit"} {
		if slices.Contains(keys, k) {
			t.Errorf("Keys() has %s, which holds no value of its own", k)
		}
	}
	keys[0] = "changed"
	if Keys()[0] == "changed" {
		t.Error("Keys() shares its slice")
	}
}

func TestGet(t *testing.T) {
	c := Default()
	for key, want := range map[string]string{
		"ui.icons":                     "nerd",
		"sync.enabled":                 "true",
		"sync.interval":                "1m",
		"details.prefetch.hover_delay": "150ms",
		"details.prefetch.rows":        "5",
		"files.prefetch.max_size":      "64KiB",
		"history.row":                  "[short_sha, subject, author, age]",
		"repos":                        "[]",
		"log.file":                     `""`,
	} {
		if got, err := c.Get(key); err != nil || got != want {
			t.Errorf("Get(%q) = %q, %v, want %q", key, got, err, want)
		}
	}
	for _, key := range []string{"", "nope", "keys.quit", "themes", "cache"} {
		if _, err := c.Get(key); !errors.Is(err, ErrUnknownKey) {
			t.Errorf("Get(%q) error = %v, want ErrUnknownKey", key, err)
		}
	}
}

func TestSet(t *testing.T) {
	tests := []struct {
		key, value string
		// want is what Get returns after, or err a substring of the error.
		want, err string
	}{
		{key: "ui.icons", value: "unicode", want: "unicode"},
		{key: "ui.icons", value: "'ascii'", want: "ascii"},
		{key: "ui.icons", value: "emoji", err: `ui.icons: must be nerd, unicode or ascii, got "emoji"`},
		{key: "ui.icons", value: "", err: `ui.icons: must be nerd, unicode or ascii, got ""`},
		{key: "theme", value: "nosuch", err: `theme: unknown theme "nosuch"`},
		{key: "sync.interval", value: "30s", want: "30s"},
		{key: "sync.interval", value: "-1s", err: "sync.interval: must be positive"},
		{key: "sync.interval", value: "soon", err: `sync.interval: can't read "soon"`},
		{key: "sync.enabled", value: "false", want: "false"},
		{key: "sync.enabled", value: "maybe", err: `sync.enabled: can't read "maybe"`},
		{key: "details.prefetch.rows", value: "10", want: "10"},
		{key: "details.prefetch.rows", value: "40", err: "details.prefetch.rows: must be between 0 and 30, got 40"},
		{key: "details.prefetch.hover_delay", value: "1s", want: "1s"},
		{key: "files.prefetch.max_size", value: "512KiB", want: "512KiB"},
		{key: "files.prefetch.max_size", value: "2MiB", err: "files.prefetch.max_size: must not exceed files.preview.max_size"},
		{key: "dashboard.calendar_glyph", value: "#", want: "#"},
		{key: "dashboard.calendar_glyph", value: `"▪"`, want: "▪"},
		{key: "history.row", value: "short_sha, subject", want: "[short_sha, subject]"},
		{key: "history.row", value: "[subject]", want: "[subject]"},
		{key: "history.row", value: "nope", err: `history.row[0]: unknown field "nope"`},
		{key: "repos", value: "cli/cli, eggzec/gh-tui", want: "[cli/cli, eggzec/gh-tui]"},
		{key: "keys.quit", value: "x", err: `unknown setting "keys.quit"`},
		{key: "nope", value: "x", err: `unknown setting "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			c := Default()
			before, _ := c.Get(tt.key)
			got, err := c.Set(tt.key, tt.value)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("Set error = %v, want one containing %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Set error = %v", err)
			}
			if v, _ := got.Get(tt.key); v != tt.want {
				t.Errorf("Get after Set = %q, want %q", v, tt.want)
			}
			if after, _ := c.Get(tt.key); after != before {
				t.Errorf("Set changed the config it was called on: %q, was %q", after, before)
			}
		})
	}
}

func TestSetKeepsTheRest(t *testing.T) {
	c := Default()
	got, err := c.Set("history.row", "subject")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.History.Row, Default().History.Row) {
		t.Errorf("the list set is shared with the config before: %v", c.History.Row)
	}
	if got.Sync.Interval != time.Minute || got.UI != c.UI || got.Details != c.Details {
		t.Error("Set changed other settings")
	}
}

func TestValues(t *testing.T) {
	c := Default()
	c.Themes = map[string]Theme{"mine": {}}
	for key, want := range map[string][]string{
		"ui.icons":                {"nerd", "unicode", "ascii"},
		"theme":                   {"default", "mine"},
		"sync.enabled":            {"true", "false"},
		"dashboard.contributions": {"30d", "90d", "year"},
		"sync.interval":           nil,
		"history.row":             nil,
		"nope":                    nil,
	} {
		if got := c.Values(key); !slices.Equal(got, want) {
			t.Errorf("Values(%q) = %q, want %q", key, got, want)
		}
	}
}

// TestValuesAreValid checks that every value Values offers is one that
// Set takes, so that completion never offers what validation refuses.
func TestValuesAreValid(t *testing.T) {
	c := Default()
	for _, key := range Keys() {
		for _, v := range c.Values(key) {
			if _, err := c.Set(key, v); err != nil {
				t.Errorf("Set(%q, %q) = %v", key, v, err)
			}
		}
	}
}
