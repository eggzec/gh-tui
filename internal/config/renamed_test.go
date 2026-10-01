package config

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// withRenames makes table the renames for the rest of the test.
func withRenames(t *testing.T, table []rename) {
	t.Helper()
	was := renames
	renames = table
	t.Cleanup(func() { renames = was })
}

// same moves the value of one old setting to one new one.
func same(from, to string) rename {
	return rename{old: []string{from}, new: []string{to}, move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
		return map[string]*yaml.Node{to: v[from]}, nil
	}}
}

// testRenames are renames of the kinds the table will hold.
var testRenames = []rename{
	// One setting to another.
	same("sync.every", "sync.poll.lists"),
	// One to several, with a new value.
	{
		old: []string{"sync.count"}, new: []string{"prefetch.issues.window.after", "prefetch.history.window.after"},
		note: "it now counts the rows after the cursor",
		move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			n, err := strconv.Atoi(v["sync.count"].Value)
			if err != nil {
				return nil, errors.New("want a number of rows")
			}
			out := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(max(n-1, 0))}
			return map[string]*yaml.Node{"prefetch.issues.window.after": out, "prefetch.history.window.after": out}, nil
		},
	},
	// One out of a group that is then left empty.
	same("old.glyph", "dashboard.calendar_glyph"),
	// Several to one: the longest delay wins.
	{
		old: []string{"a.delay", "b.delay"}, new: []string{"prefetch.files.rest"},
		move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			var longest *yaml.Node
			var most time.Duration
			for _, n := range v {
				d, err := time.ParseDuration(n.Value)
				if err != nil {
					return nil, err
				}
				if longest == nil || d > most {
					longest, most = n, d
				}
			}
			return map[string]*yaml.Node{"prefetch.files.rest": longest}, nil
		},
	},
	// A setting that keeps its name and becomes a group, as cache.ttl will.
	same("cache.revalidate", "cache.revalidate.interval"),
	// One whose release has passed.
	{old: []string{"files.hover"}, new: []string{"prefetch.files.rest"}},
	// A group renamed as a whole, whose release has passed.
	{old: []string{"gone.deep.key"}, new: []string{"sync.poll.lists"}},
}

func TestLoadRenamed(t *testing.T) {
	withRenames(t, testRenames)
	t.Setenv(EnvLog, "")
	tests := []struct {
		name, file string
		want       func(*Config)
		renamed    []string
	}{
		{
			name:    "one to one",
			file:    "sync:\n  every: 2m\n",
			want:    func(c *Config) { c.Sync.Poll.Lists = 2 * time.Minute },
			renamed: []string{"sync.every → sync.poll.lists"},
		},
		{
			name: "one to several, the value moved",
			file: "sync:\n  count: 5\n  enabled: false\n",
			want: func(c *Config) {
				c.Prefetch.Issues.Window.After, c.Prefetch.History.Window.After, c.Sync.Enabled = new(4), new(4), false
			},
			renamed: []string{"sync.count → prefetch.issues.window.after, prefetch.history.window.after (it now counts the rows after the cursor)"},
		},
		{
			name:    "a group left empty goes",
			file:    "old:\n  glyph: \"#\"\n",
			want:    func(c *Config) { c.Dashboard.CalendarGlyph = "#" },
			renamed: []string{"old.glyph → dashboard.calendar_glyph"},
		},
		{
			name:    "several to one",
			file:    "a:\n  delay: 1s\nb:\n  delay: 2s\n",
			want:    func(c *Config) { c.Prefetch.Files.Rest = new(2 * time.Second) },
			renamed: []string{"a.delay → prefetch.files.rest", "b.delay → prefetch.files.rest"},
		},
		{
			name:    "several to one, one of them set",
			file:    "b:\n  delay: 1s\n",
			want:    func(c *Config) { c.Prefetch.Files.Rest = new(time.Second) },
			renamed: []string{"b.delay → prefetch.files.rest"},
		},
		{
			name:    "a value that became a group",
			file:    "cache:\n  revalidate: 5m\n",
			want:    func(c *Config) { c.Cache.Revalidate.Interval = 5 * time.Minute },
			renamed: []string{"cache.revalidate → cache.revalidate.interval"},
		},
		{
			name: "that group in its new form",
			file: "cache:\n  revalidate:\n    interval: 5m\n",
			want: func(c *Config) { c.Cache.Revalidate.Interval = 5 * time.Minute },
		},
		{
			name:    "through an alias",
			file:    "cache:\n  revalidate:\n    interval: &d 20s\nsync:\n  every: *d\n",
			want:    func(c *Config) { c.Cache.Revalidate.Interval, c.Sync.Poll.Lists = 20*time.Second, 20*time.Second },
			renamed: []string{"sync.every → sync.poll.lists"},
		},
		{
			name:    "through a merge key",
			file:    "sync:\n  <<: {every: 3m}\n",
			want:    func(c *Config) { c.Sync.Poll.Lists = 3 * time.Minute },
			renamed: []string{"sync.every → sync.poll.lists"},
		},
		{
			name: "none",
			file: "sync:\n  poll:\n    lists: 2m\n",
			want: func(c *Config) { c.Sync.Poll.Lists = 2 * time.Minute },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, renamed, err := loadBase(writeConfig(t, tt.file))
			if err != nil {
				t.Fatalf("Load error = %v", err)
			}
			want := Default()
			tt.want(&want)
			assertEqual(t, got, want)
			var names []string
			for _, r := range renamed {
				names = append(names, r.String())
			}
			if !slices.Equal(names, tt.renamed) {
				t.Errorf("renamed = %q, want %q", names, tt.renamed)
			}
		})
	}
}

