package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestChangedDefaults(t *testing.T) {
	if got := Default().Changed(); len(got) != 0 {
		t.Errorf("Default().Changed() = %v, want none", got)
	}
}

func TestChanged(t *testing.T) {
	const secret = "ghp_16C7e42F292c6912E7710c838347Ae178B4a"
	c := Default()
	c.Sync.Interval = 30 * time.Second
	c.Keys[ActionQuit] = []string{"x"}
	c.Repos = []string{"eggzec/private"}
	c.Editor = "vim --token " + secret
	c.Log.File = "/home/someone/" + secret + ".log"
	c.Cache.Disk.Dir = "/home/someone/cache"

	got := c.Changed()
	want := []Setting{
		{"cache.disk.dir", Redacted},
		{"editor", Redacted},
		{"keys.quit", "[x]"},
		{"log.file", Redacted},
		{"repos", Redacted},
		{"sync.interval", "30s"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Changed() = %v, want %v", got, want)
	}
	for _, s := range got {
		if strings.Contains(s.Value, secret) || strings.Contains(s.Value, "someone") {
			t.Errorf("Changed() gave away %s = %q", s.Key, s.Value)
		}
	}
}

func TestPrivate(t *testing.T) {
	for key, want := range map[string]bool{
		"editor":              true,
		"cache.disk.dir":      true,
		"auth.token":          true,
		"future.proxy_url":    true,
		"sync.interval":       false,
		"auth.check":          false,
		"keys.open_file":      false,
		"themes.mine.dark.fg": false,
	} {
		if got := private(key); got != want {
			t.Errorf("private(%q) = %v, want %v", key, got, want)
		}
	}
}
