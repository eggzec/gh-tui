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
	t.Setenv(EnvLog, "")
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
				c.Details.Prefetch.Rows = 3
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
				c.Cache = Cache{TTL: 30 * time.Second, Disk: Disk{
					Enabled: false, Dir: "/var/cache/gh-tui", MaxSize: GiB,
					Compression: CompressionNone, CompressionLevel: LevelBest,
				}, Revalidate: Revalidate{Enabled: false, Interval: 5 * time.Minute, Budget: 30, Scope: ScopeAll}}
				c.Sync = Sync{Enabled: false, Interval: 2 * time.Minute}
				c.Files = Files{
					Prefetch: Prefetch{Enabled: false, MaxSize: 16 * KiB, HoverDelay: 300 * time.Millisecond},
					Preview:  Preview{MaxSize: 2_000_000},
					Finder:   Finder{Preview: false},
				}
				c.Details = Details{
					Prefetch: DetailsPrefetch{Enabled: false, Rows: 10, HoverDelay: time.Second},
				}
				c.Notifications = Notifications{MarkReadOnOpen: false}
				c.History = History{
					Row:        []string{FieldShortSHA, FieldSubject, FieldVerified, FieldAge},
					Detail:     []string{FieldSHA, FieldAuthor, FieldDate, FieldTrailers},
					DateFormat: "2006-01-02 15:04",
					ShowEmail:  true,
					Prefetch:   HistoryPrefetch{Around: 5, HoverDelay: 250 * time.Millisecond},
				}
				c.Dashboard = Dashboard{CalendarGlyph: "#", Contributions: ContributionsYear, Prefetch: false}
				c.UI = UI{Icons: IconsUnicode}
				c.Auth = Auth{Check: false}
				c.Log = Log{Level: LevelDebug, File: "/var/log/gh-tui.log", MaxSize: MiB, Keep: 5, Summary: time.Minute}
				c.Editor = "code --wait"
			},
		},
	}
	t.Setenv(EnvLog, "")
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
		{"invalid.yaml", []string{"repos[0]", "theme:", "keys.quit", "cache.ttl", "cache.disk.compression", "sync.interval: must be at least 10s, got 1s"}},
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
	cfg.Cache.Disk = Disk{Dir: "cache", MaxSize: MiB, Compression: "zip", CompressionLevel: "9"}
	cfg.Cache.Revalidate = Revalidate{Interval: time.Second, Budget: 0, Scope: "some"}
	cfg.Sync.Interval = -time.Second
	cfg.Files.Preview.MaxSize = 32 * KiB
	cfg.Files.Prefetch.HoverDelay = -time.Millisecond
	cfg.Details.Prefetch.Rows = 31
	cfg.Details.Prefetch.HoverDelay = -time.Second
	cfg.History = History{
		Row:        []string{FieldSubject, "sha", FieldSubject},
		Detail:     []string{FieldBody, "age"},
		DateFormat: "yesterday",
		Prefetch:   HistoryPrefetch{Around: 11, HoverDelay: -time.Millisecond},
	}
	cfg.Dashboard.CalendarGlyph = "■■"
	cfg.Dashboard.Contributions = "week"
	cfg.UI.Icons = "emoji"
	cfg.Log = Log{Level: "trace", File: "gh-tui.log", MaxSize: KiB, Keep: -1, Summary: time.Second}
	cfg.Editor = "vim\n-c q"

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
		`cache.disk.max_size: must be at least 8MiB, got 1MiB`,
		`cache.disk.dir: must be an absolute path, got "cache"`,
		`cache.disk.compression: must be gzip or none, got "zip"`,
		`cache.disk.compression_level: must be fastest, default or best, got "9"`,
		`cache.revalidate.interval: must be at least 10s, got 1s`,
		`cache.revalidate.budget: must be between 1 and 300, got 0`,
		`cache.revalidate.scope: must be recent or all, got "some"`,
		`sync.interval: must be at least 10s, got -1s`,
		`files.prefetch.max_size: must not exceed files.preview.max_size (32KiB), got 64KiB`,
		`files.prefetch.hover_delay: must not be negative, got -1ms`,
		`details.prefetch.rows: must be between 0 and 30, got 31`,
		`details.prefetch.hover_delay: must not be negative, got -1s`,
		`history.row[1]: unknown field "sha", want one of short_sha, subject, author, committer, age, date, verified, trailers`,
		`history.row[2]: "subject" is listed twice`,
		`history.detail[1]: unknown field "age", want one of sha, author, committer, date, verification, parents, trailers, body, stats`,
		`history.date_format: must be relative, absolute or a Go time layout such as "2006-01-02 15:04", got "yesterday"`,
		`history.prefetch.around: must be between 0 and 10, got 11`,
		`history.prefetch.hover_delay: must not be negative, got -1ms`,
		`dashboard.calendar_glyph: must be one character one cell wide, such as "■" or "#", got "■■"`,
		`dashboard.contributions: must be 30d, 90d or year, got "week"`,
		`ui.icons: must be nerd, unicode or ascii, got "emoji"`,
		`log.level: must be debug, info, warn or error, got "trace"`,
		`log.file: must be an absolute path, got "gh-tui.log"`,
		`log.max_size: must be at least 64KiB, got 1KiB`,
		`log.keep: must be between 0 and 100, got -1`,
		`log.summary: must be 0 or at least 10s, got 1s`,
		`editor: must be a program and its arguments on one line, such as "vim" or "code --wait", got "vim\n-c q"`,
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