// TestLoadRenamedShared checks that a rename of several old names to
// several new ones refuses a file only for a new name that an old name of
// the file feeds.
func TestLoadRenamedShared(t *testing.T) {
	withRenames(t, []rename{{
		old: []string{"files.old_rest", "history.old_rest"},
		new: []string{"prefetch.files.rest", "prefetch.history.rest"},
		move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			out := map[string]*yaml.Node{}
			for o, n := range v {
				out["prefetch."+strings.TrimSuffix(o, ".old_rest")+".rest"] = n
			}
			return out, nil
		},
	}})
	got, renamed, err := loadBase(writeConfig(t, "history:\n  old_rest: 200ms\nprefetch:\n  files:\n    rest: 300ms\n"))
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	want := Default()
	want.Prefetch.History.Rest, want.Prefetch.Files.Rest = new(200*time.Millisecond), new(300*time.Millisecond)
	assertEqual(t, got, want)
	if len(renamed) != 1 {
		t.Errorf("renamed = %v, want history.old_rest", renamed)
	}
	if _, _, err := loadBase(writeConfig(t, "history:\n  old_rest: 200ms\nprefetch:\n  history:\n    rest: 300ms\n")); err == nil || !strings.Contains(err.Error(), "which line 5 sets too") {
		t.Errorf("Load error = %v, want the clash refused", err)
	}
}

