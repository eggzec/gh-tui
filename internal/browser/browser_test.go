package browser

import (
	"errors"
	"os/exec"
	"slices"
	"testing"
	"time"
)

// starter records the commands a launcher starts, and ends each with err
// once release is closed, or at once when release is nil.
type starter struct {
	cmds    []*exec.Cmd
	err     error
	release chan struct{}
	reaped  chan struct{}
}

func newStarter() *starter { return &starter{reaped: make(chan struct{}, 1)} }

func (s *starter) start(cmd *exec.Cmd) (func() error, error) {
	s.cmds = append(s.cmds, cmd)
	return func() error {
		if s.release != nil {
			<-s.release
		}
		s.reaped <- struct{}{}
		return s.err
	}, nil
}

// env returns a getenv over vars.
func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// onPath returns a lookPath that finds only the programs named, in /bin.
func onPath(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		if slices.Contains(names, name) {
			return "/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
}

func TestOpenResolvesTheBrowser(t *testing.T) {
	all := []string{"xdg-open", "x-www-browser", "www-browser", "wslview", "firefox", "chromium", "open", "brave"}
	tests := []struct {
		name     string
		env      map[string]string
		ghConfig string
		goos     string
		path     []string
		want     []string
	}{
		{name: "GH_BROWSER first", env: map[string]string{"GH_BROWSER": "firefox --new-tab", "BROWSER": "brave"}, ghConfig: "chromium", want: []string{"/bin/firefox", "--new-tab"}},
		{name: "gh config next", env: map[string]string{"BROWSER": "brave"}, ghConfig: "chromium", want: []string{"/bin/chromium"}},
		{name: "BROWSER last", env: map[string]string{"BROWSER": "brave"}, want: []string{"/bin/brave"}},
		{name: "linux", goos: "linux", want: []string{"/bin/xdg-open"}},
		{name: "WSL", goos: "linux", env: map[string]string{"WSL_DISTRO_NAME": "Debian"}, want: []string{"/bin/wslview"}},
		{name: "x-www-browser", goos: "linux", path: []string{"x-www-browser", "www-browser", "wslview"}, want: []string{"/bin/x-www-browser"}},
		{name: "wslview last", goos: "linux", path: []string{"wslview"}, want: []string{"/bin/wslview"}},
		{name: "freebsd", goos: "freebsd", want: []string{"/bin/xdg-open"}},
		{name: "macOS", goos: "darwin", want: []string{"/bin/open"}},
		{name: "windows launcher", goos: "windows", env: map[string]string{"BROWSER": "firefox"}, want: []string{"/bin/firefox"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if path == nil {
				path = all
			}
			s := newStarter()
			l := New(WithEnv(env(tt.env)), WithGHConfig(func() string { return tt.ghConfig }),
				WithOS(tt.goos), WithLookPath(onPath(path...)), WithStarter(s.start))
			const u = "https://github.com/cli/cli"
			cmd, err := l.Open(u)
			if err != nil || cmd != nil {
				t.Fatalf("Open = %v, %v, want nil, nil", cmd, err)
			}
			if len(s.cmds) != 1 {
				t.Fatalf("started %d commands, want 1", len(s.cmds))
			}
			got := s.cmds[0]
			want := append(slices.Clone(tt.want), u)
			if got.Path != want[0] || !slices.Equal(got.Args[1:], want[1:]) {
				t.Errorf("started %s %q, want %q", got.Path, got.Args[1:], want)
			}
		})
	}
}

func TestOpenDetaches(t *testing.T) {
	s := newStarter()
	l := New(WithEnv(env(map[string]string{"BROWSER": "firefox"})), WithGHConfig(func() string { return "" }),
		WithLookPath(onPath("firefox")), WithStarter(s.start))
	if _, err := l.Open("https://github.com"); err != nil {
		t.Fatal(err)
	}
	cmd := s.cmds[0]
	if cmd.Stdin != nil || cmd.Stdout != nil || cmd.Stderr != nil {
		t.Errorf("streams %v %v %v, want all nil, the null device", cmd.Stdin, cmd.Stdout, cmd.Stderr)
	}
	if !detached(cmd) {
		t.Errorf("not detached: %+v", cmd.SysProcAttr)
	}
}

func TestOpenReportsQuickFailure(t *testing.T) {
	s := newStarter()
	s.err = errors.New("exit status 3")
	l := New(WithEnv(env(nil)), WithGHConfig(func() string { return "" }), WithOS("linux"),
		WithLookPath(onPath("xdg-open")), WithStarter(s.start), WithGrace(time.Minute))
	if _, err := l.Open("https://github.com"); err == nil {
		t.Error("Open succeeded, want the browser's failure")
	}
}

// TestOpenLeavesALongBrowser checks that Open returns once the grace is
// over, while the browser runs on, and that it still reaps the browser
// when it ends.
func TestOpenLeavesALongBrowser(t *testing.T) {
	s := newStarter()
	s.release = make(chan struct{})
	s.err = errors.New("killed")
	l := New(WithEnv(env(nil)), WithGHConfig(func() string { return "" }), WithOS("linux"),
		WithLookPath(onPath("xdg-open")), WithStarter(s.start), WithGrace(time.Millisecond))
	if _, err := l.Open("https://github.com"); err != nil {
		t.Fatalf("Open = %v, want nil while the browser runs", err)
	}
	close(s.release)
	select {
	case <-s.reaped:
	case <-time.After(5 * time.Second):
		t.Fatal("the browser was never waited for")
	}
}

func TestOpenHandsTextBrowsersBack(t *testing.T) {
	for _, b := range []string{"w3m", "lynx -accept_all_cookies", "/usr/bin/elinks"} {
		t.Run(b, func(t *testing.T) {
			s := newStarter()
			l := New(WithEnv(env(map[string]string{"BROWSER": b})), WithGHConfig(func() string { return "" }),
				WithLookPath(func(name string) (string, error) { return name, nil }), WithStarter(s.start))
			cmd, err := l.Open("https://github.com")
			if err != nil || cmd == nil {
				t.Fatalf("Open = %v, %v, want the command", cmd, err)
			}
			if len(s.cmds) != 0 {
				t.Error("started a browser that needs the terminal")
			}
			if cmd.Args[len(cmd.Args)-1] != "https://github.com" {
				t.Errorf("args %q lack the page", cmd.Args)
			}
		})
	}
}

func TestOpenRefuses(t *testing.T) {
	tests := []struct{ name, browser, url string }{
		{"file", "firefox", "file:///etc/passwd"},
		{"no scheme", "firefox", "github.com"},
		{"unquoted", `firefox "--new`, "https://github.com"},
		{"empty", `""`, "https://github.com"},
		{"blank", "   ", "https://github.com"},
		{"missing program", "nosuchbrowser", "https://github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStarter()
			l := New(WithEnv(env(map[string]string{"BROWSER": tt.browser})), WithGHConfig(func() string { return "" }),
				WithLookPath(onPath("firefox")), WithStarter(s.start))
			if _, err := l.Open(tt.url); err == nil {
				t.Error("Open succeeded, want an error")
			}
			if len(s.cmds) != 0 {
				t.Error("started a browser")
			}
		})
	}
}

