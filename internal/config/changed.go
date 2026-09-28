package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Redacted stands for the value of a setting that Changed leaves out.
const Redacted = "(redacted)"

// Setting is one setting of a Config: its key, the YAML keys that lead
// to it joined by dots, such as sync.interval, and its value.
type Setting struct {
	Key, Value string
}

// Changed returns the settings of c that differ from the defaults, sorted
// by key, so that a log can say how a session was set up. A value that
// may name a path, a command, a repository or a secret reads Redacted.
func (c Config) Changed() []Setting {
	have, want := flatten(c), flatten(Default())
	var out []Setting
	for _, key := range slices.Sorted(maps.Keys(have)) {
		v := have[key]
		if d, ok := want[key]; ok && d == v {
			continue
		}
		if private(key) {
			v = Redacted
		}
		out = append(out, Setting{Key: key, Value: v})
	}
	return out
}

// privateKeys are the settings whose values name the user's paths,
// commands or repositories.
var privateKeys = map[string]bool{
	"repos":          true,
	"editor":         true,
	"log.file":       true,
	"cache.disk.dir": true,
}

// privateWords, in the last key of a setting, mark a value that may be
// secret or name the user's files, in settings to come too.
var privateWords = []string{"token", "secret", "password", "auth", "credential", "path", "dir", "file", "command", "url"}

// private reports whether the value of the setting key may say what isn't
// for a log. The names of actions and themes are the user's words, and
// their values keys and colors, so they are never private.
func private(key string) bool {
	if privateKeys[key] {
		return true
	}
	if strings.HasPrefix(key, "keys.") || strings.HasPrefix(key, "themes.") {
		return false
	}
	last := strings.ToLower(key[strings.LastIndexByte(key, '.')+1:])
	return slices.ContainsFunc(privateWords, func(w string) bool { return strings.Contains(last, w) })
}

// flatten returns the settings of c by key, each value as YAML writes it,
// and a list as its items in brackets.
func flatten(c Config) map[string]string {
	out := map[string]string{}
	b, err := yaml.Marshal(c)
	if err != nil {
		return out
	}
	var m map[string]any
	if yaml.Unmarshal(b, &m) != nil {
		return out
	}
	flattenInto(out, "", m)
	return out
}

func flattenInto(out map[string]string, prefix string, v any) {
	m, ok := v.(map[string]any)
	if !ok {
		out[prefix] = fmt.Sprint(v)
		return
	}
	for k, sub := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		flattenInto(out, key, sub)
	}
}
