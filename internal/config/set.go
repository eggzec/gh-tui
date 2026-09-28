package config

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// Keys returns the keys that [Config.Set] takes, in the order of the
// fields of Config: the path of each setting that holds a value or a list,
// such as "ui.icons". The maps, themes and keys, have keys of their own,
// and aren't among them.
func Keys() []string {
	return slices.Clone(settingKeys())
}

var settingKeys = sync.OnceValue(func() []string {
	var out []string
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for f := range t.Fields() {
			name := prefix + yamlName(f)
			switch {
			case f.Type.Kind() == reflect.Map:
			case f.Type.Kind() == reflect.Struct:
				walk(f.Type, name+".")
			default:
				out = append(out, name)
			}
		}
	}
	walk(reflect.TypeFor[Config](), "")
	return out
})

// yamlName returns the name of f in the config file.
func yamlName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
	return name
}

// ErrUnknownKey is matched by the error of a key that no setting has.
var ErrUnknownKey = errors.New("unknown setting")

// setting returns the field of v, a Config, that key names.
func setting(v reflect.Value, key string) (reflect.Value, error) {
	if !slices.Contains(settingKeys(), key) {
		return reflect.Value{}, fmt.Errorf("%w %q", ErrUnknownKey, key)
	}
	for part := range strings.SplitSeq(key, ".") {
		for f := range v.Type().Fields() {
			if yamlName(f) == part {
				v = v.FieldByIndex(f.Index)
				break
			}
		}
	}
	return v, nil
}

// Get returns the value of the setting key in c, spelled as the config
// file may spell it.
func (c Config) Get(key string) (string, error) {
	v, err := setting(reflect.ValueOf(c), key)
	if err != nil {
		return "", err
	}
	return format(v), nil
}

// format spells v as the config file may.
func format(v reflect.Value) string {
	switch x := v.Interface().(type) {
	case time.Duration:
		return formatDuration(x)
	case fmt.Stringer:
		return x.String()
	}
	switch v.Kind() {
	case reflect.String:
		if v.String() == "" {
			return `""`
		}
		return v.String()
	case reflect.Slice:
		items := make([]string, v.Len())
		for i := range v.Len() {
			items[i] = format(v.Index(i))
		}
		return "[" + strings.Join(items, ", ") + "]"
	default:
		return fmt.Sprint(v.Interface())
	}
}

// formatDuration spells d without the zero units at its end, such as 1m
// rather than 1m0s.
func formatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// Set returns c with the setting key set to value, read as the config file
// reads it and validated as [Load] validates the file. A string is taken
// as it is, without quotes around it if it has them, and a list may leave
// out its brackets: "short_sha, subject". c is left as it was, and nothing
// is written anywhere.
func (c Config) Set(key, value string) (Config, error) {
	out := c
	v, err := setting(reflect.ValueOf(&out).Elem(), key)
	if err != nil {
		return c, err
	}
	nv := reflect.New(v.Type())
	switch {
	case v.Kind() == reflect.String:
		nv.Elem().SetString(unquote(value))
	default:
		if v.Kind() == reflect.Slice && !strings.HasPrefix(strings.TrimSpace(value), "[") {
			value = "[" + value + "]"
		}
		if err := yaml.Unmarshal([]byte(value), nv.Interface()); err != nil {
			return c, fmt.Errorf("%s: can't read %q: %w", key, value, readError(err))
		}
	}
	v.Set(nv.Elem())
	if err := out.Validate(); err != nil {
		return c, err
	}
	return out, nil
}

// unquote returns s without the quotes around it, if it has them, as
// YAML would read a quoted string.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		if u, err := strconv.Unquote(`"` + s[1:len(s)-1] + `"`); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}

// readError returns what YAML says of a value it can't read, without
// the line number, which the value alone has no use for.
func readError(err error) error {
	var te *yaml.TypeError
	if errors.As(err, &te) && len(te.Errors) > 0 {
		msg := te.Errors[0]
		if _, after, ok := strings.Cut(msg, ": "); ok && strings.HasPrefix(msg, "line ") {
			msg = after
		}
		return errors.New(msg)
	}
	return err
}

// Values returns the values that the setting key takes, for completion:
// those it chooses from, true and false for a switch, or none for text,
// numbers and lists.
func (c Config) Values(key string) []string {
	if key == "theme" {
		return slices.Sorted(maps.Keys(c.Themes))
	}
	if vs, ok := choices[key]; ok {
		return slices.Clone(vs)
	}
	if v, err := setting(reflect.ValueOf(c), key); err == nil && v.Kind() == reflect.Bool {
		return []string{"true", "false"}
	}
	return nil
}

// choices are the values of the settings that choose one of a few.
var choices = map[string][]string{
	"ui.icons":                     {IconsNerd, IconsUnicode, IconsASCII},
	"dashboard.contributions":      {Contributions30d, Contributions90d, ContributionsYear},
	"history.date_format":          {DateRelative, DateAbsolute},
	"cache.revalidate.scope":       {ScopeRecent, ScopeAll},
	"cache.disk.compression":       {CompressionGzip, CompressionNone},
	"cache.disk.compression_level": {LevelFastest, LevelDefault, LevelBest},
	"log.level":                    {LevelDebug, LevelInfo, LevelWarn, LevelError},
}
