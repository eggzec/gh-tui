package config

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// TestDefaultsComplete checks that default.yaml is the whole of the
// defaults: it decodes strictly and validates, it sets every setting, so
// that none is left to Go's zero value, and it gives keys to exactly the
// actions that keys.go names.
func TestDefaultsComplete(t *testing.T) {
	dec := yaml.NewDecoder(bytes.NewReader(defaultYAML))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("decode default.yaml: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("default.yaml doesn't validate:\n%v", err)
	}

	root := defaultTree()
	for _, key := range Keys() {
		// A knob of a page or a kind is set only where it differs.
		if Inherits(key) {
			continue
		}
		if lookup(root, key) == nil {
			t.Errorf("default.yaml doesn't set %s", key)
		}
	}

	consts := actionConsts(t)
	for _, action := range slices.Sorted(maps.Keys(cfg.Keys)) {
		if !slices.Contains(consts, action) {
			t.Errorf("default.yaml gives keys to %s, which keys.go has no Action constant for", action)
		}
	}
	for _, action := range consts {
		if _, ok := cfg.Keys[action]; !ok {
			t.Errorf("default.yaml gives no keys to %s, which keys.go names", action)
		}
	}
}

// lookup returns the node at the dotted path in the mapping n, or nil.
func lookup(n *yaml.Node, path string) *yaml.Node {
	for part := range strings.SplitSeq(path, ".") {
		if n.Kind != yaml.MappingNode {
			return nil
		}
		i := mappingIndex(n, part)
		if i < 0 {
			return nil
		}
		n = n.Content[i+1]
	}
	return n
}

// actionConsts returns the values of the Action constants of keys.go.
func actionConsts(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "keys.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, id := range vs.Names {
			lit, ok := vs.Values[i].(*ast.BasicLit)
			if !strings.HasPrefix(id.Name, "Action") || !ok || lit.Kind != token.STRING {
				continue
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, v)
		}
		return false
	})
	return out
}

func TestLoadMerges(t *testing.T) {
	tests := []struct {
		name, file string
		want       func(*Config)
	}{
		{
			name: "a mapping merges key by key",
			file: "cache:\n  disk:\n    max_size: 1GiB\n",
			want: func(c *Config) { c.Cache.Disk.MaxSize = GiB },
		},
		{
			name: "a list replaces the default",
			file: "history:\n  row: [age]\n",
			want: func(c *Config) { c.History.Row = []string{FieldAge} },
		},
		{
			name: "an empty list replaces the default",
			file: "history:\n  detail: []\n",
			want: func(c *Config) { c.History.Detail = []string{} },
		},
		{
			name: "a theme merges colour by colour",
			file: "themes:\n  default:\n    dark:\n      accent: \"#ffffff\"\n",
			want: func(c *Config) {
				th := c.Themes["default"]
				th.Dark.Accent = "#ffffff"
				c.Themes["default"] = th
			},
		},
		{
			name: "aliases read as what they name",
			file: "github:\n  timeout: &d 2m\nsync:\n  poll:\n    lists: *d\n",
			want: func(c *Config) {
				c.GitHub.Timeout = 2 * time.Minute
				c.Sync.Poll.Lists = 2 * time.Minute
			},
		},
		{
			name: "a merge key brings what the mapping doesn't set",
			file: "details:\n  prefetch:\n    <<: {enabled: false, rows: 1}\n    rows: 2\n",
			want: func(c *Config) { c.Details.Prefetch.Enabled, c.Details.Prefetch.Rows = false, 2 },
		},
		{
			name: "a file of comments only",
			file: "# nothing yet\n",
			want: func(*Config) {},
		},
		{
			name: "an empty document",
			file: "---\n",
			want: func(*Config) {},
		},
	}
	t.Setenv(EnvLog, "")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := loadBase(writeConfig(t, tt.file))
			if err != nil {
				t.Fatalf("Load error = %v", err)
			}
			want := Default()
			tt.want(&want)
			assertEqual(t, got, want)
		})
	}
}

func TestLoadRefusesEmptyValues(t *testing.T) {
	tests := []struct {
		file string
		want []string
	}{
		{"editor:\n", []string{"line 1: editor is empty: remove the line to keep the default"}},
		{"cache:\n  disk:\n    dir: ~\n", []string{"line 3: cache.disk.dir is empty"}},
		{"cache:\n", []string{"line 1: cache is empty"}},
		{"repos: [a/b, null]\n", []string{"line 1: repos[1] is empty"}},
		{"theme:\nlog:\n  file:\n", []string{"line 1: theme is empty", "line 3: log.file is empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			_, _, err := loadBase(writeConfig(t, tt.file))
			if err == nil {
				t.Fatal("Load error = nil, want an error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("Load error = %q, want it to contain %q", err, w)
				}
			}
		})
	}
}

// TestLoadValidatesMerged checks that validation sees the merged config:
// a theme of the user's own needs every colour, which the default theme
// doesn't lend it.
func TestLoadValidatesMerged(t *testing.T) {
	_, _, err := loadBase(writeConfig(t, "theme: mine\nthemes:\n  mine:\n    dark:\n      accent: \"#ffffff\"\n"))
	if err == nil || !strings.Contains(err.Error(), "themes.mine.light.accent") {
		t.Errorf("Load error = %v, want the colours mine lacks", err)
	}
}

func TestMergeLeavesItsInputs(t *testing.T) {
	before := Default()
	if _, _, err := loadBase(writeConfig(t, "cache:\n  ttl: 1m\nkeys:\n  quit: [x]\n")); err != nil {
		t.Fatal(err)
	}
	assertEqual(t, Default(), before)
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func BenchmarkDefault(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		Default()
	}
}
