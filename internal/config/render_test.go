package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDefaultFile(t *testing.T) {
	want, err := os.ReadFile("default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if DefaultFile() != string(want) {
		t.Error("DefaultFile isn't default.yaml as it is")
	}
}

// renderFile is a config file that sets a value, a list, one key, one
// colour of a built-in theme and a theme of its own, whose dark palette is
// an alias of its light one.
const renderFile = `theme: mine
ui:
  icons: ascii
keys:
  quit: [x]
history:
  row: [short_sha, subject]
themes:
  default:
    dark:
      accent: "#000000"
  mine:
    light: &p
      accent: "#ff0000"
      foreground: "#111111"
      muted: "#222222"
      subtle: "#333333"
      border: "#444444"
      success: "#555555"
      warning: "#666666"
      error: "#777777"
    dark: *p
`

// loadLayers loads renderFile, raises the log level as --debug does, and
// sets the icons for the session.
func loadLayers(t *testing.T) Layers {
	t.Helper()
	t.Setenv(EnvLog, "")
	cfg, src, err := resolvePath(t, writeConfig(t, renderFile))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Log.Level = LevelDebug
	session, err := cfg.Set("ui.icons", IconsUnicode)
	if err != nil {
		t.Fatal(err)
	}
	return Layers{Source: src, Start: cfg, Session: session}
}

func TestLayersYAML(t *testing.T) {
	l := loadLayers(t)
	got, err := l.YAML()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got, "\n")
	// want are the lines that must follow one another in the output,
	// trimmed, with any lines between them.
	want := []string{
		"theme: mine # config.yaml:1",
		`editor: ""`,
		"ui:",
		"icons: unicode # session (:set)",
		"ttl: 5m",
		"row: [short_sha, subject] # config.yaml:7",
		"level: debug # --debug, GH_DEBUG or GH_TUI_LOG",
		"keys:",
		"quit: [x] # config.yaml:5",
		`help: ["?"]`,
		"themes:",
		"default:",
		"light:",
		`accent: "#3b63c4"`,
		"dark:",
		`accent: "#000000" # config.yaml:11`,
		"mine:",
		"light:",
		`accent: "#ff0000" # config.yaml:14`,
		"dark:",
		`accent: "#ff0000" # config.yaml:14`,
		`error: "#777777" # config.yaml:21`,
	}
	i := 0
	for _, line := range lines {
		if i < len(want) && strings.TrimSpace(line) == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("the config lacks %q after %q:\n%s", want[i], want[max(i-1, 0)], got)
	}
}

// TestLayersYAMLReadsBack checks that the config shown, comments and all,
// reads back as the session's config, so that it can be saved as a config
// file.
func TestLayersYAMLReadsBack(t *testing.T) {
	l := loadLayers(t)
	got, err := l.YAML()
	if err != nil {
		t.Fatal(err)
	}
	dec := yaml.NewDecoder(strings.NewReader(got))
	dec.KnownFields(true)
	var back Config
	if err := dec.Decode(&back); err != nil {
		t.Fatalf("decode: %v\n%s", err, got)
	}
	if !reflect.DeepEqual(back, l.Session) {
		t.Errorf("read back\n%+v\nwant the session's\n%+v", back, l.Session)
	}
}

func TestLayersYAMLIsDeterministic(t *testing.T) {
	l := loadLayers(t)
	first, err := l.YAML()
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if got, _ := l.YAML(); got != first {
			t.Fatalf("YAML changed between calls:\n%s\nthen\n%s", first, got)
		}
	}
}

// TestLayersYAMLMapEntries checks the entries of the maps that a later
// layer has and an earlier one doesn't, which have nothing to compare
// with: a theme and a key only the session has, and with the zero Source,
// as when the app is given none, the start's own as the file's.
func TestLayersYAMLMapEntries(t *testing.T) {
	start := Default()
	session := Default()
	session.Themes = map[string]Theme{"new": start.Themes["default"], "default": start.Themes["default"]}
	session.Keys = map[string][]string{"quit": {"q"}, "zzz": {"z"}}
	got, err := Layers{Start: start, Session: session}.YAML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  new:\n    light:\n      accent: \"#3b63c4\" # session (:set)\n",
		"  quit: [q] # session (:set)\n",
		"  zzz: [z] # session (:set)\n",
		"  default:\n    light:\n      accent: \"#3b63c4\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the config lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "config.yaml") || strings.Contains(got, OriginStartup) {
		t.Errorf("the config names a file or the startup without them:\n%s", got)
	}
}

// TestLayersYAMLEscapes checks that neither a value nor the file's name
// brings an escape sequence or a line break into the output.
func TestLayersYAMLEscapes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "conf\x1b[2J\nig.yaml"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("theme: default\n"), 0o600); err != nil {
		t.Skipf("the file system takes no such name: %v", err)
	}
	t.Setenv(EnvLog, "")
	cfg, src, err := resolvePath(t, path)
	if err != nil {
		t.Fatal(err)
	}
	session := cfg
	session.Editor = "vim \x1b]52;c;aGk=\x07 \u202e"
	got, err := Layers{Source: src, Start: cfg, Session: session}.YAML()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(got, "\x1b\x07\u202e") {
		t.Errorf("the config holds a control character:\n%q", got)
	}
	if !strings.Contains(got, "theme: default # conf ig.yaml:1\n") {
		t.Errorf("the file's name isn't on one line:\n%s", got)
	}
}

