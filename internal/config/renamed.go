package config

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// rename is a setting, or several, that moved, and how a file that still
// has them under their old names is read.
//
// An old name is the path of a setting that held a value or a list, never
// a group of them, and never one inside a list or a map entry, such as a
// key or a theme: only there is a path the same in every file. Its old
// form is that value, so where the file has a group at an old path, it is
// in the new form already, as for a setting that keeps its name and
// becomes a group, cache.ttl: 5m becoming cache.ttl.pulls and the rest.
type rename struct {
	// old are the paths the settings had, such as "details.prefetch.rows".
	// Most renames have one. Several old settings that became one, such
	// as three delays that became prefetch.rest, share a rename, so that
	// move sees them all.
	old []string
	// new are the paths they moved to.
	new []string
	// note says what changed besides the name, such as a count that now
	// leaves out the row under the cursor, or is empty.
	note string
	// move returns the values of the new paths, by path, from those of the
	// old ones that the file has, by path: at least one. It may leave out
	// a new path, but not return one that new doesn't list, nor a nil
	// node. It is nil once the release that read the old names has passed:
	// they are then refused, with an error that names the new ones, so
	// nobody has to guess.
	move func(old map[string]*yaml.Node) (map[string]*yaml.Node, error)
}

// renames are the settings that moved, in the order they are moved in:
// where two set one path, the later wins, so the more specific of two
// old settings comes later.
var renames = []rename{
	// One interval was every poll's but those of the Actions runs and
	// checks, which have intervals of their own now too.
	{
		old: []string{"sync.interval"}, new: []string{"sync.poll.notifications", "sync.poll.lists"},
		move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			return map[string]*yaml.Node{"sync.poll.notifications": v["sync.interval"], "sync.poll.lists": v["sync.interval"]}, nil
		},
	},
	// The history's date format is every date's now.
	{
		old: []string{"history.date_format"}, new: []string{"ui.date_format"},
		note: "it now applies to every date",
		move: func(old map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			return map[string]*yaml.Node{"ui.date_format": old["history.date_format"]}, nil
		},
	},
	renameTo("files.prefetch.enabled", "prefetch.files.preview.enabled"),
	renameTo("files.prefetch.max_size", "prefetch.files.preview.max_size"),
	restRename("files.prefetch.hover_delay", "prefetch.files.rest"),
	{
		old: []string{"history.prefetch.around"}, new: []string{"prefetch.history.window.before", "prefetch.history.window.after"},
		note: "the commits on each side of the cursor",
		move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			n := v["history.prefetch.around"]
			return map[string]*yaml.Node{"prefetch.history.window.before": n, "prefetch.history.window.after": n}, nil
		},
	},
	restRename("history.prefetch.hover_delay", "prefetch.history.rest"),
}

// renameTo moves the value of the setting from to the setting to.
func renameTo(from, to string) rename {
	return rename{old: []string{from}, new: []string{to}, move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
		return map[string]*yaml.Node{to: v[from]}, nil
	}}
}

// restRename moves a delay of one page, from, to its rest, to, unless it
// is the default of prefetch.rest, which the page then takes from there,
// so that changing prefetch.rest changes the page too.
func restRename(from, to string) rename {
	return rename{old: []string{from}, new: []string{to}, move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
		d, err := time.ParseDuration(v[from].Value)
		if err != nil {
			return nil, errors.New("want a duration such as 150ms")
		}
		if d == Default().Prefetch.Rest {
			return map[string]*yaml.Node{}, nil
		}
		return map[string]*yaml.Node{to: v[from]}, nil
	}}
}

// checkRenames returns what is wrong with table: an old name that is a
// setting still, a new one that isn't, or an old name inside another,
// which a file can't have in its old form at once.
func checkRenames(table []rename, keys []string) []error {
	var errs []error
	var olds []string
	for _, r := range table {
		if len(r.old) == 0 || len(r.new) == 0 {
			errs = append(errs, fmt.Errorf("rename %v → %v: needs an old name and a new one", r.old, r.new))
		}
		for _, o := range r.old {
			if slices.Contains(keys, o) {
				errs = append(errs, fmt.Errorf("%s was renamed, but is still a setting", o))
			}
			olds = append(olds, o)
		}
		for _, n := range r.new {
			if !slices.Contains(keys, n) {
				errs = append(errs, fmt.Errorf("%v was renamed to %s, which is no setting", r.old, n))
			}
		}
	}
	seen := map[string]bool{}
	for _, a := range olds {
		if seen[a] {
			errs = append(errs, fmt.Errorf("the old name %s is listed twice", a))
		}
		seen[a] = true
		for _, b := range olds {
			if strings.HasPrefix(b, a+".") {
				errs = append(errs, fmt.Errorf("the old name %s sits inside %s", b, a))
			}
		}
	}
	return errs
}

