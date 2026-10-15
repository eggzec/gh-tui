package keyhelp

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
)

// Status is what becomes of a binding's keys.
type Status int

const (
	// Active is a binding that some of its keys reach.
	Active Status = iota
	// Disabled is a binding that is turned off for now.
	Disabled
	// Shadowed is a binding that loses a key to a binding of an earlier
	// layer.
	Shadowed
	// Conflict is a binding that loses a key to an earlier binding of its
	// own layer.
	Conflict
	// Typed is a binding whose keys are all typed into the input of an
	// earlier layer.
	Typed
)

// String returns the name of the status.
func (s Status) String() string {
	switch s {
	case Active:
		return "active"
	case Disabled:
		return "disabled"
	case Shadowed:
		return "shadowed"
	case Conflict:
		return "conflict"
	case Typed:
		return "typed"
	}
	return "unknown"
}

// Row is a binding as the help lists it.
type Row struct {
	Binding key.Binding
	// Source is the source of the binding's layer, and Layer its index.
	Source string
	Layer  int
	Status Status
	// Lost holds the keys of the binding that reach something else, in
	// the order of its keys.
	Lost []Loss
}

// Loss is a key that a binding doesn't get.
type Loss struct {
	Key string
	// By is the binding that gets the key, and Source its layer's source.
	// A key typed in has a zero By and the source of the layer that types
	// it.
	By     key.Binding
	Source string
	// Status is Conflict, Shadowed or Typed.
	Status Status
}

// Analyze returns a row for every binding of layers, in order, and finds
// which binding each key reaches: the first enabled binding that holds it,
// unless a layer before that one types it. A later enabled binding with
// the key loses it, in a conflict within one layer and shadowed across
// layers. A binding that loses a key to another is marked so even if its
// other keys work; one whose keys are all typed is Typed. A binding listed
// again, with the same keys and help, loses nothing to itself. A binding
// listed again in one layer gets one row. So does a disabled one listed
// again, which is on if any copy is: it stands where the first copy that is
// on is, in its layer, or where the first copy is when none is on.
// An enabled binding listed again in a later layer keeps its row there.
func Analyze(layers []Layer) []Row {
	rows, _ := analyze(layers)
	return rows
}

// analyze is Analyze that also says, for each row, which bindings it stands
// for: the position of each in the bindings of all layers, counted layer
// after layer. A row that folds copies lists them all.
func analyze(layers []Layer) (listed []Row, from [][]int) {
	type claim struct {
		b     key.Binding
		layer int
	}
	claimed := make(map[string]claim)
	var rows []Row
	var srcs [][]int
	// dropped marks the rows that a later copy of the same binding replaced.
	var dropped []bool
	// firsts is the row that stands for each binding, by what identifies it:
	// the newest that was added.
	firsts := make(map[string]int)
	// typing is the first layer so far that types printable keys, or -1.
	typing := -1
	n := -1
	for li, l := range layers {
		for _, b := range l.Bindings {
			n++
			r := Row{Binding: b, Source: l.Source, Layer: li, Status: Disabled}
			id := identity(b)
			first, seen := firsts[id]
			// A binding without keys, which a state may have switched on,
			// takes none.
			if !b.Enabled() || len(b.Keys()) == 0 {
				if seen {
					srcs[first] = append(srcs[first], n)
					continue
				}
				firsts[id] = len(rows)
				rows = append(rows, r)
				srcs = append(srcs, []int{n})
				dropped = append(dropped, false)
				continue
			}
			if seen && rows[first].Status != Disabled && rows[first].Layer == li {
				// A layer that lists a binding twice, such as two key maps
				// that both have the key that closes, shows it once.
				srcs[first] = append(srcs[first], n)
				continue
			}
			won := false
			for _, k := range b.Keys() {
				if c, ok := claimed[k]; ok {
					if same(b, c.b) {
						// A layer that lists a binding again, such as the
						// keys a section claims before the app's and then
						// lists with its own, loses nothing to itself.
						continue
					}
					st := Shadowed
					if c.layer == li {
						st = Conflict
					}
					r.Lost = append(r.Lost, Loss{Key: k, By: c.b, Source: layers[c.layer].Source, Status: st})
					continue
				}
				if typing >= 0 && Printable(k) {
					r.Lost = append(r.Lost, Loss{Key: k, Source: layers[typing].Source, Status: Typed})
					continue
				}
				claimed[k] = claim{b: b, layer: li}
				won = true
			}
			r.Status = status(r.Lost, won)
			// A copy that is on stands for the binding, in place of one that
			// is off: a row that works shows where it works, not where an
			// off twin was listed.
			off := seen && rows[first].Status == Disabled
			src := []int{n}
			if off {
				dropped[first] = true
				src = append(slices.Clone(srcs[first]), n)
			}
			// The newest row stands for the binding, so that a layer that
			// lists it again folds into its own row, not an earlier layer's.
			firsts[id] = len(rows)
			rows = append(rows, r)
			srcs = append(srcs, src)
			dropped = append(dropped, false)
		}
		if l.Typing && typing < 0 {
			typing = li
		}
	}
	kept, keptSrcs := rows[:0], srcs[:0]
	for i, r := range rows {
		if !dropped[i] {
			kept = append(kept, r)
			keptSrcs = append(keptSrcs, srcs[i])
		}
	}
	return kept, keptSrcs
}

// identity returns what tells a binding apart in the help: its keys and its
// key and description as listed.
func identity(b key.Binding) string {
	h := b.Help()
	return strings.Join(b.Keys(), "\x00") + "\x01" + h.Key + "\x01" + h.Desc
}

// same reports whether a and b are one binding listed twice: the same
// keys, named and described alike.
func same(a, b key.Binding) bool {
	return a.Help() == b.Help() && slices.Equal(a.Keys(), b.Keys())
}

// status returns the status of an enabled binding that lost keys, and won
// some others if won is set.
func status(lost []Loss, won bool) Status {
	st := Active
	for _, l := range lost {
		switch l.Status {
		case Conflict:
			return Conflict
		case Shadowed:
			st = Shadowed
		case Active, Disabled, Typed:
		}
	}
	if st == Active && !won && len(lost) > 0 {
		return Typed
	}
	return st
}

// Printable reports whether k, a key as tea.KeyPressMsg.String names it,
// types a character, so that a layer that types takes it.
func Printable(k string) bool {
	if k == "space" {
		return true
	}
	r, n := utf8.DecodeRuneInString(k)
	return n == len(k) && r != utf8.RuneError && unicode.IsPrint(r)
}
