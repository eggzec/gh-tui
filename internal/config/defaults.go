package config

import (
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"sync"

	"go.yaml.in/yaml/v3"
)

// defaultYAML is default.yaml, which holds the default of every setting
// with a comment on what it does. It is the only place a default is
// written down: Go holds the types, the bounds and the validation.
//
//go:embed default.yaml
var defaultYAML []byte

// defaultTree is defaultYAML parsed, which every call of Default decodes
// and every Load merges the user's file over. Nothing changes it.
var defaultTree = sync.OnceValue(func() *yaml.Node {
	root, err := parseYAML(defaultYAML)
	if err != nil || root == nil {
		// The tests decode and validate default.yaml, so this can only be
		// a build of a broken file.
		panic(fmt.Sprintf("config: default.yaml: %v", err))
	}
	return root
})

// defaults is defaultTree decoded, which every call of Default copies.
// Nothing changes it.
var defaults = sync.OnceValue(func() Config {
	cfg, err := decode(defaultTree())
	if err != nil {
		panic(fmt.Sprintf("config: default.yaml: %v", err))
	}
	return cfg
})

// Default returns the default configuration, as default.yaml sets it. It
// is what the constructors of the app start from before they are given
// the settings, so none of them holds a default of its own. Each call
// returns fresh maps and slices, so callers may modify the result.
func Default() Config {
	return defaults().clone()
}

// clone returns c with maps and slices of its own.
func (c Config) clone() Config {
	c.Repos = slices.Clone(c.Repos)
	c.Themes = maps.Clone(c.Themes)
	c.Keys = maps.Clone(c.Keys)
	for action, keys := range c.Keys {
		c.Keys[action] = slices.Clone(keys)
	}
	c.History.Row = slices.Clone(c.History.Row)
	c.History.Detail = slices.Clone(c.History.Detail)
	clonePointers(reflect.ValueOf(&c.Prefetch).Elem())
	return c
}

// clonePointers points each pointer in v, an addressable struct, however
// deep, at a copy of its own of what it points at.
func clonePointers(v reflect.Value) {
	for _, f := range v.Fields() {
		switch f.Kind() {
		case reflect.Struct:
			clonePointers(f)
		case reflect.Pointer:
			if !f.IsNil() {
				p := reflect.New(f.Type().Elem())
				p.Elem().Set(f.Elem())
				f.Set(p)
			}
		default:
		}
	}
}

// parseYAML returns the root node of the document in data, or nil when
// data holds none, such as an empty file or one of comments only.
func parseYAML(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if isNull(root) {
		return nil, nil
	}
	return root, nil
}

// decode returns the Config that root spells.
func decode(root *yaml.Node) (Config, error) {
	var cfg Config
	if err := root.Decode(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// merge returns over laid on base, leaving both as they were: a mapping
// merges key by key, and anything else, a list too, replaces what base
// has. Lists replace because their order matters, as in history.row, and
// because appending could never take an item out. over must be plain.
func merge(base, over *yaml.Node) *yaml.Node {
	if base == nil || base.Kind != yaml.MappingNode || over.Kind != yaml.MappingNode {
		return over
	}
	out := *base
	out.Content = append([]*yaml.Node(nil), base.Content...)
	out.Line, out.Column = over.Line, over.Column
	for i := 0; i < len(over.Content); i += 2 {
		k, v := over.Content[i], over.Content[i+1]
		if j := mappingIndex(&out, k.Value); j >= 0 {
			out.Content[j+1] = merge(out.Content[j+1], v)
			continue
		}
		out.Content = append(out.Content, k, v)
	}
	return &out
}

// mappingIndex returns the index of key in the content of the mapping m,
// or -1.
func mappingIndex(m *yaml.Node, key string) int {
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// plain returns a copy of the user's tree n, at path, that merge can lay
// over the defaults key by key: aliases are replaced by what they name,
// merge keys (<<) by the keys they bring, and every empty value is
// refused. An empty value would be a third state beside a value and no
// line at all, and it is easy to write by accident, as "editor:" with
// nothing after it. The one exception is a knob that may take its value
// from the layer above (Inherits), where null hands it back, over what
// default.yaml sets for it.
func plain(n *yaml.Node, path string) (*yaml.Node, error) {
	return plainAt(n, path, "")
}

// names stands for the setting path of the entries of hosts and of
// profiles, whose keys are names, such as a host's, rather than settings.
const names = "\x00names"

// plainAt is plain of n at path in the file, which is the setting at
// setting: the same path, but inside an entry of hosts or profiles, where
// it starts again at the entry. Host names hold dots, so it is kept
// apart as the tree is walked rather than cut from path.
func plainAt(n *yaml.Node, path, setting string) (*yaml.Node, error) {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	if isNull(n) && !Inherits(setting) {
		return nil, fmt.Errorf("line %d: %s is empty: remove the line to keep the default", n.Line, name(path))
	}
	out := *n
	switch n.Kind {
	case yaml.SequenceNode:
		out.Content = make([]*yaml.Node, len(n.Content))
		var errs []error
		for i, item := range n.Content {
			var err error
			if out.Content[i], err = plainAt(item, path+"["+strconv.Itoa(i)+"]", setting+"["+strconv.Itoa(i)+"]"); err != nil {
				errs = append(errs, err)
			}
		}
		return &out, errors.Join(errs...)
	case yaml.MappingNode:
		return plainMapping(n, path, setting)
	case yaml.DocumentNode, yaml.ScalarNode, yaml.AliasNode:
		// A scalar is taken as it is; the others were resolved above, or
		// by parseYAML.
	}
	return &out, nil
}

// plainMapping is plain of a mapping: its own keys, then those that its
// merge keys bring and it doesn't set itself, as YAML reads them.
func plainMapping(n *yaml.Node, path, setting string) (*yaml.Node, error) {
	out := *n
	out.Content = nil
	var merged []*yaml.Node
	var errs []error
	for i := 0; i < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.ShortTag() == "!!merge" {
			merged = append(merged, v)
			continue
		}
		if j := mappingIndex(&out, k.Value); j >= 0 {
			errs = append(errs, fmt.Errorf("line %d: %s is set twice, here and at line %d", k.Line, join(path, k.Value), out.Content[j].Line))
			continue
		}
		child := join(setting, k.Value)
		switch {
		case path == "" && (k.Value == hostsKey || k.Value == profilesKey):
			child = names
		case setting == names:
			// An entry's settings, under its name.
			child = ""
		}
		pv, err := plainAt(v, join(path, k.Value), child)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out.Content = append(out.Content, k, pv)
	}
	for _, m := range merged {
		pm, err := plainAt(m, path, setting)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// A merge key takes a mapping or a list of them, and the first to
		// set a key wins.
		sources := []*yaml.Node{pm}
		if pm.Kind == yaml.SequenceNode {
			sources = pm.Content
		}
		for _, s := range sources {
			if s.Kind != yaml.MappingNode {
				errs = append(errs, fmt.Errorf("line %d: %s: a merge key (<<) takes a mapping or a list of them", s.Line, name(path)))
				continue
			}
			for i := 0; i < len(s.Content); i += 2 {
				if mappingIndex(&out, s.Content[i].Value) < 0 {
					out.Content = append(out.Content, s.Content[i], s.Content[i+1])
				}
			}
		}
	}
	return &out, errors.Join(errs...)
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

// join returns the path of key within the mapping at path.
func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// name returns path as an error names it.
func name(path string) string {
	if path == "" {
		return "the file"
	}
	return path
}
