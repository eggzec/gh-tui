// Package keymap fills the key maps of the bubbles from a source of keys,
// such as a config, so that a bubble holds no key of its own.
//
// A key map is a struct of key.Binding fields. Each field carries the name
// of its action and its help text as tags:
//
//	PageDown key.Binding `keymap:"page_down" help:"page down"`
//
// Fill sets the field to the keys that its source gives for the action. A
// name with a dot, such as "global.select", names an action of another
// context, which the source resolves; a name without one is the bubble's
// own. A struct field of a key map that is a key map itself may carry a
// keymap tag too, naming the context its fields' names belong to.
package keymap

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
)

// Lookup returns the keys of action, such as "page_down", or none.
type Lookup func(action string) []string

var bindingType = reflect.TypeFor[key.Binding]()

// Fill sets every exported key.Binding field of km, and of the structs it
// holds, that has a keymap tag to the keys that look gives for the tag's
// name, labelled by the first of them and the help tag. An action without
// keys, which a config unbinds, gives a disabled binding that keeps its
// help text, so help lists it without a key. Fields without a keymap tag
// are left as they are. A struct field's tag replaces the context of the
// struct that holds it rather than nesting under it. The bindings get
// their own copy of the keys.
//
// km must be a pointer to a struct, and a keymap tag must be on a
// key.Binding or a struct; Fill panics otherwise, since that is a mistake
// in the key map's declaration.
func Fill(km any, look Lookup) {
	v := reflect.ValueOf(km)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Sprintf("keymap.Fill: want a non-nil pointer to a struct, got %T", km))
	}
	walk(v.Elem(), "", func(name, help string, f reflect.Value) {
		keys := slices.Clone(look(name))
		if len(keys) == 0 {
			f.Set(reflect.ValueOf(key.NewBinding(key.WithHelp("", help), key.WithDisabled())))
			return
		}
		f.Set(reflect.ValueOf(key.NewBinding(key.WithKeys(keys...), key.WithHelp(Label(keys[0]), help))))
	})
}

// Names returns the names of the actions that km's tagged bindings and
// those of the structs it holds are for, sorted and without repeats. km is
// a struct or a pointer to one.
func Names(km any) []string {
	v := reflect.ValueOf(km)
	for v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		panic(fmt.Sprintf("keymap.Names: want a struct or a pointer to one, got %T", km))
	}
	// A copy is addressable, which walk needs for its fields.
	c := reflect.New(v.Type()).Elem()
	c.Set(v)
	var names []string
	walk(c, "", func(name, _ string, _ reflect.Value) { names = append(names, name) })
	slices.Sort(names)
	return slices.Compact(names)
}

// walk calls f with the name, help text and field of every tagged
// key.Binding of v, a struct, and of the structs in it. A struct's tag is
// the context that the names of its fields belong to, which they inherit
// from it when they have no tag of their own.
func walk(v reflect.Value, ctx string, f func(name, help string, field reflect.Value)) {
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag, tagged := sf.Tag.Lookup("keymap")
		switch {
		case sf.Type == bindingType:
			if tagged {
				f(qualify(ctx, tag), sf.Tag.Get("help"), v.Field(i))
			}
		case sf.Type.Kind() == reflect.Struct:
			inner := ctx
			if tagged {
				inner = tag
			}
			walk(v.Field(i), inner, f)
		case tagged:
			panic(fmt.Sprintf("keymap: field %s of %s has a keymap tag but is a %s, not a key.Binding", sf.Name, t, sf.Type))
		}
	}
}

// qualify returns the name of an action tagged name within context ctx: a
// name that has a dot is another context's already.
func qualify(ctx, name string) string {
	if ctx == "" || strings.Contains(name, ".") {
		return name
	}
	return ctx + "." + name
}

// Label shortens a key name for help: "enter" is "↵", "up" is "↑", and
// "ctrl+x" is "^x".
func Label(k string) string {
	switch k {
	case "enter":
		return "↵"
	case "esc":
		return "esc"
	case "space", " ":
		return "space"
	case "up":
		return "↑"
	case "down":
		return "↓"
	}
	return strings.ReplaceAll(k, "ctrl+", "^")
}