func TestLoadRenamedErrors(t *testing.T) {
	withRenames(t, testRenames)
	tests := []struct {
		file string
		want []string
	}{
		{"sync:\n  every: 2m\n  poll:\n    lists: 3m\n", []string{"line 2: sync.every was renamed to sync.poll.lists, which line 4 sets too: set only sync.poll.lists"}},
		{"sync:\n  count: many\n", []string{"line 2: sync.count: want a number of rows"}},
		{"files:\n  hover: 1s\n", []string{"line 2: files.hover was renamed to prefetch.files.rest"}},
		{"gone:\n  deep:\n    key: 1m\n", []string{"line 3: gone.deep.key was renamed to sync.poll.lists"}},
		{"sync:\n  evry: 2m\n", []string{"line 2: unknown setting sync.evry"}},
		{"old:\n  glyph: \"#\"\n  other: 1\n", []string{"line 1: unknown setting old"}},
		{"themes:\n  mine:\n    dark:\n      accnt: \"#fff\"\n", []string{"line 4: unknown setting themes.mine.dark.accnt"}},
		{"theme: default\ntheme: dusk\n", []string{"line 2: theme is set twice, here and at line 1"}},
		// Validate names the old setting the file has.
		{"sync:\n  every: 1s\n", []string{"line 2: sync.every (now sync.poll.lists): must be at least 10s, got 1s"}},
		// A value a move made has no line of its own, so it is given the
		// old name's.
		{"sync:\n  count: 40\n", []string{
			"line 2: sync.count (now prefetch.issues.window.after): must be between 0 and 30, got 39",
			"line 2: sync.count (now prefetch.history.window.after): must be between 0 and 10, got 39",
		}},
		// Several old names that fed one setting are all named.
		{"a:\n  delay: -1s\nb:\n  delay: -5s\n", []string{"a.delay, b.delay (now prefetch.files.rest): must be between 0 and 2s, got -1s"}},
		// put can't make a group where the file sets a value.
		{"a:\n  delay: 1s\nprefetch: 3\n", []string{"line 2: a.delay: can't move it to prefetch.files.rest: line 3 sets prefetch to a value"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			_, renamed, err := loadBase(writeConfig(t, tt.file))
			if err == nil {
				t.Fatalf("Load error = nil, renamed %v; want an error", renamed)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("Load error = %q, want it to contain %q", err, w)
				}
			}
			if strings.Contains(err.Error(), "line 0:") {
				t.Errorf("Load error = %q, names line 0", err)
			}
		})
	}
}

// TestMoveContract checks that a move that returns a path its rename
// doesn't list, or no node, is refused rather than dropped or followed.
func TestMoveContract(t *testing.T) {
	for name, move := range map[string]func(map[string]*yaml.Node) (map[string]*yaml.Node, error){
		"a path not listed": func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			return map[string]*yaml.Node{"editor": v["x.y"]}, nil
		},
		"no node": func(map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			return map[string]*yaml.Node{"sync.poll.lists": nil}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			withRenames(t, []rename{{old: []string{"x.y"}, new: []string{"sync.poll.lists"}, move: move}})
			if _, _, err := loadBase(writeConfig(t, "x:\n  y: 1m\n")); err == nil || !strings.Contains(err.Error(), "isn't one of its new names or has no value") {
				t.Errorf("Load error = %v, want the move refused", err)
			}
		})
	}
}

func TestRenamedWarning(t *testing.T) {
	one := Renamed{Old: "a.b", New: []string{"c.d"}}
	two := Renamed{Old: "e", New: []string{"f", "g"}, Note: "it counts differently"}
	for _, tt := range []struct {
		in   []Renamed
		want string
	}{
		{nil, ""},
		{[]Renamed{one}, "Your config uses an old setting: a.b → c.d. Rename it in the config file: the next release refuses the old names."},
		{[]Renamed{two, one}, "Your config uses old settings: e → f, g (it counts differently) (and 1 more; the log lists them). Rename them in the config file: the next release refuses the old names."},
		{[]Renamed{one, two, one, two}, "Your config uses old settings: a.b → c.d (and 3 more; the log lists them). Rename them in the config file: the next release refuses the old names."},
	} {
		if got := RenamedWarning(tt.in); got != tt.want {
			t.Errorf("RenamedWarning(%v) =\n%q, want\n%q", tt.in, got, tt.want)
		}
	}
}

func TestLoadSyncInterval(t *testing.T) {
	t.Setenv(EnvLog, "")
	got, renamed, err := loadBase(writeConfig(t, "sync:\n  interval: 2m\n"))
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	want := Default()
	want.Sync.Poll.Notifications, want.Sync.Poll.Lists = 2*time.Minute, 2*time.Minute
	assertEqual(t, got, want)
	if len(renamed) != 1 || renamed[0].String() != "sync.interval → sync.poll.notifications, sync.poll.lists" {
		t.Errorf("renamed = %v, want sync.interval", renamed)
	}
	if _, _, err := loadBase(writeConfig(t, "sync:\n  interval: 5s\n")); err == nil ||
		!strings.Contains(err.Error(), "line 2: sync.interval (now sync.poll.notifications): must be at least 10s, got 5s") ||
		!strings.Contains(err.Error(), "line 2: sync.interval (now sync.poll.lists): must be at least 10s, got 5s") {
		t.Errorf("Load error = %v, want the old name in what is wrong", err)
	}
}