func TestDiskPath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/xdg")
	if runtime.GOOS == "linux" {
		if got, err := Default().Cache.Disk.Path(); err != nil || got != "/tmp/xdg/gh-tui" {
			t.Errorf("default Path() = %q, %v; want /tmp/xdg/gh-tui", got, err)
		}
	}
	d := Disk{Dir: "/var/cache/gh"}
	if got, err := d.Path(); err != nil || got != "/var/cache/gh" {
		t.Errorf("Path() = %q, %v; want the configured dir", got, err)
	}
}

func TestLogLevelFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("log:\n  level: warn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		env, want, wantErr string
	}{
		{env: "", want: LevelWarn},
		{env: "debug", want: LevelDebug},
		{env: "ERROR", want: LevelError},
		{env: "loud", wantErr: `log.level: must be debug, info, warn or error, got "loud"`},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv(EnvLog, tt.env)
			cfg, err := Load(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Load error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Log.Level != tt.want {
				t.Errorf("level = %q, want %q", cfg.Log.Level, tt.want)
			}
		})
	}

	// The override applies without a config file too.
	t.Setenv(EnvLog, "debug")
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil || cfg.Log.Level != LevelDebug {
		t.Errorf("Load without a file = %q, %v; want debug", cfg.Log.Level, err)
	}
}

func TestLogPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the state directory is %LocalAppData% on Windows")
	}
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	if got, err := Default().Log.Path(); err != nil || got != "/tmp/state/gh-tui/gh-tui.log" {
		t.Errorf("default Path() = %q, %v; want /tmp/state/gh-tui/gh-tui.log", got, err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "relative")
	if got, err := Default().Log.Path(); err != nil || got != filepath.Join(home, ".local", "state", "gh-tui", "gh-tui.log") {
		t.Errorf("Path() without XDG_STATE_HOME = %q, %v; want it in ~/.local/state", got, err)
	}
	l := Log{File: "/var/log/gh.log"}
	if got, err := l.Path(); err != nil || got != "/var/log/gh.log" {
		t.Errorf("Path() = %q, %v; want the configured file", got, err)
	}
}

func TestActionsModalActions(t *testing.T) {
	defaults := Default().Keys
	for action, want := range map[string]string{
		ActionActions: "a", ActionNextFilter: "]", ActionPrevFilter: "[", ActionPaneLeft: "h", ActionPaneRight: "l",
		ActionZoom: "z", ActionRerunFailed: "ctrl+r", ActionRerun: "R", ActionRerunJob: "J", ActionCancelRun: "x",
	} {
		if got := defaults[action]; !slices.Equal(got, []string{want}) {
			t.Errorf("default %s = %v, want [%s]", action, got, want)
		}
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  rerun_failed: [\"F\"]\n  cancel_run: [\"C\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys[ActionRerunFailed]; !slices.Equal(got, []string{"F"}) {
		t.Errorf("rerun_failed = %v, want [F]", got)
	}
	if got := cfg.Keys[ActionRerun]; !slices.Equal(got, []string{"R"}) {
		t.Errorf("rerun = %v, want the default [R]", got)
	}

	cfg = Default()
	cfg.Keys[ActionZoom] = nil
	cfg.Keys["rerun_all"] = []string{"A"}
	err = cfg.Validate()
	for _, want := range []string{"keys.zoom: needs at least one key", "keys.rerun_all: unknown action"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() = %v, want %q", err, want)
		}
	}
}

func TestAuthDefaults(t *testing.T) {
	if !Default().Auth.Check {
		t.Error("the token isn't checked by default")
	}
}

func TestNotifications(t *testing.T) {
	if !Default().Notifications.MarkReadOnOpen {
		t.Error("opening a thread doesn't mark it read by default")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("notifications:\n  mark_read_on_open: sometimes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "sometimes") {
		t.Errorf("Load of a word for a bool = %v, want an error naming it", err)
	}
}

func TestFinderDefaults(t *testing.T) {
	cfg := Default()
	if got := cfg.Keys[ActionFindFile]; !slices.Equal(got, []string{"t", "ctrl+p"}) {
		t.Errorf("find_file = %v, want [t ctrl+p]", got)
	}
	if !cfg.Files.Finder.Preview {
		t.Error("the finder hides its preview by default")
	}
}

func TestCommandKey(t *testing.T) {
	if got := Default().Keys[ActionCommand]; !slices.Equal(got, []string{":"}) {
		t.Errorf("command = %v, want [:]", got)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  command: [\";\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Keys[ActionCommand]; !slices.Equal(got, []string{";"}) {
		t.Errorf("command after rebinding = %v, want [;]", got)
	}
}

func TestSortAndStarKeys(t *testing.T) {
	defaults := Default().Keys
	for action, want := range map[string]string{ActionFilter: "f", ActionSort: "s", ActionStar: "S"} {
		if got := defaults[action]; !slices.Equal(got, []string{want}) {
			t.Errorf("default %s = %v, want [%s]", action, got, want)
		}
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("keys:\n  sort: [\"o\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Keys[ActionSort]; !slices.Equal(got, []string{"o"}) {
		t.Errorf("sort = %v, want [o]", got)
	}
	if got := cfg.Keys[ActionStar]; !slices.Equal(got, []string{"S"}) {
		t.Errorf("star = %v, want the default [S]", got)
	}

	cfg = Default()
	cfg.Keys["sort_by"] = []string{"s"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "keys.sort_by: unknown action") {
		t.Errorf("Validate() = %v, want sort_by rejected", err)
	}
}
