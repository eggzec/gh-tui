package config

import (
	"fmt"
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
	a.Keys[ActionQuit][0] = "x"
	a.Keys[ActionHelp] = []string{"x"}
	a.History.Row[0] = "x"
	if b := Default(); b.Keys[ActionQuit][0] == "x" || b.Keys[ActionHelp][0] == "x" || b.History.Row[0] == "x" {
		t.Errorf("Default() shares its keymap or lists between calls: quit = %v, help = %v, row = %v", b.Keys[ActionQuit], b.Keys[ActionHelp], b.History.Row)
	}
	// Every map and list, however deep, a new one included.
	if shared := sharedRefs(reflect.ValueOf(Default()), reflect.ValueOf(Default()), ""); len(shared) > 0 {
		t.Errorf("Default() shares these between calls: %v", shared)
	}
}

// sharedRefs returns the paths of the maps, non-empty slices and
// pointers that a and b, two values of one type, share.
func sharedRefs(a, b reflect.Value, path string) []string {
	var out []string
	switch a.Kind() {
	case reflect.Struct:
		for i := range a.NumField() {
			out = append(out, sharedRefs(a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name)...)
		}
	case reflect.Map:
		if !a.IsNil() && a.UnsafePointer() == b.UnsafePointer() {
			out = append(out, path)
		}
		for _, k := range a.MapKeys() {
			out = append(out, sharedRefs(a.MapIndex(k), b.MapIndex(k), fmt.Sprintf("%s[%v]", path, k))...)
		}
	case reflect.Pointer:
		if !a.IsNil() && a.UnsafePointer() == b.UnsafePointer() {
			out = append(out, path)
		}
		if !a.IsNil() && !b.IsNil() {
			out = append(out, sharedRefs(a.Elem(), b.Elem(), path)...)
		}
	case reflect.Interface:
		if !a.IsNil() && !b.IsNil() {
			out = append(out, sharedRefs(a.Elem(), b.Elem(), path)...)
		}
	case reflect.Slice:
		if a.Len() > 0 && a.UnsafePointer() == b.UnsafePointer() {
			out = append(out, path)
		}
		for i := range a.Len() {
			out = append(out, sharedRefs(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i))...)
		}
	default:
	}
	return out
}

