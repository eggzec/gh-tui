package filterform

import (
	"context"
	"slices"
)

// Spec declares what a form edits: its fields, in the order they are shown
// and written to the query, and an optional sort.
type Spec struct {
	Fields []Field
	// Sort adds a Sort tab after the Filters tab. Nil leaves it out, and
	// the form has no tabs.
	Sort *SortField
}

// Kind is the kind of value a field holds, which decides how it is edited.
type Kind int

const (
	// Choice is one of the field's options, changed in place.
	Choice Kind = iota
	// Multi is any number of options, shown as a list of names and chosen
	// in a picker.
	Multi
	// Toggle is on or off.
	Toggle
	// Text is free text, edited in a text input.
	Text
	// Person is one user login, such as @me, chosen in a picker or typed.
	Person
)

// String returns the kind's name.
func (k Kind) String() string {
	switch k {
	case Choice:
		return "choice"
	case Multi:
		return "multi"
	case Toggle:
		return "toggle"
	case Text:
		return "text"
	case Person:
		return "person"
	default:
		return "unknown"
	}
}

// Item is one value a field can take.
type Item struct {
	// Label is what the form shows.
	Label string
	// Value is what the form writes to the query. For a Choice with no
	// Qualifier it is the whole token, such as review:approved. The empty
	// value writes nothing, which suits options such as "Any".
	Value string
	// Detail is shown after the label in the picker.
	Detail string
}

// Loader returns the options of a remote field, such as the labels of a
// repository. The form calls it in a command the first time the field is
// opened, and again when the user retries after an error. query is empty
// then; a Person field also calls it with what the user types, so it can
// search users that can't all be listed. The form cancels ctx when it is
// blurred, so it may block.
type Loader func(ctx context.Context, query string) ([]Item, error)

// Field is one row of the form.
type Field struct {
	// Key names the field in Values.
	Key string
	// Label is shown in the left column.
	Label string
	Kind  Kind
	// Options lists what a Choice, Multi or Person field offers. A Multi
	// or Person field may use Load instead.
	Options []Item
	// Load loads the options of a Multi or Person field when it is first
	// opened. Loaded options come after Options.
	Load Loader
	// Default is the value the form starts with and resets to.
	Default Value
	// Qualifier is the search qualifier the field writes and claims, such
	// as label or is: a Choice writes is:open, a Multi label:bug,docs, a
	// Text base:main and a Person author:@me. A Toggle writes Qualifier
	// whole when it is on, so it holds a full token such as -is:draft. A
	// Choice with no Qualifier writes its option's Value whole.
	Qualifier string
	// Each makes a Multi write one qualifier per value, label:a label:b,
	// which GitHub reads as all of them, instead of one comma list, which
	// it reads as any of them.
	Each bool
	// Hint is shown beside a Toggle's box, and in place of the value of an
	// empty Text, Multi or Person field.
	Hint string
	// Empty is the picker's text when a Multi or Person field has nothing
	// to offer.
	Empty string
	// Format, when set, writes the value instead of Qualifier: it returns
	// the query text for v, or "" for none.
	Format func(v Value) string
	// Parse, when set, claims tokens instead of Qualifier. It is given
	// each token the fields before it didn't claim, and the value so far,
	// which starts empty for every SetQuery. It returns the new value and
	// whether it claimed the token; the form keeps unclaimed tokens as
	// free text.
	Parse func(tok Token, v Value) (Value, bool)
}

// Sort is a sort order: the value of a sort option, and its direction.
type Sort struct {
	By   string
	Desc bool
}

// SortField declares the Sort tab: a row for what the list is sorted by,
// and one for the order. It writes sort:<by>-desc or sort:<by>-asc, or
// nothing for an option with an empty value, and claims sort: tokens with
// one of its options.
type SortField struct {
	// Label names the row of what is sorted by. The default is "Sort by".
	Label string
	// Options are what can be sorted by, such as updated, created and
	// comments.
	Options []SortOption
	// Default is the sort the form starts with and resets to.
	Default Sort
}

// SortOption is one thing a list can be sorted by.
type SortOption struct {
	// Label is what the form shows.
	Label string
	// Value is what the form writes after sort:, such as updated. The
	// empty value writes no sort, which suits "Best match", and has no
	// order.
	Value string
	// Desc and Asc name the orders, such as "Newest first" and "Oldest
	// first", after an arrow the form draws. The defaults are "Descending"
	// and "Ascending".
	Desc, Asc string
	// Ascending is the order the option sorts in once it is chosen, such
	// as names from A. Without it, it sorts descending, the latest or the
	// most first.
	Ascending bool
}

func (s Spec) clone() Spec {
	out := Spec{Fields: slices.Clone(s.Fields)}
	for i := range out.Fields {
		f := &out.Fields[i]
		f.Options = slices.Clone(f.Options)
		f.Default = f.Default.clone()
	}
	if s.Sort != nil {
		sf := *s.Sort
		sf.Options = slices.Clone(sf.Options)
		if sf.Label == "" {
			sf.Label = "Sort by"
		}
		for i := range sf.Options {
			o := &sf.Options[i]
			if o.Desc == "" {
				o.Desc = "Descending"
			}
			if o.Asc == "" {
				o.Asc = "Ascending"
			}
		}
		out.Sort = &sf
	}
	return out
}

// Value is the value of one field. Build one with [TextValue],
// [ListValue] or [BoolValue], and read it with the accessor for its
// field's kind.
type Value struct {
	text string
	list []string
	on   bool
}

// TextValue returns the value of a Choice, Text or Person field.
func TextValue(s string) Value { return Value{text: s} }

// ListValue returns the value of a Multi field. It keeps a copy.
func ListValue(items ...string) Value { return Value{list: slices.Clone(items)} }

// BoolValue returns the value of a Toggle field.
func BoolValue(on bool) Value { return Value{on: on} }

// Text returns the value of a Choice, Text or Person field.
func (v Value) Text() string { return v.text }

// List returns a copy of the values of a Multi field.
func (v Value) List() []string { return slices.Clone(v.list) }

// Bool returns whether a Toggle field is on.
func (v Value) Bool() bool { return v.on }

// IsZero reports whether the value is empty: no text, no items, and off.
func (v Value) IsZero() bool { return v.text == "" && len(v.list) == 0 && !v.on }

// Equal reports whether v and w hold the same value.
func (v Value) Equal(w Value) bool {
	return v.text == w.text && v.on == w.on && slices.Equal(v.list, w.list)
}

func (v Value) clone() Value {
	v.list = slices.Clone(v.list)
	return v
}
