package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
)

// newConfigApp returns an app of the config file holding file, on a
// screen tall enough to show the head of the config.
func newConfigApp(t *testing.T, file string) *Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvLog, "")
	f, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, src, err := f.Resolve("github.com", "octocat")
	if err != nil {
		t.Fatal(err)
	}
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	m := New(t.Context(), cfg, layout, WithRepo(testRepo), WithSource(src))
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 80})
	return m
}

// shownText returns the open modal's title and what it shows, without
// styles, or fails if no text modal is open.
func shownText(t *testing.T, m *Model) (title, text string) {
	t.Helper()
	tm, ok := m.modal.(*textModal)
	if !ok {
		t.Fatalf("the open modal is %T, want the text modal", m.modal)
	}
	return tm.Title(), ansi.Strip(tm.View())
}

func TestConfigCommand(t *testing.T) {
	// Not parallel: it sets environment variables, which the whole process shares.
	m := newConfigApp(t, "ui:\n  icons: ascii\nkeys:\n  global: {quit: [Q]}\n")
	runCommand(t, m, "set log.level=warn")
	runCommand(t, m, "config")
	title, text := shownText(t, m)
	if title != "Config" {
		t.Errorf("title %q, want Config", title)
	}
	for _, want := range []string{
		"# file: ",
		"# account: octocat@github.com",
		"icons: ascii # config.yaml:2",
		"pulls: 5m ",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the config lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "pulls: 5m #") {
		t.Errorf("a default has a comment:\n%s", text)
	}
	// What lies below the screen is in the pager, a search away.
	tm := m.modal.(*textModal)
	for _, want := range []string{"level: warn # session (:set)", "quit: [Q] # config.yaml:4"} {
		tm.pager.GoToLine(0)
		drive(m, tm.pager.SetSearch(want))
		if !strings.Contains(ansi.Strip(tm.View()), want) {
			t.Errorf("the config lacks %q", want)
		}
	}
	// q is not the quit key of this config, and Q is.
	drive(m, m.key(press("q")))
	if m.modal == nil {
		t.Errorf("q closed the pager, but this config quits with Q")
	}
	drive(m, m.key(press("Q")))
	if m.modal != nil {
		t.Errorf("Q left %T open, want the pager closed", m.modal)
	}
}

func TestConfigDefaults(t *testing.T) {
	// Not parallel: it sets environment variables, which the whole process shares.
	m := newConfigApp(t, "")
	runCommand(t, m, "config defaults")
	title, text := shownText(t, m)
	if title != "Default config" {
		t.Errorf("title %q, want Default config", title)
	}
	first, _, _ := strings.Cut(config.DefaultFile(), "\n")
	if !strings.Contains(text, first) {
		t.Errorf("the defaults lack their first line %q:\n%s", first, text)
	}
}

func TestConfigCommandNoFile(t *testing.T) {
	t.Parallel()
	m := New(t.Context(), config.Default(), Layout{Files: &fakeSection{title: "Files"}}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	runCommand(t, m, "config")
	_, text := shownText(t, m)
	if !strings.Contains(text, "# file: none") || strings.Contains(text, "config.yaml:") {
		t.Errorf("without a source the config names a file:\n%s", text)
	}
}

func TestConfigCommandRefusesMore(t *testing.T) {
	// Not parallel: it sets environment variables, which the whole process shares.
	m := newConfigApp(t, "")
	runCommand(t, m, "config nope")
	if m.modal != nil {
		t.Errorf("config nope opened %T", m.modal)
	}
	if !hasToast(m, `Can't show config "nope". Use config, or config defaults.`) {
		t.Errorf("toasts: %s", toasted(m))
	}
}

func TestCompleteConfig(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	tests := []struct {
		line string
		want []string
	}{
		{line: "con", want: []string{"config "}},
		{line: "config ", want: []string{"defaults"}},
		{line: "config d", want: []string{"defaults"}},
		{line: "config x", want: nil},
		{line: "config defaults x", want: nil},
	}
	for _, tt := range tests {
		if got := texts(m.complete(tt.line, len(tt.line))); !slices.Equal(got, tt.want) {
			t.Errorf("complete(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

// TestConfigAfterReset checks that a setting that :set changed, and :set
// key& reset, shows where the file set it again.
func TestConfigAfterReset(t *testing.T) {
	// Not parallel: it sets environment variables, which the whole process shares.
	m := newConfigApp(t, "ui:\n  icons: ascii\n")
	runCommand(t, m, "set ui.icons=unicode")
	runCommand(t, m, "config")
	if _, text := shownText(t, m); !strings.Contains(text, "icons: unicode # session (:set)") {
		t.Errorf("after set, the config lacks the session's icons:\n%s", text)
	}
	drive(m, m.key(press("q")))
	runCommand(t, m, "set ui.icons&")
	runCommand(t, m, "config")
	if _, text := shownText(t, m); !strings.Contains(text, "icons: ascii # config.yaml:2") {
		t.Errorf("after the reset, the config lacks the file's icons:\n%s", text)
	}
}

// TestPrefetchTable checks the comments at the end of the config command
// that show what each page and kind of item reads ahead, resolved.
func TestPrefetchTable(t *testing.T) {
	t.Parallel()
	text := prefetchTable(config.Default().Prefetch)
	for _, want := range []string{
		"# What each page and kind of item reads ahead",
		"#   pulls.checks ",
		"enabled false (prefetch.pulls.checks.enabled)",
		"window 0 (prefetch.files.preview.window.before) / 32 (prefetch.files.preview.window.after)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the table lacks %q:\n%s", want, text)
		}
	}
	for line := range strings.Lines(text) {
		if line != "\n" && !strings.HasPrefix(line, "#") {
			t.Errorf("line %q isn't a comment, so the YAML would read it", line)
		}
	}
}