// TestLoadRenamedPrefetch checks the settings that moved under prefetch.
func TestLoadRenamedPrefetch(t *testing.T) {
	t.Setenv(EnvLog, "")
	file := "files:\n  prefetch:\n    enabled: false\n    max_size: 16KiB\n    hover_delay: 300ms\n"
	got, renamed, err := loadBase(writeConfig(t, file))
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	want := Default()
	files := &want.Prefetch.Files
	files.Preview.Enabled, files.Preview.MaxSize, files.Rest = new(false), 16*KiB, new(300*time.Millisecond)
	assertEqual(t, got, want)
	names := make([]string, 0, len(renamed))
	for _, r := range renamed {
		names = append(names, r.String())
	}
	wantNames := []string{
		"files.prefetch.enabled → prefetch.files.preview.enabled",
		"files.prefetch.max_size → prefetch.files.preview.max_size",
		"files.prefetch.hover_delay → prefetch.files.rest",
	}
	if !slices.Equal(names, wantNames) {
		t.Errorf("renamed = %q, want %q", names, wantNames)
	}
}

// TestRenames checks the table itself.
func TestRenames(t *testing.T) {
	for _, err := range checkRenames(renames, Keys()) {
		t.Error(err)
	}
}

func TestCheckRenames(t *testing.T) {
	bad := []rename{
		{old: []string{"old.grp"}, new: []string{"sync.poll.lists"}},
		{old: []string{"old.grp.a"}, new: []string{"sync.poll.lists"}},
		{old: []string{"theme"}, new: []string{"nope"}},
		{old: []string{"x"}, new: []string{"editor"}},
		{old: []string{"x"}, new: []string{"editor"}},
		{new: []string{"editor"}},
	}
	errs := checkRenames(bad, Keys())
	got := make([]string, 0, len(errs))
	for _, err := range errs {
		got = append(got, err.Error())
	}
	for _, want := range []string{
		"the old name old.grp.a sits inside old.grp",
		"theme was renamed, but is still a setting",
		"[theme] was renamed to nope, which is no setting",
		"the old name x is listed twice",
		"rename [] → [editor]: needs an old name and a new one",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("checkRenames is missing %q; got %q", want, got)
		}
	}
	if err := checkRenames(testRenames, Keys()); len(err) > 0 {
		t.Errorf("checkRenames(testRenames) = %v, want none", err)
	}
}

// TestHistoryDateFormatRenamed reads the old history.date_format as
// ui.date_format, which now tells every date, at the top level and for a
// host, and names the old setting in what is wrong with it.
func TestHistoryDateFormatRenamed(t *testing.T) {
	t.Setenv(EnvLog, "")
	cfg, renamed, err := loadBase(writeConfig(t, "history:\n  date_format: absolute\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UI.DateFormat != DateAbsolute {
		t.Errorf("ui.date_format = %q, want absolute", cfg.UI.DateFormat)
	}
	want := "history.date_format → ui.date_format (it now applies to every date)"
	if len(renamed) != 1 || renamed[0].String() != want {
		t.Errorf("renamed = %v, want %s", renamed, want)
	}
	f, err := Load(writeConfig(t, "hosts:\n  ghe.corp.com:\n    history:\n      date_format: absolute\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg, _, _ := f.Resolve("ghe.corp.com", ""); cfg.UI.DateFormat != DateAbsolute {
		t.Errorf("the host's ui.date_format = %q, want absolute", cfg.UI.DateFormat)
	}
	for file, want := range map[string]string{
		"history:\n  date_format: absolute\nui:\n  date_format: relative\n": "set only ui.date_format",
		"history:\n  date_format: yesterday\n":                              `history.date_format (now ui.date_format): must be relative, absolute`,
	} {
		if _, err := Load(writeConfig(t, file)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load(%q) error = %v, want it to contain %q", file, err, want)
		}
	}
}
