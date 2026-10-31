package feed

import (
	"strings"
	"time"
)

// Level is a field a list is grouped or sorted by, as its user chose it.
type Level struct {
	// Name is the name of the Field.
	Name string
	// Desc flips the field's direction.
	Desc bool
}

// Field is a way to group or sort the items of a feed.
type Field[T any] struct {
	// Name is what a Level calls the field by.
	Name string
	// Key returns the key an item is filed under, "" for none.
	Key func(T) string
	// Label returns what a group's header reads for its key. Without it the
	// header reads the key.
	Label func(key string) string
	// Time, when set, makes the field a time field: it only sorts, newest
	// first unless the level is Desc, and Groups leaves it out.
	Time func(T) time.Time
}

// Groups returns the groups of the levels, outermost first. A level that names
// no field, or a time field, is left out.
func Groups[T any](fields []Field[T], levels []Level) []Group[T] {
	var out []Group[T]
	for _, l := range levels {
		for _, f := range fields {
			if f.Name != l.Name || f.Time != nil || f.Key == nil {
				continue
			}
			out = append(out, Group[T]{Key: f.Key, Label: f.Label, Desc: l.Desc})
		}
	}
	return out
}

// Less returns the order of the levels for slices.SortFunc: each level in
// turn, an item without a key after those with one whichever way the level
// runs, and the newest by tie among those the levels leave equal. Without tie
// they stay equal. A level that names no field is skipped.
func Less[T any](fields []Field[T], levels []Level, tie func(T) time.Time) func(a, b T) int {
	type step struct {
		field Field[T]
		desc  bool
	}
	var steps []step
	for _, l := range levels {
		for _, f := range fields {
			if f.Name == l.Name && (f.Time != nil || f.Key != nil) {
				steps = append(steps, step{f, l.Desc})
			}
		}
	}
	return func(a, b T) int {
		for _, s := range steps {
			var d int
			if s.field.Time != nil {
				d = s.field.Time(b).Compare(s.field.Time(a))
			} else {
				ka, kb := s.field.Key(a), s.field.Key(b)
				switch {
				case ka == kb:
				case ka == "":
					return 1
				case kb == "":
					return -1
				default:
					d = strings.Compare(ka, kb)
				}
			}
			if s.desc {
				d = -d
			}
			if d != 0 {
				return d
			}
		}
		if tie == nil {
			return 0
		}
		return tie(b).Compare(tie(a))
	}
}
