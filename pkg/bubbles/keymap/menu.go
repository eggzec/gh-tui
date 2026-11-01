package keymap

import (
	"reflect"

	"charm.land/bubbles/v2/key"
)

// menuTag marks a field of a key map as an action of the context menu of its
// item, as in `menu:"item"`. Only marked fields are listed there.
const (
	menuTag  = "menu"
	menuItem = "item"
)

// Menu returns the bindings of the exported fields of km, a struct or a
// pointer to one, that are tagged `menu:"item"`, in field order: the actions
// the context menu offers on the item the keys act on. A binding keeps its
// state, so one that is disabled for the item is still returned.
func Menu(km any) []key.Binding {
	v := reflect.ValueOf(km)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	bindingType := reflect.TypeFor[key.Binding]()
	var out []key.Binding
	for i := range v.NumField() {
		f := v.Type().Field(i)
		if !f.IsExported() || f.Type != bindingType || f.Tag.Get(menuTag) != menuItem {
			continue
		}
		out = append(out, v.Field(i).Interface().(key.Binding))
	}
	return out
}
