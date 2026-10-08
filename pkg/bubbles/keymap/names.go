package keymap

import (
	"runtime"
	"slices"
	"sync"
	"unsafe"
	"weak"

	"charm.land/bubbles/v2/key"
)

// names remembers which actions each binding was made for, so that help
// can say where a key is set in the config. A binding has no field for it,
// and is copied by value, so it is found by what its copies share: the
// array of its keys. A binding that is bound to nothing has one too, an
// empty slice with room for a key, which [Unbound] makes.
var names = struct {
	sync.Mutex
	byKeys map[weak.Pointer[string]][]string
}{byKeys: map[weak.Pointer[string]][]string{}}

// Unbound returns b as a disabled binding without keys that [Record] can
// tell from every other, such as one that the config unbinds. Without
// keys it never takes a key press, and while it is disabled it is off.
func Unbound(b key.Binding) key.Binding {
	b.SetKeys(make([]string, 0, 1)...)
	b.SetEnabled(false)
	return b
}

// identity returns what the copies of a binding with keys share, or false
// for keys that have no array of their own.
func identity(keys []string) (weak.Pointer[string], bool) {
	if cap(keys) == 0 {
		return weak.Pointer[string]{}, false
	}
	return weak.Make(unsafe.SliceData(keys)), true
}

// fresh returns b with an array of keys of its own, so that what is
// recorded for it is recorded for it alone.
func fresh(b key.Binding) key.Binding {
	keys := make([]string, len(b.Keys()), max(len(b.Keys()), 1))
	copy(keys, b.Keys())
	b.SetKeys(keys...)
	if len(keys) == 0 {
		// Without keys it is off, which an empty slice, unlike none,
		// doesn't say of itself.
		b.SetEnabled(false)
	}
	return b
}

// Record returns b, now known as the binding of actions, each named as the
// config names it, such as "pulls.merge", in addition to what it was
// known for. If b shares its keys with a binding known for other actions,
// it gets keys of its own first, so that the two don't answer for each
// other's. Bindings made from others should use [Derive].
func Record(b key.Binding, actions ...string) key.Binding {
	actions = slices.DeleteFunc(slices.Clone(actions), func(a string) bool { return a == "" })
	if len(actions) == 0 {
		return b
	}
	names.Lock()
	defer names.Unlock()
	if p, ok := identity(b.Keys()); ok {
		if have := names.byKeys[p]; len(have) > 0 && len(union(have, actions)) != len(have) {
			b = fresh(b)
			actions = union(have, actions)
		}
	} else {
		b = fresh(b)
	}
	keys := b.Keys()
	p, _ := identity(keys)
	_, known := names.byKeys[p]
	names.byKeys[p] = union(names.byKeys[p], actions)
	if !known {
		runtime.AddCleanup(unsafe.SliceData(keys), func(p weak.Pointer[string]) {
			names.Lock()
			defer names.Unlock()
			delete(names.byKeys, p)
		}, p)
	}
	return b
}

// Derive returns b, made from the bindings from, as the binding of all
// their actions. It gets keys of its own, since it may share them with
// one of those.
func Derive(b key.Binding, from ...key.Binding) key.Binding {
	var all []string
	for _, f := range from {
		all = union(all, Actions(f))
	}
	if len(all) == 0 {
		return b
	}
	return Record(fresh(b), all...)
}

// Actions returns the actions b was made for, as the config names them,
// or none.
func Actions(b key.Binding) []string {
	names.Lock()
	defer names.Unlock()
	p, ok := identity(b.Keys())
	if !ok {
		return nil
	}
	return slices.Clone(names.byKeys[p])
}

// union returns a with the names of b it lacks.
func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, n := range b {
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// Enable switches b on or off as on says, except that a binding without
// keys stays off: the keys of an action that the config unbinds are an
// empty slice that [Unbound] gave it, which unlike none doesn't keep a
// binding off by itself.
func Enable(b *key.Binding, on bool) { b.SetEnabled(on && len(b.Keys()) > 0) }
