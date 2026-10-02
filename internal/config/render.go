package config

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// DefaultFile returns default.yaml as it is embedded, with its comments:
// every setting and its default.
func DefaultFile() string {
	return string(defaultYAML)
}

// fileLines records in out where the user's plain tree n, the layer
// named layer, at path, sets each value: the line of its key, and a list
// as one value, over what an earlier layer recorded. A value reached
// through an alias or a merge key (<<) carries the lines of the anchor it
// came from, where the file wrote it. A value that a rename moved has the
// line of its old name, or, where the move made a node of its own, which
// has no line, that of the old name in renamed; a value with neither has
// no line, and keeps an earlier layer's, if any.
func fileLines(n *yaml.Node, path, layer string, out map[string]Origin, renamed []Renamed) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		p := join(path, k.Value)
		if v.Kind == yaml.MappingNode {
			fileLines(v, p, layer, out, renamed)
			continue
		}
		line := k.Line
		if line == 0 {
			line = v.Line
		}
		if line == 0 {
			if j := slices.IndexFunc(renamed, func(r Renamed) bool { return slices.Contains(r.New, p) }); j >= 0 {
				line = renamed[j].Line
			}
		}
		if line > 0 {
			out[p] = Origin{Layer: layer, Line: line}
		}
	}
}

// Origins of a value, in the comment after it, besides the config file's
// name and line.
const (
	// OriginSession marks a value that :set changed.
	OriginSession = "session (:set)"
	// OriginStartup marks a value that a flag or the environment raised
	// at startup: only the log level.
	OriginStartup = "--debug, GH_DEBUG or GH_TUI_LOG"
)

// Layers are what a session's config is made of, each over the one before:
// default.yaml, the config file, what the flags and the environment raised
// at startup, and what :set changed.
type Layers struct {
	// Source says which layers of the config file the session resolved.
	Source Source
	// Start is the config the session started with.
	Start Config
	// Session is the config as :set left it.
	Session Config
}

// YAML returns the session's config as a config file would spell it:
// every setting, every key and every theme, in the order of default.yaml,
// and the keys of a map that default.yaml lacks after its own, sorted. A
// value that a layer over default.yaml set is followed by a comment that
// says which: the config file and its line, such as "config.yaml:12",
// with the layer of hosts or profiles it is in, if any, as in
// "config.yaml:30 (hosts.ghe.corp.com)", or OriginStartup or
// OriginSession. A list replaces a list, so its comment is that of the
// whole list.
func (l Layers) YAML() (string, error) {
	file := l.Start
	if l.Source.f != nil {
		file = l.Source.file
	}
	r := render{lines: l.Source.origins(), name: termtext.OneLine(filepath.Base(l.Source.Path()))}
	root := r.node("", defaultTree(), reflect.ValueOf(l.Session), reflect.ValueOf(l.Start), reflect.ValueOf(file))
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", fmt.Errorf("spell the config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("spell the config: %w", err)
	}
	return b.String(), nil
}

// render spells the layers of a config as YAML with origin comments.
type render struct {
	lines map[string]Origin
	// name is the config file's, for the comments.
	name string
}

// node returns the node of s, the session's value at path, and of what it
// holds. start and file are the values at path that the session started
// with and that the file made, each the zero Value where its layer has
// none, such as a theme only a later layer has. def is the node of
// default.yaml at path, or nil, whose keys give a mapping its order.
func (r render) node(path string, def *yaml.Node, s, start, file reflect.Value) *yaml.Node {
	switch s.Kind() {
	case reflect.Pointer:
		// A knob that may take its value from the layer above, set.
		return r.node(path, def, s.Elem(), elem(start), elem(file))
	case reflect.Struct:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		fields := slices.Collect(s.Type().Fields())
		names := make([]string, len(fields))
		for i, f := range fields {
			names[i] = yamlName(f)
		}
		for _, i := range inOrder(def, names, false) {
			name := names[i]
			v := s.Field(i)
			if v.Kind() == reflect.Pointer && v.IsNil() {
				// Left to the layer above, as the table of prefetch shows.
				continue
			}
			c := r.node(join(path, name), child(def, name), v, field(start, i), field(file, i))
			if c.Kind == yaml.MappingNode && len(c.Content) == 0 {
				// A group of such knobs, none of them set.
				continue
			}
			n.Content = append(n.Content, str(name), c)
		}
		return n
	case reflect.Map:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := s.MapKeys()
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = k.String()
		}
		for _, i := range inOrder(def, names, true) {
			name, k := names[i], keys[i]
			n.Content = append(n.Content, str(visible(name)), r.node(join(path, name), child(def, name), s.MapIndex(k), entry(start, k), entry(file, k)))
		}
		return n
	default:
		n := leaf(s)
		n.LineComment = r.origin(path, s, start, file)
		return n
	}
}

