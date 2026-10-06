package config

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestKeys(t *testing.T) {
	keys := Keys()
	for _, k := range []string{"repos", "theme", "ui.icons", "sync.poll.lists", "prefetch.window.after", "prefetch.files.preview.max_size", "history.row", "cache.disk.dir", "log.level"} {
		if !slices.Contains(keys, k) {
			t.Errorf("Keys() lacks %s", k)
		}
	}
	for _, k := range []string{"keys", "themes", "cache", "cache.disk", "keys.global", "keys.global.quit"} {
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
		"ui.icons":                        "nerd",
		"sync.enabled":                    "true",
		"sync.poll.notifications":         "1m",
		"sync.poll.lists":                 "1m",
		"sync.poll.actions":               "10s",
		"sync.poll.checks":                "15s",
		"sync.unfocused_slowdown":         "4",
		"prefetch.rest":                   "150ms",
		"prefetch.window.after":           "4",
		"prefetch.files.preview.max_size": "64KiB",
		"history.row":                     "[short_sha, subject, author, age]",
		"repos":                           "[]",
		"log.file":                        `""`,
	} {
		if got, err := c.Get(key); err != nil || got != want {
			t.Errorf("Get(%q) = %q, %v, want %q", key, got, err, want)
		}
	}
	for _, key := range []string{"", "nope", "keys.global.quit", "themes", "cache"} {
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
		{key: "sync.poll.lists", value: "30s", want: "30s"},
		{key: "sync.poll.lists", value: "10s", want: "10s"},
		{key: "sync.poll.lists", value: "-1s", err: "sync.poll.lists: must be at least 10s, got -1s"},
		{key: "sync.poll.lists", value: "1ms", err: "sync.poll.lists: must be at least 10s, got 1ms"},
		{key: "sync.poll.lists", value: "soon", err: `sync.poll.lists: can't read "soon"`},
		{key: "sync.unfocused_slowdown", value: "60", want: "60"},
		{key: "sync.unfocused_slowdown", value: "61", err: "sync.unfocused_slowdown: must be between 1 and 60, got 61"},
		// A factor that would overflow the intervals it multiplies.
		{key: "sync.unfocused_slowdown", value: "100000000", err: "sync.unfocused_slowdown: must be between 1 and 60, got 100000000"},
		{key: "sync.enabled", value: "false", want: "false"},
		{key: "sync.enabled", value: "maybe", err: `sync.enabled: can't read "maybe"`},
		{key: "prefetch.window.after", value: "10", want: "10"},
		{key: "prefetch.window.after", value: "40", err: "prefetch.window.after: must be between 0 and 30, got 40"},
		{key: "prefetch.rest", value: "1s", want: "1s"},
		{key: "prefetch.files.preview.max_size", value: "512KiB", want: "512KiB"},
		{key: "prefetch.files.preview.max_size", value: "2MiB", err: "prefetch.files.preview.max_size: must be between 0B and files.preview.max_size"},
		{key: "dashboard.calendar_glyph", value: "#", want: "#"},
		{key: "dashboard.calendar_glyph", value: `"▪"`, want: "▪"},
		{key: "history.row", value: "short_sha, subject", want: "[short_sha, subject]"},
		{key: "history.row", value: "[subject]", want: "[subject]"},
		{key: "history.row", value: "nope", err: `history.row[0]: unknown field "nope"`},
		{key: "repos", value: "cli/cli, eggzec/gh-tui", want: "[cli/cli, eggzec/gh-tui]"},
		{key: "keys.global.quit", value: "x", err: `unknown setting "keys.global.quit"`},
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
	if got.Sync != c.Sync || got.UI != c.UI || got.Notifications != c.Notifications {
		t.Error("Set changed other settings")
	}
}

// TestSetRefusedKeepsTheConfig checks that a list that fails validation
// leaves the config Set was called on as it was, down to the items of its
// lists.
func TestSetRefusedKeepsTheConfig(t *testing.T) {
	for key, value := range map[string]string{
		"history.row":    "subject, nope",
		"history.detail": "[body, nope]",
		"repos":          "cli/cli, nope",
	} {
		t.Run(key, func(t *testing.T) {
			c := Default()
			c.Repos = []string{"eggzec/gh-tui", "cli/cli"}
			want := Default()
			want.Repos = slices.Clone(c.Repos)
			got, err := c.Set(key, value)
			if err == nil {
				t.Fatalf("Set(%q, %q) took it", key, value)
			}
			if !reflect.DeepEqual(c, want) {
				v, _ := c.Get(key)
				t.Errorf("Set changed the config it was called on: %s is %s", key, v)
			}
			if !reflect.DeepEqual(got, want) {
				v, _ := got.Get(key)
				t.Errorf("Set returned a changed config with its error: %s is %s", key, v)
			}
		})
	}
}

func TestValues(t *testing.T) {
	c := Default()
	c.Themes["mine"] = Theme{}
	for key, want := range map[string][]string{
		"ui.icons":                {"nerd", "unicode", "ascii"},
		"theme":                   {"default", "mine"},
		"sync.enabled":            {"true", "false"},
		"dashboard.contributions": {"30d", "90d", "year"},
		"sync.poll.lists":         nil,
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

// TestReset checks that Reset copies the value as it is, so that a string
// that Set would read as quoted comes back unchanged, and that a list
// comes back as a list of its own.
func TestReset(t *testing.T) {
	file := Default()
	file.Editor = `"C:\tools\vim"`
	session, err := file.Set("editor", "vim")
	if err != nil {
		t.Fatal(err)
	}
	got, err := session.Reset("editor", file)
	if err != nil || got.Editor != file.Editor {
		t.Errorf("Reset(editor) = %q, %v; want %q", got.Editor, err, file.Editor)
	}
	got, err = session.Reset("history.row", file)
	if err != nil {
		t.Fatal(err)
	}
	got.History.Row[0] = FieldAge
	if file.History.Row[0] == FieldAge {
		t.Error("Reset shares its list with the config it copies from")
	}
	if _, err := session.Reset("nope", file); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("Reset(nope) = %v, want ErrUnknownKey", err)
	}
}
