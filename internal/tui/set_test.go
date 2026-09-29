package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// newSetApp returns an app of cfg on testRepo, whose settings are told to
// told.
func newSetApp(t *testing.T, cfg config.Config, told *[]config.Config) (*Model, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	m := New(t.Context(), cfg, layout, WithRepo(testRepo), WithSettings(func(c config.Config) { *told = append(*told, c) }))
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	return m, fakes
}

// userConfig returns the defaults with a theme of the user's, mine.
func userConfig() config.Config {
	cfg := config.Default()
	p, _ := cfg.Palette(true)
	p.Accent = "#ff00ff"
	cfg.Themes["mine"] = config.Theme{Light: p, Dark: p}
	return cfg
}

func TestSetCommand(t *testing.T) {
	tests := []struct {
		line  string
		toast string
		// changes reports whether the setting changes.
		changes bool
	}{
		{line: "set", toast: "Set what? Use set key=value, set key to see its value, or set key& to reset it."},
		{line: "set &", toast: "Set what? Use set key=value, set key to see its value, or set key& to reset it."},
		{line: "set theme&=mine", toast: "Set theme& resets it, and takes no value."},
		{line: "set theme&", toast: "theme is default, as gh-tui started with."},
		{line: "set nope&", toast: "Unknown setting: nope."},
		{line: "set cache.ttl&", toast: "cache.ttl is 5m, as gh-tui started with."},
		{line: "set theme", toast: "theme is default."},
		{line: "set ui.icons", toast: "ui.icons is nerd."},
		{line: "set log.file", toast: `log.file is "".`},
		{line: "set nope=1", toast: "Unknown setting: nope."},
		{line: "set nope", toast: "Unknown setting: nope."},
		{line: "set keys.quit=x", toast: "Keys and themes can't be set here: change them in the config file, then restart gh-tui."},
		{line: "set cache.ttl=1m", toast: "cache.ttl can't change while gh-tui runs: the cache is opened at startup. Set it in the config file, then restart."},
		{line: "set log.keep=5", toast: "log.keep can't change while gh-tui runs: the log file is opened at startup."},
		{line: "set log.level=debug", toast: "log.level is debug for this session.", changes: true},
		{line: "set github.timeout=1m", toast: "github.timeout can't change while gh-tui runs: the connection to GitHub is set up at startup. Set it in the config file, then restart."},
		{line: "set page_size.pulls=50", toast: "page_size.pulls can't change while gh-tui runs: the services read pages of this size from the start. Set it in the config file, then restart."},
		{line: "set commands.history=500", toast: "commands.history can't change while gh-tui runs: the command history is opened at startup. Set it in the config file, then restart."},
		{line: "set theme=nosuch", toast: `Can't set theme: unknown theme "nosuch".`},
		{line: "set theme=mine", toast: "theme is mine for this session.", changes: true},
		{line: "set  theme = mine ", toast: "theme is mine for this session.", changes: true},
		{line: "set ui.icons=ascii", toast: "ui.icons is ascii for this session.", changes: true},
		{line: "set details.prefetch.enabled=false", toast: "details.prefetch.enabled is false for this session.", changes: true},
		{line: "set sync.interval=1ms", toast: "Can't set sync.interval: must be at least 10s, got 1ms."},
		{line: "set sync.interval=10s", toast: "sync.interval is 10s for this session.", changes: true},
		{line: "set files.prefetch.max_size=2MiB", toast: "Can't set files.prefetch.max_size: must not exceed files.preview.max_size (1MiB), got 2MiB."},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			var told []config.Config
			m, fakes := newSetApp(t, userConfig(), &told)
			accent := m.theme.Palette.Accent
			for _, f := range fakes {
				f.themed = false
			}
			runCommand(t, m, tt.line)
			if !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
			changed := !reflect.DeepEqual(m.cfg, userConfig())
			if changed != tt.changes || len(told) > 0 != tt.changes {
				t.Fatalf("config changed %v, told %d times; want a change %v", changed, len(told), tt.changes)
			}
			if !tt.changes {
				return
			}
			if !reflect.DeepEqual(told[0], m.cfg) {
				t.Errorf("told %+v, want the config set", told[0])
			}
			if (m.cfg.Theme == "mine") != (m.theme.Palette.Accent != accent) {
				t.Errorf("theme %q with accent %s, was %s", m.cfg.Theme, m.theme.Palette.Accent, accent)
			}
			for _, f := range fakes {
				if !f.themed || !f.got(func(msg tea.Msg) bool { s, ok := msg.(ui.SettingsMsg); return ok && reflect.DeepEqual(s.Config, m.cfg) }) {
					t.Errorf("%s wasn't given the settings, then the theme", f.title)
				}
			}
		})
	}
}

// TestSetWritesNothing checks that a setting changes for the session only:
// the config file stays as it was, and nothing else is written beside it.
func TestSetWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const file = "theme: default\nui:\n  icons: nerd\n"
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvPath, path)
	t.Setenv("XDG_CONFIG_HOME", dir)
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded.Base()
	cfg.Themes["mine"] = userConfig().Themes["mine"]
	var told []config.Config
	m, _ := newSetApp(t, cfg, &told)
	runCommand(t, m, "set theme=mine")
	if m.cfg.Theme != "mine" {
		t.Fatalf("theme = %q, want it set", m.cfg.Theme)
	}
	if got, _ := os.ReadFile(path); string(got) != file {
		t.Errorf("the config file changed:\n%s", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("files beside the config: %v", entries)
	}
}

