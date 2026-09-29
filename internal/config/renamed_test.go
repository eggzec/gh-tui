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
	same("sync.every", "sync.interval"),
	// One to several, with a new value.
	{
		old: []string{"details.prefetch.count"}, new: []string{"details.prefetch.rows", "history.prefetch.around"},
		note: "it now counts the rows after the cursor",
		move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			n, err := strconv.Atoi(v["details.prefetch.count"].Value)
			if err != nil {
				return nil, errors.New("want a number of rows")
			}
			out := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(max(n-1, 0))}
			return map[string]*yaml.Node{"details.prefetch.rows": out, "history.prefetch.around": out}, nil
		},
	},
	// One out of a group that is then left empty.
	same("old.glyph", "dashboard.calendar_glyph"),
	// Several to one: the longest delay wins.
	{
		old: []string{"a.delay", "b.delay"}, new: []string{"files.prefetch.hover_delay"},
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
			return map[string]*yaml.Node{"files.prefetch.hover_delay": longest}, nil
		},
	},
	// A setting that keeps its name and becomes a group, as cache.ttl will.
	same("cache.revalidate", "cache.revalidate.interval"),
	// One whose release has passed.
	{old: []string{"files.hover"}, new: []string{"files.prefetch.hover_delay"}},
	// A group renamed as a whole, whose release has passed.
	{old: []string{"gone.deep.key"}, new: []string{"sync.interval"}},
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
			want:    func(c *Config) { c.Sync.Interval = 2 * time.Minute },
			renamed: []string{"sync.every → sync.interval"},
		},
		{
			name: "one to several, the value moved",
			file: "details:\n  prefetch:\n    count: 5\n    enabled: false\n",
			want: func(c *Config) {
				c.Details.Prefetch.Rows, c.History.Prefetch.Around, c.Details.Prefetch.Enabled = 4, 4, false
			},
			renamed: []string{"details.prefetch.count → details.prefetch.rows, history.prefetch.around (it now counts the rows after the cursor)"},
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
			want:    func(c *Config) { c.Files.Prefetch.HoverDelay = 2 * time.Second },
			renamed: []string{"a.delay → files.prefetch.hover_delay", "b.delay → files.prefetch.hover_delay"},
		},
		{
			name:    "several to one, one of them set",
			file:    "b:\n  delay: 1s\n",
			want:    func(c *Config) { c.Files.Prefetch.HoverDelay = time.Second },
			renamed: []string{"b.delay → files.prefetch.hover_delay"},
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
			file:    "files:\n  prefetch:\n    hover_delay: &d 20s\nsync:\n  every: *d\n",
			want:    func(c *Config) { c.Files.Prefetch.HoverDelay, c.Sync.Interval = 20*time.Second, 20*time.Second },
			renamed: []string{"sync.every → sync.interval"},
		},
		{
			name:    "through a merge key",
			file:    "sync:\n  <<: {every: 3m}\n",
			want:    func(c *Config) { c.Sync.Interval = 3 * time.Minute },
			renamed: []string{"sync.every → sync.interval"},
		},
		{
			name: "none",
			file: "sync:\n  interval: 2m\n",
			want: func(c *Config) { c.Sync.Interval = 2 * time.Minute },
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
		old: []string{"details.old_hover", "history.old_hover"},
		new: []string{"details.prefetch.hover_delay", "history.prefetch.hover_delay"},
		move: func(v map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			out := map[string]*yaml.Node{}
			for o, n := range v {
				out[strings.Replace(o, "old_hover", "prefetch.hover_delay", 1)] = n
			}
			return out, nil
		},
	}})
	got, renamed, err := loadBase(writeConfig(t, "history:\n  old_hover: 2s\ndetails:\n  prefetch:\n    hover_delay: 3s\n"))
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	want := Default()
	want.History.Prefetch.HoverDelay, want.Details.Prefetch.HoverDelay = 2*time.Second, 3*time.Second
	assertEqual(t, got, want)
	if len(renamed) != 1 {
		t.Errorf("renamed = %v, want history.old_hover", renamed)
	}
	if _, _, err := loadBase(writeConfig(t, "history:\n  old_hover: 2s\n  prefetch:\n    hover_delay: 3s\n")); err == nil || !strings.Contains(err.Error(), "which line 4 sets too") {
		t.Errorf("Load error = %v, want the clash refused", err)
	}
}

func TestLoadRenamedErrors(t *testing.T) {
	withRenames(t, testRenames)
	tests := []struct {
		file string
		want []string
	}{
		{"sync:\n  every: 2m\n  interval: 3m\n", []string{"line 2: sync.every was renamed to sync.interval, which line 3 sets too: set only sync.interval"}},
		{"details:\n  prefetch:\n    count: many\n", []string{"line 3: details.prefetch.count: want a number of rows"}},
		{"files:\n  hover: 1s\n", []string{"line 2: files.hover was renamed to files.prefetch.hover_delay"}},
		{"gone:\n  deep:\n    key: 1m\n", []string{"line 3: gone.deep.key was renamed to sync.interval"}},
		{"sync:\n  evry: 2m\n", []string{"line 2: unknown setting sync.evry"}},
		{"old:\n  glyph: \"#\"\n  other: 1\n", []string{"line 1: unknown setting old"}},
		{"themes:\n  mine:\n    dark:\n      accnt: \"#fff\"\n", []string{"line 4: unknown setting themes.mine.dark.accnt"}},
		{"theme: default\ntheme: dusk\n", []string{"line 2: theme is set twice, here and at line 1"}},
		// Validate names the old setting the file has.
		{"sync:\n  every: 1s\n", []string{"sync.every (now sync.interval): must be at least 10s, got 1s"}},
		// Several old names that fed one setting are all named.
		{"a:\n  delay: -1s\nb:\n  delay: -5s\n", []string{"a.delay, b.delay (now files.prefetch.hover_delay): must not be negative, got -1s"}},
		// put can't make a group where the file sets a value.
		{"a:\n  delay: 1s\nfiles: 3\n", []string{"line 2: a.delay: can't move it to files.prefetch.hover_delay: line 3 sets files to a value"}},
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
			return map[string]*yaml.Node{"sync.interval": nil}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			withRenames(t, []rename{{old: []string{"x.y"}, new: []string{"sync.interval"}, move: move}})
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

// TestRenames checks the table itself.
func TestRenames(t *testing.T) {
	for _, err := range checkRenames(renames, Keys()) {
		t.Error(err)
	}
}

func TestCheckRenames(t *testing.T) {
	bad := []rename{
		{old: []string{"old.grp"}, new: []string{"sync.interval"}},
		{old: []string{"old.grp.a"}, new: []string{"sync.interval"}},
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
