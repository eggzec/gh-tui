package config

import (
	"reflect"
	"strings"
	"sync"
)

// What the struct tags of Config say of a setting, besides its name:
//
//	when:"startup" why:"…"  the setting is read once, at startup, so the
//	                        set command refuses it and says why
//	scope:"global"          the setting may only be set at the top level
//	                        of the config file, not for one host or
//	                        profile: it is about the machine, the
//	                        terminal or the person, not the host
//
// A tag on a struct holds for everything in it, and when a field of it
// has a when tag of its own, that one holds for the field. No tag makes a
// field live again under a struct tagged startup, so where one field of
// a group must stay live, as log.level does, tag the others one by one.
const (
	whenStartup = "startup"
	scopeGlobal = "global"
)

// meta is what the tags say of a setting, a struct of them, or a map.
type meta struct {
	// startup says why the setting is read at startup, or is empty.
	startup string
	global  bool
}

// metas maps the path of every setting, struct of them and map in
// Config, such as "cache.disk" or "keys", to what its tags say.
var metas = sync.OnceValue(func() map[string]meta {
	out := map[string]meta{}
	var walk func(t reflect.Type, prefix string, inherit meta)
	walk = func(t reflect.Type, prefix string, inherit meta) {
		for f := range t.Fields() {
			m := inherit
			if f.Tag.Get("when") == whenStartup {
				m.startup = f.Tag.Get("why")
			}
			if f.Tag.Get("scope") == scopeGlobal {
				m.global = true
			}
			name := prefix + yamlName(f)
			out[name] = m
			if f.Type.Kind() == reflect.Struct {
				walk(f.Type, name+".", m)
			}
		}
	}
	walk(reflect.TypeFor[Config](), "", meta{})
	return out
})

// Startup reports whether the setting key is read only at startup, so
// that changing it takes a restart, and why.
func Startup(key string) (why string, ok bool) {
	m := metas()[key]
	return m.startup, m.startup != ""
}

// Global reports whether key, a setting, a group of them such as "log", a
// map such as "keys", or what is in one, such as "keys.global.quit", may
// only be set at the top level of the config file, and not for one host
// or profile.
func Global(key string) bool {
	for {
		if m, ok := metas()[key]; ok {
			return m.global
		}
		i := strings.LastIndex(key, ".")
		if i < 0 {
			return false
		}
		key = key[:i]
	}
}