// TestSetToast checks that the set command changes how long later toasts
// stay, and refuses a time too short to read them.
func TestSetToast(t *testing.T) {
	if n := New(t.Context(), config.Default(), Layout{}); n.toast.Duration() != 4*time.Second || n.toast.ErrorDuration() != 8*time.Second {
		t.Errorf("toasts stay %v and %v, want ui.toast's 4s and 8s", n.toast.Duration(), n.toast.ErrorDuration())
	}
	var told []config.Config
	m, _ := newSetApp(t, userConfig(), &told)
	runCommand(t, m, "set ui.toast.error=0s")
	if !hasToast(m, "Can't set ui.toast.error: must be at least 1s, got 0s.") {
		t.Errorf("toasts: %s", toasted(m))
	}
	if m.setCommand("ui.toast.error=30s") == nil {
		t.Error("no command to show the change")
	}
	if m.toast.ErrorDuration() != 30*time.Second || m.toast.Duration() != m.cfg.UI.Toast.Info {
		t.Errorf("durations = %v, %v; want %v, 30s", m.toast.Duration(), m.toast.ErrorDuration(), m.cfg.UI.Toast.Info)
	}
}

// TestSetReset checks that key& drops what the session set of key, back
// to what the config file says, which need not be the default, and
// leaves what the session set of the others.
func TestSetReset(t *testing.T) {
	file := userConfig()
	file.UI.Icons = config.IconsUnicode
	var told []config.Config
	m, _ := newSetApp(t, file, &told)
	runCommand(t, m, "set ui.icons=ascii")
	runCommand(t, m, "set theme=mine")
	runCommand(t, m, "set ui.icons&")
	if !hasToast(m, "ui.icons is unicode again, as gh-tui started with.") {
		t.Errorf("toasts: %s", toasted(m))
	}
	if m.cfg.UI.Icons != config.IconsUnicode || m.cfg.Theme != "mine" {
		t.Errorf("icons %q, theme %q; want the file's icons and the session's theme", m.cfg.UI.Icons, m.cfg.Theme)
	}
	if n := len(told); n != 3 || !reflect.DeepEqual(told[n-1], m.cfg) {
		t.Errorf("told %d times, last %+v; want 3, the last the config reset", n, told[n-1])
	}
	runCommand(t, m, "set theme &")
	if !reflect.DeepEqual(m.cfg, file) {
		t.Errorf("after resetting both, the config isn't the file's")
	}
}

func TestCompleteSet(t *testing.T) {
	m, _ := newTestApp(t)
	tests := []struct {
		line string
		want []string
	}{
		{line: "se", want: []string{"search ", "set "}},
		{line: "set ui.ic", want: []string{"ui.icons="}},
		{line: "set icons", want: []string{"ui.icons="}},
		{line: "set sync.", want: []string{"sync.enabled=", "sync.interval="}},
		{line: "set ui.icons=", want: []string{"nerd", "unicode", "ascii"}},
		{line: "set ui.icons=u", want: []string{"unicode"}},
		{line: "set sync.enabled=", want: []string{"true", "false"}},
		{line: "set theme=", want: []string{"default"}},
		{line: "set sync.interval=", want: nil},
		{line: "set nope=", want: nil},
		{line: "set ui.icons nerd", want: nil},
	}
	for _, tt := range tests {
		if got := texts(m.complete(tt.line, len(tt.line))); !slices.Equal(got, tt.want) {
			t.Errorf("complete(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
	if got := m.complete("set them x", 8); len(got) != 1 || got[0].Text != "theme" || got[0].Detail != "default" {
		t.Errorf("a key before more text = %+v, want theme without = and its value", got)
	}
	if got := m.complete("set cache.ttl", 13); len(got) != 1 || got[0].Detail != "5m, at startup" {
		t.Errorf("cache.ttl = %+v, want its value and that it is read at startup", got)
	}
	if got := m.complete("set ui.icons=a", 14); got[0].Start != 13 || got[0].End != 14 {
		t.Errorf("value span = %d..%d, want 13..14", got[0].Start, got[0].End)
	}
	if got := m.complete("set ", 4); len(got) != maxCandidates || !strings.HasSuffix(got[0].Text, "=") {
		t.Errorf("keys = %q, want the first %d", texts(got), maxCandidates)
	}
}

// orderSection records whether it was given the settings, and whether the
// theme came after them.
type orderSection struct {
	*fakeSection
	settings, themedAfter bool
}

func (s *orderSection) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(ui.SettingsMsg); ok {
		s.settings = true
	}
	return s.fakeSection.Update(msg)
}

func (s *orderSection) SetTheme(t ui.Theme) {
	s.themedAfter = s.settings
	s.fakeSection.SetTheme(t)
}

// TestSetThemesAfterSettings checks that sections are drawn again after
// they took the settings, so that they draw with what changed, such as the
// icons.
func TestSetThemesAfterSettings(t *testing.T) {
	files := &orderSection{fakeSection: &fakeSection{title: "Files"}}
	m := New(t.Context(), config.Default(), Layout{Files: files}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	runCommand(t, m, "set ui.icons=unicode")
	if !files.settings || !files.themedAfter {
		t.Errorf("settings %v, themed after them %v", files.settings, files.themedAfter)
	}
}

// TestSetDrawsTheHintsAgain checks that a setting set drops the key hints
// the status bar found, which the theme draws.
func TestSetDrawsTheHintsAgain(t *testing.T) {
	m, _ := newTestApp(t)
	_ = m.View()
	if m.layers == nil {
		t.Fatal("the view found no key hints to keep")
	}
	// The line closing lays the screen out, which drops them too, so the
	// settings are applied alone here.
	m.applySettings()
	if m.layers != nil {
		t.Error("the key hints were kept across the settings")
	}
	if s := onScreen(m); !strings.Contains(s, "?") {
		t.Errorf("the hints are gone:\n%s", s)
	}
}
