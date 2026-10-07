// Package keytest checks key maps in tests: that full help lists every
// binding, that no two bindings of one map hold the same key, and that
// every binding is tagged for keymap.Fill.
package keytest

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

var bindingType = reflect.TypeFor[key.Binding]()

// Complete fails tb unless km's FullHelp returns every exported key.Binding
// field of km, and of the structs it holds, exactly once. km is a struct
// or a pointer to one.
func Complete(tb testing.TB, km help.KeyMap) {
	tb.Helper()
	want := make(map[string]int)
	names := make(map[string][]string)
	collect(reflect.ValueOf(km), "", func(name string, b key.Binding) {
		id := identify(b)
		want[id]++
		names[id] = append(names[id], name)
	})
	got := make(map[string]int)
	for _, g := range km.FullHelp() {
		for _, b := range g {
			got[identify(b)]++
		}
	}
	for id, n := range want {
		if got[id] != n {
			tb.Errorf("FullHelp lists %s %d times, want %d", strings.Join(names[id], ", "), got[id], n)
		}
	}
	for id, n := range got {
		if _, ok := want[id]; !ok {
			tb.Errorf("FullHelp lists %d binding(s) %s that are no field", n, id)
		}
	}
}

// Tagged fails tb if an exported key.Binding field of km, or of the structs
// it holds, has no keymap tag, which keymap.Fill needs to fill it. km is a
// struct or a pointer to one.
func Tagged(tb testing.TB, km any) {
	tb.Helper()
	walkFields(reflect.ValueOf(km), "", func(name string, sf reflect.StructField, _ key.Binding) {
		if _, ok := sf.Tag.Lookup("keymap"); !ok {
			tb.Errorf("%s has no keymap tag", name)
		}
	})
}

// HelpTags fails tb if the help tag of a tagged key.Binding field of km, or
// of the structs it holds, is not the description the binding has, so that
// a key map filled by keymap.Fill is worded as it is today. km is a struct
// or a pointer to one.
func HelpTags(tb testing.TB, km any) {
	tb.Helper()
	walkFields(reflect.ValueOf(km), "", func(name string, sf reflect.StructField, b key.Binding) {
		if _, ok := sf.Tag.Lookup("keymap"); !ok {
			return
		}
		if got, want := sf.Tag.Get("help"), b.Help().Desc; got != want {
			tb.Errorf("%s: help tag is %q, but the binding says %q", name, got, want)
		}
	})
}

// NoConflicts fails tb if two enabled bindings of km's FullHelp hold the
// same key.
func NoConflicts(tb testing.TB, km help.KeyMap) {
	tb.Helper()
	for _, r := range keyhelp.Analyze([]keyhelp.Layer{keyhelp.FromHelp("", km, false)}) {
		for _, l := range r.Lost {
			tb.Errorf("%q (%s) and %q (%s) both hold %s",
				l.By.Help().Desc, strings.Join(l.By.Keys(), " "),
				r.Binding.Help().Desc, strings.Join(r.Binding.Keys(), " "), l.Key)
		}
	}
}

// collect calls f with every exported key.Binding field of v, and of the
// structs it holds, named by its path.
func collect(v reflect.Value, path string, f func(string, key.Binding)) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for i := range v.NumField() {
		sf := v.Type().Field(i)
		if !sf.IsExported() {
			continue
		}
		name := path + sf.Name
		if sf.Type == bindingType {
			f(name, v.Field(i).Interface().(key.Binding))
			continue
		}
		collect(v.Field(i), name+".", f)
	}
}

// identify returns what tells a binding apart: its keys and its help.
func identify(b key.Binding) string {
	h := b.Help()
	return strings.Join(b.Keys(), " ") + " (" + h.Key + ": " + h.Desc + ")"
}

// walkFields calls f with every exported key.Binding field of v, and of the
// structs it holds, with its path, declaration and value.
func walkFields(v reflect.Value, path string, f func(string, reflect.StructField, key.Binding)) {
	// Only the key map itself may be a pointer; keymap.Fill follows no
	// pointer or interface inside it, so neither does this.
	for path == "" && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for i := range v.NumField() {
		sf := v.Type().Field(i)
		if !sf.IsExported() {
			continue
		}
		name := path + sf.Name
		if sf.Type == bindingType {
			f(name, sf, v.Field(i).Interface().(key.Binding))
			continue
		}
		if sf.Type.Kind() == reflect.Struct {
			walkFields(v.Field(i), name+".", f)
		}
	}
}

// Table returns a lookup of the keys that table lists for each action, so
// that a bubble's tests can fill its key map with keys of their own rather
// than an app's config.
func Table(table map[string][]string) keymap.Lookup {
	return func(action string) []string { return table[action] }
}