// Renamed is a setting that the config file has under its old name, and
// that Load read under its new ones.
type Renamed struct {
	// Old is the name the file has, and Line its line there.
	Old  string
	Line int
	// New are the names it was read as.
	New []string
	// Note says what changed besides the name, or is empty.
	Note string
}

// String says what r was renamed to, and what changed besides, such as
// "details.prefetch.rows → prefetch.pulls.window.after (it now counts the
// rows after the cursor)".
func (r Renamed) String() string {
	s := r.Old + " → " + strings.Join(r.New, ", ")
	if r.Note != "" {
		s += " (" + r.Note + ")"
	}
	return s
}

// RenamedWarning returns what to tell the user once at startup of the
// settings their config file has under old names, or "" if it has none.
// The log lists each of them.
func RenamedWarning(renamed []Renamed) string {
	if len(renamed) == 0 {
		return ""
	}
	if n := len(renamed) - 1; n > 0 {
		return "Your config uses old settings: " + renamed[0].String() + " (and " + strconv.Itoa(n) +
			" more; the log lists them). Rename them in the config file: the next release refuses the old names."
	}
	return "Your config uses an old setting: " + renamed[0].String() + ". Rename it in the config file: the next release refuses the old names."
}

// migrate moves the settings of the user's tree root that table renamed
// to their new names, and returns them, one for each old name the file
// has. A file that sets both an old name and a new one is refused, since
// it would be unclear which the user meant; so is one with an old name
// whose release has passed, with an error that names the new ones.
func migrate(root *yaml.Node, table []rename) ([]Renamed, error) {
	type match struct {
		r      rename
		keys   map[string]*yaml.Node
		vals   map[string]*yaml.Node
		values map[string]*yaml.Node
		err    error
	}
	var (
		found []match
		errs  []error
	)
	for _, r := range table {
		m := match{r: r, keys: map[string]*yaml.Node{}, vals: map[string]*yaml.Node{}}
		for _, o := range r.old {
			if k, v := find(root, o); k != nil && v.Kind != yaml.MappingNode {
				m.keys[o], m.vals[o] = k, v
			}
		}
		if len(m.keys) == 0 {
			continue
		}
		if r.move == nil {
			for _, o := range r.old {
				if k, ok := m.keys[o]; ok {
					errs = append(errs, fmt.Errorf("line %d: %s was renamed to %s", k.Line, o, strings.Join(r.new, ", ")))
				}
			}
		} else if m.values, m.err = r.move(m.vals); m.err == nil {
			// Only the paths that move gives are fed by the file's old
			// names, so only they can clash with a new name it sets.
			for _, p := range r.new {
				if _, ok := m.values[p]; !ok {
					continue
				}
				if nk, _ := find(root, p); nk != nil {
					first := m.keys[r.old[slices.IndexFunc(r.old, func(o string) bool { return m.keys[o] != nil })]]
					errs = append(errs, fmt.Errorf("line %d: %s was renamed to %s, which line %d sets too: set only %s", first.Line, strings.Join(slices.Sorted(maps.Keys(m.keys)), ", "), p, nk.Line, p))
				}
			}
		}
		found = append(found, m)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	var out []Renamed
	for _, m := range found {
		first := m.keys[m.r.old[slices.IndexFunc(m.r.old, func(o string) bool { return m.keys[o] != nil })]]
		values := m.values
		if m.err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", first.Line, strings.Join(slices.Sorted(maps.Keys(m.keys)), ", "), m.err)
		}
		for p, nv := range values {
			if !slices.Contains(m.r.new, p) || nv == nil {
				// A mistake in the table, which its tests are for.
				return nil, fmt.Errorf("rename of %v: move gave %s, which isn't one of its new names or has no value", m.r.old, p)
			}
		}
		for _, o := range m.r.old {
			if k, ok := m.keys[o]; ok {
				remove(root, o)
				out = append(out, Renamed{Old: o, Line: k.Line, New: m.r.new, Note: m.r.note})
			}
		}
		for _, p := range m.r.new {
			if nv, ok := values[p]; ok {
				if err := put(root, p, nv); err != nil {
					return nil, fmt.Errorf("line %d: %s: %w", first.Line, strings.Join(slices.Sorted(maps.Keys(m.keys)), ", "), err)
				}
			}
		}
	}
	return out, nil
}