// inOrder returns the indexes of names in the order that the mapping def
// has them, and then those it hasn't: sorted, if sorted is set, as the
// keys of a map, or else in their order, as the fields of a struct.
func inOrder(def *yaml.Node, names []string, sorted bool) []int {
	at := func(name string) int {
		if def == nil || def.Kind != yaml.MappingNode {
			return -1
		}
		return mappingIndex(def, name)
	}
	order := make([]int, len(names))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		ia, ib := at(names[a]), at(names[b])
		switch {
		case ia >= 0 && ib >= 0:
			return ia - ib
		case ia >= 0:
			return -1
		case ib >= 0:
			return 1
		case sorted:
			return strings.Compare(names[a], names[b])
		}
		return a - b
	})
	return order
}

// child returns the value of key in the mapping n, or nil.
func child(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	if i := mappingIndex(n, key); i >= 0 {
		return n.Content[i+1]
	}
	return nil
}

// origin returns the comment of the value s at path, or "" for a default.
func (r render) origin(path string, s, start, file reflect.Value) string {
	switch {
	case !start.IsValid() || format(s) != format(start):
		return "# " + OriginSession
	case !file.IsValid() || format(start) != format(file):
		return "# " + OriginStartup
	}
	o, ok := r.lines[path]
	if !ok {
		return ""
	}
	comment := "# " + r.name + ":" + strconv.Itoa(o.Line)
	if o.Layer != "" {
		comment += " (" + termtext.OneLine(o.Layer) + ")"
	}
	return comment
}

// field returns field i of the struct v, or the zero Value if v is.
func field(v reflect.Value, i int) reflect.Value {
	if !v.IsValid() {
		return reflect.Value{}
	}
	return v.Field(i)
}

// elem returns what the pointer v points at, or the zero Value where v is
// the zero Value or nil.
func elem(v reflect.Value) reflect.Value {
	if !v.IsValid() || v.IsNil() {
		return reflect.Value{}
	}
	return v.Elem()
}

// entry returns the value of k in the map v, or the zero Value where v
// is the zero Value or has no k.
func entry(v, k reflect.Value) reflect.Value {
	if !v.IsValid() {
		return reflect.Value{}
	}
	return v.MapIndex(k)
}

// leaf returns the node of v, a setting that holds a value or a list,
// spelled as the config file spells it.
func leaf(v reflect.Value) *yaml.Node {
	switch x := v.Interface().(type) {
	case time.Duration:
		return str(formatDuration(x))
	case fmt.Stringer:
		return str(x.String())
	}
	switch v.Kind() {
	case reflect.Bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v.Bool())}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(v.Int(), 10)}
	case reflect.Slice:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
		for i := range v.Len() {
			n.Content = append(n.Content, leaf(v.Index(i)))
		}
		return n
	default:
		return str(v.String())
	}
}

// visible returns the key of a map entry, such as a theme's name, with a
// U+FFFD in place of each character that the terminal wouldn't show as it
// is, as the pager shows them: a control, an invisible or bidi character,
// or a byte that isn't UTF-8. str would drop them, and so make the key of
// one entry look like another's.
func visible(s string) string {
	return strings.Map(func(r rune) rune {
		if termtext.Control(r) || termtext.Hidden(r) {
			return utf8.RuneError
		}
		return r
	}, strings.ToValidUTF8(s, string(utf8.RuneError)))
}

// str returns the node of the string s, in double quotes where it needs
// quotes, as default.yaml quotes its colours. It is for the terminal, so
// s comes without what termtext.OneLine drops, such as a bidi override,
// which YAML would leave as it is.
func str(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: termtext.OneLine(s)}
	if out, err := yaml.Marshal(n); err == nil && len(out) > 0 && out[0] == '\'' {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}
