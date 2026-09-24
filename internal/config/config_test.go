package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Errorf("Default().Validate() = %v, want nil", err)
	}
}

func TestDefaultReturnsFreshMaps(t *testing.T) {
	a := Default()
	a.Keys[ActionQuit] = []string{"x"}
	if got := Default().Keys[ActionQuit]; slices.Contains(got, "x") {
		t.Errorf("Default() shares its keymap between calls: quit = %v", got)
	}
}

func TestLoadDefaults(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(t.TempDir(), "config.yaml")},
		{"empty file", "testdata/empty.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(tt.path)
			if err != nil {
				t.Fatalf("Load(%q) error = %v", tt.path, err)
			}
			assertEqual(t, got, Default())
		})
	}
}

func TestLoadMergesOverDefaults(t *testing.T) {
	tests := []struct {
		file string
		want func(*Config)
	}{
		{
			file: "partial.yaml",
			want: func(c *Config) {
				c.Keys[ActionQuit] = []string{"x"}
				c.Cache.TTL = 10 * time.Minute
			},
		},
		{
			file: "full.yaml",
			want: func(c *Config) {
				c.Repos = []string{"eggzec/gh-tui", "cli/cli"}
				c.Theme = "dusk"
				c.Themes["dusk"] = Theme{
					Light: Palette{
						Accent: "#8a4fbf", Foreground: "#222222", Muted: "#555555", Subtle: "#888888",
						Border: "#dddddd", Success: "#2f7d4f", Warning: "#9a6700", Error: "#c0392b",
					},
					Dark: Palette{
						Accent: "#c39bf0", Foreground: "#e0e0e0", Muted: "#aaaaaa", Subtle: "#777777",
						Border: "#444", Success: "#7fc99a", Warning: "#e5c07b", Error: "#ef7d7d",
					},
				}
				c.Keys[ActionQuit] = []string{"x"}
				c.Keys[ActionSearch] = []string{"/", "ctrl+f"}
				c.Cache.TTL = 30 * time.Second
				c.Sync = Sync{Enabled: false, Interval: 2 * time.Minute}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, err := Load(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("Load error = %v", err)
			}
			want := Default()
			tt.want(&want)
			assertEqual(t, got, want)
		})
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		file string
		want []string
	}{
		{"unknown_field.yaml", []string{"line 3", "field size not found"}},
		{"malformed.yaml", []string{"malformed.yaml", "line 3"}},
		{"invalid.yaml", []string{"repos[0]", "theme:", "keys.quit", "cache.ttl"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			_, err := Load(filepath.Join("testdata", tt.file))
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

func TestValidateReportsEveryProblem(t *testing.T) {
	cfg := Default()
	cfg.Repos = []string{"eggzec/gh-tui", "nope", "a/b/c"}
	cfg.Theme = "missing"
	cfg.Themes["bad"] = Theme{Light: builtinThemes[DefaultTheme].Light}
	cfg.Keys[ActionHelp] = nil
	cfg.Keys[ActionSearch] = []string{""}
	cfg.Keys["jump"] = []string{"j"}
	cfg.Cache.TTL = 0
	cfg.Sync.Interval = -time.Second

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want errors")
	}
	if _, ok := err.(interface{ Unwrap() []error }); !ok {
		t.Fatalf("Validate() = %T, want a joined error", err)
	}
	got := strings.Split(err.Error(), "\n")
	want := []string{
		`repos[1]: invalid repo "nope": want owner/name`,
		`repos[2]: invalid repo "a/b/c": want owner/name`,
		`theme: unknown theme "missing"`,
		`themes.bad.dark.accent: want a quoted hex color like "#7aa2f7", got ""`,
		`keys.help: needs at least one key`,
		`keys.jump: unknown action`,
		`keys.search: empty key`,
		`cache.ttl: must be positive, got 0s`,
		`sync.interval: must be positive, got -1s`,
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("Validate() is missing %q; got:\n%s", w, strings.Join(got, "\n"))
		}
	}
	// Each of the 8 dark colors of "bad" is empty.
	if n := len(got); n != len(want)+7 {
		t.Errorf("Validate() reported %d problems, want %d", n, len(want)+7)
	}
}

func TestPath(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "ios", "windows", "plan9":
		t.Skip("os.UserConfigDir ignores XDG_CONFIG_HOME on " + runtime.GOOS)
	}
	tests := []struct {
		name, env, xdg, want string
	}{
		{name: "env override", env: "/etc/gh-tui.yaml", xdg: "/xdg", want: "/etc/gh-tui.yaml"},
		{name: "xdg", xdg: "/xdg", want: "/xdg/gh-tui/config.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvPath, tt.env)
			t.Setenv("XDG_CONFIG_HOME", tt.xdg)
			got, err := Path()
			if err != nil {
				t.Fatalf("Path() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func assertEqual(t *testing.T, got, want Config) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("config mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestSectionActionsCanBeRebound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  merge: [\"ctrl+m\"]\n  star: [\"*\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys[ActionMerge]; !slices.Equal(got, []string{"ctrl+m"}) {
		t.Errorf("merge = %v, want [ctrl+m]", got)
	}
	if got := cfg.Keys[ActionClose]; !slices.Equal(got, []string{"x"}) {
		t.Errorf("close = %v, want the default [x]", got)
	}
}

func TestComposeActions(t *testing.T) {
	defaults := Default().Keys
	for action, want := range map[string]string{ActionComment: "c", ActionLabel: "l"} {
		if got := defaults[action]; !slices.Equal(got, []string{want}) {
			t.Errorf("default %s = %v, want [%s]", action, got, want)
		}
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  comment: [\"C\", \"ctrl+o\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys[ActionComment]; !slices.Equal(got, []string{"C", "ctrl+o"}) {
		t.Errorf("comment = %v, want [C ctrl+o]", got)
	}
	if got := cfg.Keys[ActionLabel]; !slices.Equal(got, []string{"l"}) {
		t.Errorf("label = %v, want the default [l]", got)
	}

	cfg = Default()
	cfg.Keys[ActionLabel] = nil
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "keys.label: needs at least one key") {
		t.Errorf("Validate() = %v, want label to need a key", err)
	}
}

func TestScreenActions(t *testing.T) {
	defaults := Default().Keys
	for action, want := range map[string]string{
		ActionPane1: "1", ActionPane2: "2", ActionPane3: "3", ActionNotifications: "n",
	} {
		if got := defaults[action]; !slices.Equal(got, []string{want}) {
			t.Errorf("default %s = %v, want [%s]", action, got, want)
		}
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  notifications: [\"N\"]\n  pane_1: [\"F\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys[ActionNotifications]; !slices.Equal(got, []string{"N"}) {
		t.Errorf("notifications = %v, want [N]", got)
	}
	if got := cfg.Keys[ActionPane1]; !slices.Equal(got, []string{"F"}) {
		t.Errorf("pane_1 = %v, want [F]", got)
	}
}