func TestResolveSource(t *testing.T) {
	path := writeConfig(t, renderFile)
	t.Setenv(EnvLog, "warn")
	cfg, src, err := resolvePath(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if src.Path() != path || !src.Exists() {
		t.Errorf("Source path %q, exists %v; want %q and true", src.Path(), src.Exists(), path)
	}
	if cfg.Log.Level != LevelWarn || src.file.Log.Level != Default().Log.Level {
		t.Errorf("log level %q, the file's %q; want GH_TUI_LOG's over the file's", cfg.Log.Level, src.file.Log.Level)
	}
	want := map[string]int{"theme": 1, "ui.icons": 3, "keys.quit": 5, "history.row": 7, "themes.default.dark.accent": 11, "themes.mine.light.error": 21, "themes.mine.dark.error": 21}
	origins := src.origins()
	for k, line := range want {
		if origins[k] != (Origin{Line: line}) {
			t.Errorf("origin of %s = %+v, want line %d", k, origins[k], line)
		}
	}
	if _, ok := origins["themes.mine"]; ok {
		t.Error("a mapping has a line of its own; want only its values to")
	}

	missing := filepath.Join(t.TempDir(), "none.yaml")
	if _, src, err := resolvePath(t, missing); err != nil || src.Exists() || len(src.origins()) != 0 || src.Path() != missing {
		t.Errorf("Load of a missing file: %+v, %v", src, err)
	}
}

func BenchmarkLayersYAML(b *testing.B) {
	l := Layers{Start: Default(), Session: Default()}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := l.YAML(); err != nil {
			b.Fatal(err)
		}
	}
}

// TestLoadSourceRenamed checks that the lines of renamed settings are kept
// under their new names: the line of the old name, also where the move
// made a node of its own, which has none.
func TestResolveSourceRenamed(t *testing.T) {
	withRenames(t, testRenames)
	t.Setenv(EnvLog, "")
	_, src, err := resolvePath(t, writeConfig(t, "details:\n  prefetch:\n    count: 6\nsync:\n  every: 2m\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Origin{"details.prefetch.rows": {Line: 3}, "history.prefetch.around": {Line: 3}, "sync.poll.lists": {Line: 5}}
	if got := src.origins(); !reflect.DeepEqual(got, want) {
		t.Errorf("origins = %v, want %v", got, want)
	}
	if got := src.Renamed(); len(got) != 2 {
		t.Errorf("Renamed = %v, want both old names", got)
	}
}

// TestLayersYAMLFileLines checks the comments of values that a file sets
// to the default, and through a merge key, which carries the lines of the
// mapping it brings.
func TestLayersYAMLFileLines(t *testing.T) {
	t.Setenv(EnvLog, "")
	// 1m is sync.poll.lists's default.
	cfg, src, err := resolvePath(t, writeConfig(t, "sync:\n  poll:\n    lists: 1m\nui:\n  <<: {icons: ascii}\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Layers{Source: src, Start: cfg, Session: cfg}.YAML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"    lists: 1m # config.yaml:3\n", "  icons: ascii # config.yaml:5\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("the config lacks %q:\n%s", want, got)
		}
	}
}

// TestLayersYAMLKeysStayDistinct checks that a key with a character the
// terminal wouldn't show, such as a bidi override, shows a placeholder
// for it, rather than looking like the key without it.
func TestLayersYAMLKeysStayDistinct(t *testing.T) {
	session := Default()
	session.Keys["quit\u202e"] = []string{"z"}
	session.Keys["quit\u200b"] = []string{"y"}
	got, err := Layers{Start: Default(), Session: session}.YAML()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  quit: [q, ctrl+c]\n", "  quit\ufffd: [y] # session (:set)\n", "  quit\ufffd: [z] # session (:set)\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("the config lacks %q:\n%s", want, got)
		}
	}
	if strings.ContainsAny(got, "\u202e\u200b") {
		t.Errorf("the config holds an invisible character:\n%q", got)
	}
}

// resolvePath loads the config file at path and resolves it for a session
// on github.com with no account.
func resolvePath(t *testing.T, path string) (Config, Source, error) {
	t.Helper()
	f, err := Load(path)
	if err != nil {
		return Config{}, Source{}, err
	}
	return f.Resolve("github.com", "")
}

// TestLayersYAMLHostsAndProfiles checks that a value set under hosts or
// profiles names its entry, and that a later layer's line wins.
func TestLayersYAMLHostsAndProfiles(t *testing.T) {
	t.Setenv(EnvLog, "")
	f, err := Load(writeConfig(t, `sync:
  poll:
    lists: 2m
dashboard:
  contributions: 90d
hosts:
  ghe.corp.com:
    sync:
      poll:
        lists: 3m
profiles:
  work:
    accounts: [mona@ghe.corp.com]
    sync:
      poll:
        lists: 4m
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		host, login string
		want        string
	}{
		{"github.com", "mona", "    lists: 2m # config.yaml:3\n"},
		{"ghe.corp.com", "", "    lists: 3m # config.yaml:10 (hosts.ghe.corp.com)\n"},
		{"ghe.corp.com", "mona", "    lists: 4m # config.yaml:16 (profiles.work)\n"},
	} {
		cfg, src, err := f.Resolve(tt.host, tt.login)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Layers{Source: src, Start: cfg, Session: cfg}.YAML()
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{tt.want, "  contributions: 90d # config.yaml:5\n"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s as %q: the config lacks %q:\n%s", tt.host, tt.login, want, got)
			}
		}
	}
}