func TestOpenOnWindowsUsesTheShell(t *testing.T) {
	s := newStarter()
	l := New(WithEnv(env(nil)), WithGHConfig(func() string { return "" }), WithOS("windows"),
		WithLookPath(onPath("rundll32")), WithStarter(s.start))
	var opened []string
	l.shellOpen = func(u string) error {
		opened = append(opened, u)
		return nil
	}
	const u = "https://github.com/cli/cli"
	if cmd, err := l.Open(u); cmd != nil || err != nil {
		t.Fatalf("Open = %v, %v, want nil, nil", cmd, err)
	}
	if !slices.Equal(opened, []string{u}) || len(s.cmds) != 0 {
		t.Errorf("shell opened %q and %d commands started, want only the shell", opened, len(s.cmds))
	}
}

func TestOpenHandsWWWBrowserBack(t *testing.T) {
	s := newStarter()
	l := New(WithEnv(env(nil)), WithGHConfig(func() string { return "" }), WithOS("linux"),
		WithLookPath(onPath("www-browser", "wslview")), WithStarter(s.start))
	cmd, err := l.Open("https://github.com")
	if err != nil || cmd == nil || cmd.Path != "/bin/www-browser" {
		t.Fatalf("Open = %v, %v, want www-browser to run in the terminal", cmd, err)
	}
	if len(s.cmds) != 0 {
		t.Error("started www-browser detached")
	}
}

func TestOpenFindsNoOpener(t *testing.T) {
	l := New(WithEnv(env(nil)), WithGHConfig(func() string { return "" }), WithOS("linux"),
		WithLookPath(onPath()), WithStarter(newStarter().start))
	if _, err := l.Open("https://github.com"); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("Open = %v, want exec.ErrNotFound", err)
	}
}

func TestOpenPassesTheURLAsGiven(t *testing.T) {
	for _, u := range []string{
		"https://github.com/cli/cli/blob/trunk/go.mod#L3",
		"https://ghe.example.com:8443/cli/cli/issues?q=is%3Aopen+label%3Abug",
		"http://github.com/cli/cli/pull/3/files",
	} {
		s := newStarter()
		l := New(WithEnv(env(map[string]string{"BROWSER": "firefox"})), WithGHConfig(func() string { return "" }),
			WithLookPath(onPath("firefox")), WithStarter(s.start))
		if _, err := l.Open(u); err != nil {
			t.Fatal(err)
		}
		if got := s.cmds[0].Args[len(s.cmds[0].Args)-1]; got != u {
			t.Errorf("passed %q, want %q", got, u)
		}
	}
}

func TestInTerminalPastEnv(t *testing.T) {
	tests := []struct {
		argv []string
		want bool
	}{
		{[]string{"env", "LANG=C", "TERM=xterm", "w3m"}, true},
		{[]string{"/usr/bin/env", "-i", "lynx", "-accept_all_cookies"}, true},
		{[]string{"env", "LANG=C", "firefox"}, false},
		{[]string{"env"}, false},
	}
	for _, tt := range tests {
		if got := InTerminal(program(tt.argv)); got != tt.want {
			t.Errorf("InTerminal(program(%q)) = %v, want %v", tt.argv, got, tt.want)
		}
	}
}