// TestSharedRefs checks that sharedRefs finds what two values share
// through a pointer or an interface too, such as the optional settings
// that fall back to others.
func TestSharedRefs(t *testing.T) {
	type inner struct{ list []int }
	type outer struct {
		p *inner
		i any
	}
	shared, own := &inner{list: []int{1}}, &inner{list: []int{1}}
	list := []int{1}
	a := outer{p: shared, i: inner{list: list}}
	b := outer{p: shared, i: inner{list: list}}
	if got := sharedRefs(reflect.ValueOf(a), reflect.ValueOf(b), ""); len(got) != 3 {
		t.Errorf("sharedRefs = %v, want .p, .p.list and .i.list", got)
	}
	c := outer{p: own, i: inner{list: []int{1}}}
	if got := sharedRefs(reflect.ValueOf(a), reflect.ValueOf(c), ""); len(got) != 0 {
		t.Errorf("sharedRefs of copies = %v, want none", got)
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
			got, _, err := loadBase(tt.path)
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
				c.Cache.TTL.Pulls = 10 * time.Minute
				c.Prefetch.Pulls.Window.After = new(3)
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
				c.Cache = Cache{
					TTL: TTL{
						Pulls: time.Minute, Issues: 2 * time.Minute, Notifications: 3 * time.Minute, Repos: 4 * time.Minute,
						DashboardRepos: 30 * time.Minute, WaitingOnYou: 6 * time.Minute, RepoInfo: 2 * time.Hour,
						Files: 7 * time.Minute, History: 8 * time.Minute, Compare: time.Minute,
						Actions: 9 * time.Minute, ActionsRunning: 20 * time.Second, Filters: 10 * time.Minute,
						Search: 40 * time.Second, CodeSearch: 2 * time.Minute, Releases: 3 * time.Hour,
						Profile: 4 * time.Hour, Contributions: 12 * time.Hour, People: 2 * time.Hour, Readme: 3 * time.Hour,
					},
					Memory: Memory{Entries: 512, Files: 16 * MiB, Trees: 24 * MiB, Diffs: 8 * MiB, Logs: 128 * MiB},
					Disk: Disk{
						Enabled: false, Dir: "/var/cache/gh-tui", MaxSize: GiB,
						Compression: CompressionNone, CompressionLevel: LevelBest,
					},
					Revalidate: Revalidate{Enabled: false, Interval: 5 * time.Minute, PerMinute: 30, Scope: ScopeAll, Recent: 24 * time.Hour},
				}
				c.Sync = Sync{
					Enabled:           false,
					Poll:              Poll{Notifications: 2 * time.Minute, Lists: 3 * time.Minute, Actions: 20 * time.Second, Checks: 30 * time.Second},
					UnfocusedSlowdown: 2,
				}
				c.Files = Files{
					Preview:  Preview{MaxSize: 2_000_000},
					Finder:   Finder{Preview: false},
					Markdown: MarkdownRaw,
				}
				c.Prefetch.Enabled, c.Prefetch.Window, c.Prefetch.Rest, c.Prefetch.Parallel = false, Window{Before: 2, After: 6}, time.Second, 2
				c.Prefetch.Pulls.OtherTabs.Enabled = new(false)
				c.Notifications = Notifications{MarkReadOnOpen: false}
				c.Images = Images{Enabled: ImagesOff, MaxRows: 8}
				c.History = History{
					Row:       []string{FieldShortSHA, FieldSubject, FieldVerified, FieldAge},
					Detail:    []string{FieldSHA, FieldAuthor, FieldDate, FieldTrailers},
					ShowEmail: true,
				}
				files, finder, history := &c.Prefetch.Files, &c.Prefetch.Finder, &c.Prefetch.History
				files.Rest = new(300 * time.Millisecond)
				files.Preview.Enabled, files.Preview.MaxSize = new(false), 16*KiB
				finder.Preview.Window, finder.Preview.MaxSize = Span{Before: new(1), After: new(2)}, 8*KiB
				history.Window, history.Rest = Span{Before: new(5), After: new(5)}, new(250*time.Millisecond)
				c.Dashboard = Dashboard{CalendarGlyph: "#", Contributions: ContributionsYear}
				c.Owner = Owner{DefaultTab: OwnerTabPeople}
				c.UI = UI{Icons: IconsUnicode, Toast: Toast{Info: 6 * time.Second, Error: 12 * time.Second}, DateFormat: "2006-01-02 15:04"}
				c.Auth = Auth{Check: false}
				c.GitHub = GitHub{Timeout: time.Minute, Concurrency: 4}
				c.PageSize = PageSize{Pulls: 50, Issues: 40, Notifications: 20, Repos: 60, Runs: 25, Commits: 100, Search: 10, WaitingOnYou: 15, People: 40}
				c.Commands = Commands{History: 500}
				c.Log = Log{Level: LevelDebug, File: "/var/log/gh-tui.log", MaxSize: MiB, Keep: 5, Summary: time.Minute}
				c.Editor = "code --wait"
			},
		},
	}
	t.Setenv(EnvLog, "")
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, _, err := loadBase(filepath.Join("testdata", tt.file))
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
		{"unknown_field.yaml", []string{"line 3: unknown setting cache.size"}},
		{"malformed.yaml", []string{"malformed.yaml", "line 3"}},
		{"invalid.yaml", []string{"repos[0]", "theme:", `line 4: keys.quit: unknown key "ctlr+q"`, "cache.ttl.pulls: must be positive", "cache.disk.compression", "sync.poll.lists: must be at least 10s, got 1s"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			_, _, err := loadBase(filepath.Join("testdata", tt.file))
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
	cfg.Themes["bad"] = Theme{Light: cfg.Themes["default"].Light}
	cfg.Keys[ActionHelp] = []string{"ctlr+h"}
	cfg.Keys[ActionSearch] = []string{""}
	cfg.Keys["jump"] = []string{"j"}
	cfg.Cache.TTL.Pulls = 0
	cfg.Cache.Memory.Entries = 0
	cfg.Cache.Disk = Disk{Dir: "cache", MaxSize: MiB, Compression: "zip", CompressionLevel: "9"}
	cfg.Cache.Revalidate = Revalidate{Interval: time.Second, PerMinute: 0, Scope: "some"}
	cfg.Sync.Poll.Checks = -time.Second
	cfg.Sync.UnfocusedSlowdown = 0
	cfg.Files.Preview.MaxSize = 32 * KiB
	cfg.Prefetch.Finder.Preview.MaxSize = -1
	cfg.Prefetch.Window.After = 31
	cfg.Prefetch.Rest = -time.Second
	cfg.History = History{
		Row:    []string{FieldSubject, "sha", FieldSubject},
		Detail: []string{FieldBody, "age"},
	}
	cfg.Dashboard.CalendarGlyph = "■■"
	cfg.Dashboard.Contributions = "week"
	cfg.UI.Icons = "emoji"
	cfg.UI.DateFormat = "yesterday"
	cfg.Images = Images{Enabled: "yes", MaxRows: 0}
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
		`keys.help: unknown key "ctlr+h", want a name such as r, R, ctrl+r, shift+tab, enter or space`,
		`keys.jump: unknown action`,
		`keys.search: empty key`,
		`cache.ttl.pulls: must be positive, got 0s`,
		`cache.memory.entries: must be at least 1, got 0`,
		`cache.disk.max_size: must be at least 8MiB, got 1MiB`,
		`cache.disk.dir: must be an absolute path, got "cache"`,
		`cache.disk.compression: must be gzip or none, got "zip"`,
		`cache.disk.compression_level: must be fastest, default or best, got "9"`,
		`cache.revalidate.interval: must be at least 10s, got 1s`,
		`cache.revalidate.per_minute: must be between 1 and 300, got 0`,
		`cache.revalidate.scope: must be recent or all, got "some"`,
		`cache.revalidate.recent: must be positive, got 0s`,
		`sync.poll.checks: must be at least 10s, got -1s`,
		`sync.unfocused_slowdown: must be between 1 and 60, got 0`,
		`prefetch.files.preview.max_size: must be between 0B and files.preview.max_size (32KiB), got 64KiB`,
		`prefetch.finder.preview.max_size: must be between 0B and files.preview.max_size (32KiB), got -1B`,
		`prefetch.window.after: must be between 0 and 30, got 31`,
		`prefetch.rest: must be between 0 and 2s, got -1s`,
		`history.row[1]: unknown field "sha", want one of short_sha, subject, author, committer, age, date, verified, trailers`,
		`history.row[2]: "subject" is listed twice`,
		`history.detail[1]: unknown field "age", want one of sha, author, committer, date, verification, parents, trailers, body, stats`,
		`dashboard.calendar_glyph: must be one character one cell wide, such as "■" or "#", got "■■"`,
		`dashboard.contributions: must be 30d, 90d or year, got "week"`,
		`ui.icons: must be nerd, unicode or ascii, got "emoji"`,
		`ui.date_format: must be relative, absolute or a Go time layout such as "2006-01-02 15:04", got "yesterday"`,
		`images.enabled: must be auto, on or off, got "yes"`,
		`images.max_rows: must be at least 1, got 0`,
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
	cfg, _, err := loadBase(path)
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
	cfg, _, err := loadBase(path)
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
	cfg.Keys[ActionLabel] = []string{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want label unbound", err)
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
	cfg, _, err := loadBase(path)
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
			cfg, _, err := loadBase(path)
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
	cfg, _, err := loadBase(filepath.Join(t.TempDir(), "missing.yaml"))
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
	cfg, _, err := loadBase(path)
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
	cfg.Keys[ActionZoom] = []string{"zz"}
	cfg.Keys["rerun_all"] = []string{"A"}
	err = cfg.Validate()
	for _, want := range []string{`keys.zoom: unknown key "zz"`, "keys.rerun_all: unknown action"} {
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
	if _, _, err := loadBase(path); err == nil || !strings.Contains(err.Error(), "sometimes") {
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

func TestMarkdownSetting(t *testing.T) {
	if got := Default().Files.Markdown; got != MarkdownRendered {
		t.Errorf("files.markdown = %q by default, want %q", got, MarkdownRendered)
	}
	for _, v := range []string{MarkdownRendered, MarkdownRaw} {
		cfg, err := Default().Set("files.markdown", v)
		if err != nil || cfg.Files.Markdown != v {
			t.Errorf("set files.markdown=%s: %q, %v", v, cfg.Files.Markdown, err)
		}
	}
	if _, err := Default().Set("files.markdown", "html"); err == nil || !strings.Contains(err.Error(), "rendered or raw") {
		t.Errorf("set files.markdown=html: %v, want an error naming the values", err)
	}
	if got := Default().Values("files.markdown"); !slices.Equal(got, []string{MarkdownRendered, MarkdownRaw}) {
		t.Errorf("values of files.markdown = %v", got)
	}
	// The config command shows it, and where a value set for the session
	// came from.
	session, err := Default().Set("files.markdown", MarkdownRaw)
	if err != nil {
		t.Fatal(err)
	}
	for cfg, want := range map[*Config]string{
		new(Default()): "  markdown: rendered\n",
		&session:       "  markdown: raw # session (:set)\n",
	} {
		got, err := Layers{Start: Default(), Session: *cfg}.YAML()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, want) {
			t.Errorf("the config lacks %q:\n%s", want, got)
		}
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
	cfg, _, err := loadBase(path)
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
	cfg, _, err := loadBase(path)
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