// find returns the key and the value of the dotted path in the mapping
// root, or nils.
func find(root *yaml.Node, path string) (key, value *yaml.Node) {
	n := root
	for part := range strings.SplitSeq(path, ".") {
		if n.Kind != yaml.MappingNode {
			return nil, nil
		}
		i := mappingIndex(n, part)
		if i < 0 {
			return nil, nil
		}
		key, n = n.Content[i], n.Content[i+1]
	}
	return key, n
}

// remove takes path out of the mapping root, and the mappings that held
// only it, so that a group of settings that was renamed as a whole isn't
// left behind as an unknown empty one.
func remove(root *yaml.Node, path string) {
	parent, last := root, path
	if i := strings.LastIndex(path, "."); i >= 0 {
		_, parent = find(root, path[:i])
		last = path[i+1:]
	}
	if j := mappingIndex(parent, last); j >= 0 {
		parent.Content = append(parent.Content[:j], parent.Content[j+2:]...)
	}
	if parent != root && len(parent.Content) == 0 {
		remove(root, path[:strings.LastIndex(path, ".")])
	}
}

// put sets path in the mapping root to v, making the mappings on the way.
func put(root *yaml.Node, path string, v *yaml.Node) error {
	n := root
	parts := strings.Split(path, ".")
	for i, part := range parts {
		j := mappingIndex(n, part)
		if i == len(parts)-1 {
			if j >= 0 {
				n.Content[j+1] = v
			} else {
				n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part, Line: v.Line}, v)
			}
			return nil
		}
		if j < 0 {
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Line: v.Line}
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part, Line: v.Line}, m)
			n = m
			continue
		}
		if n = n.Content[j+1]; n.Kind != yaml.MappingNode {
			return fmt.Errorf("can't move it to %s: line %d sets %s to a value, not a group of settings", path, n.Line, strings.Join(parts[:i+1], "."))
		}
	}
	return nil
}

// checkKnown returns an error for each key of the user's tree n, at path,
// that no setting of the type t has. It runs after migrate, which has
// moved or refused every old name.
func checkKnown(n *yaml.Node, t reflect.Type, path string) []error {
	var errs []error
	switch {
	case t.Kind() == reflect.Struct && n.Kind == yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			p := join(path, k.Value)
			f, ok := fieldByName(t, k.Value)
			if !ok {
				errs = append(errs, fmt.Errorf("line %d: unknown setting %s", k.Line, p))
				continue
			}
			errs = append(errs, checkKnown(v, f.Type, p)...)
		}
	case t.Kind() == reflect.Map && n.Kind == yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			errs = append(errs, checkKnown(n.Content[i+1], t.Elem(), join(path, n.Content[i].Value))...)
		}
	case t.Kind() == reflect.Slice && n.Kind == yaml.SequenceNode:
		for i, item := range n.Content {
			errs = append(errs, checkKnown(item, t.Elem(), path+"["+strconv.Itoa(i)+"]")...)
		}
	}
	// A value of another shape than its setting's is left to the decoding,
	// which says what it wanted.
	return errs
}

// renamedErrors returns err, an error of Validate, with each setting it
// names that the file has under an old name named by that name too, as
// "sync.interval (now sync.poll.lists): must be at least 10s", since the
// old name is the one the file shows. A setting that several old ones fed
// is named by all of them, such as "a.d, b.d (now x.d)".
func renamedErrors(err error, renamed []Renamed) error {
	if err == nil || len(renamed) == 0 {
		return err
	}
	lines := strings.Split(err.Error(), "\n")
	for i, line := range lines {
	paths:
		for _, r := range renamed {
			for _, p := range r.New {
				rest, ok := strings.CutPrefix(line, p)
				if !ok || !strings.HasPrefix(rest, ":") && !strings.HasPrefix(rest, "[") {
					continue
				}
				var olds []string
				for _, o := range renamed {
					if slices.Contains(o.New, p) {
						olds = append(olds, o.Old)
					}
				}
				lines[i] = strings.Join(olds, ", ") + " (now " + p + ")" + rest
				break paths
			}
		}
	}
	return errors.New(strings.Join(lines, "\n"))
}

// fieldByName returns the field of the struct t that the config file
// names name.
func fieldByName(t reflect.Type, name string) (reflect.StructField, bool) {
	for f := range t.Fields() {
		if yamlName(f) == name {
			return f, true
		}
	}
	return reflect.StructField{}, false
}
