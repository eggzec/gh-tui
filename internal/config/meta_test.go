package config

import (
	"reflect"
	"testing"
)

// TestTags checks the tags that say when a setting is read and where it
// may be set: a when tag says startup and why, and a scope tag says
// global, so that a typo in a tag doesn't silently make a setting live.
func TestTags(t *testing.T) {
	var walk func(t2 reflect.Type, prefix string)
	walk = func(t2 reflect.Type, prefix string) {
		for f := range t2.Fields() {
			name := prefix + yamlName(f)
			when, why, scope := f.Tag.Get("when"), f.Tag.Get("why"), f.Tag.Get("scope")
			if when != "" && when != whenStartup || when == whenStartup && why == "" || when == "" && why != "" {
				t.Errorf("%s: want when:%q with a why, or neither; got when:%q why:%q", name, whenStartup, when, why)
			}
			if scope != "" && scope != scopeGlobal {
				t.Errorf("%s: want scope:%q or none, got %q", name, scopeGlobal, scope)
			}
			if f.Type.Kind() == reflect.Struct {
				walk(f.Type, name+".")
			}
		}
	}
	walk(reflect.TypeFor[Config](), "")
}

func TestStartup(t *testing.T) {
	for key, want := range map[string]string{
		"repos":                          "the pinned repositories are read at startup",
		"cache.ttl.pulls":                "the cache is opened at startup",
		"cache.disk.dir":                 "the cache is opened at startup",
		"sync.enabled":                   "the polls are set up at startup",
		"sync.poll.lists":                "",
		"sync.unfocused_slowdown":        "the polls and the revalidation are set up at startup",
		"files.preview.max_size":         "the files are read with it from the start",
		"prefetch.files.preview.enabled": "",
		"auth.check":                     "the token's checks start with the app",
		"log.level":                      "",
		"log.keep":                       "the log file is opened at startup",
		"theme":                          "",
		"ui.icons":                       "",
		"editor":                         "",
		"nope":                           "",
	} {
		why, ok := Startup(key)
		if why != want || ok != (want != "") {
			t.Errorf("Startup(%q) = %q, %v; want %q", key, why, ok, want)
		}
	}
}

func TestGlobal(t *testing.T) {
	for key, want := range map[string]bool{
		"keys": true, "themes": true, "editor": true, "ui.icons": true,
		"keys.global.quit": true, "themes.dusk.dark.accent": true, "log.level.x": true, "theme.x": false,
		"log": true, "log.level": true, "log.file": true,
		"cache.disk.dir": true, "cache.disk.max_size": true, "cache.disk.compression": true, "cache.disk.compression_level": true,
		"cache.memory": true, "cache.memory.entries": true, "cache.memory.logs": true,
		"theme": false, "ui": false, "repos": false, "cache": false, "cache.ttl.pulls": false,
		"cache.disk.enabled": false, "cache.disk.entries": false, "cache.revalidate.per_minute": false,
		"sync.poll.lists": false, "prefetch.window.after": false, "nope": false,
	} {
		if got := Global(key); got != want {
			t.Errorf("Global(%q) = %v, want %v", key, got, want)
		}
	}
}
