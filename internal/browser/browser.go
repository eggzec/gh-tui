// Package browser opens web pages in the user's browser without touching
// the terminal the app draws on.
//
// It picks the browser as gh does: GH_BROWSER, then the browser in gh's
// config, then BROWSER, else the platform's opener. A graphical browser
// starts detached, with no standard streams and in a session of its own,
// so nothing it or its opener prints lands on the screen and nothing
// reads the keys meant for the app. A browser that runs in the terminal,
// such as w3m or lynx, can't work detached, so Open hands its command
// back for the caller to run with the terminal handed over.
package browser

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/google/shlex"
)

// Starter starts cmd and returns the function that waits for it to end.
type Starter func(cmd *exec.Cmd) (wait func() error, err error)

// Start starts cmd without waiting for it.
func Start(cmd *exec.Cmd) (func() error, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}

// grace is how long Open waits for the browser to fail before it takes
// the launch as done. An opener such as xdg-open fails at once when it
// finds nothing to open the page with, while a browser that starts may
// not end until the user closes it.
const grace = time.Second

// Launcher opens URLs in the browser. Build it with New.
type Launcher struct {
	launcher string
	goos     string
	getenv   func(string) string
	ghConfig func() string
	lookPath func(string) (string, error)
	start    Starter
	grace    time.Duration
}

// Option configures a Launcher.
type Option func(*Launcher)

// WithEnv sets where the launcher reads environment variables, os.Getenv
// by default.
func WithEnv(getenv func(string) string) Option {
	return func(l *Launcher) { l.getenv = getenv }
}

// WithGHConfig sets what reads the browser in gh's config, by default
// gh's config file.
func WithGHConfig(browser func() string) Option {
	return func(l *Launcher) { l.ghConfig = browser }
}

// WithLookPath sets how a command is found, exec.LookPath by default.
func WithLookPath(lookPath func(string) (string, error)) Option {
	return func(l *Launcher) { l.lookPath = lookPath }
}

// WithStarter sets what starts the browser, Start by default.
func WithStarter(start Starter) Option {
	return func(l *Launcher) { l.start = start }
}

// WithOS sets the platform whose opener is the default, runtime.GOOS by
// default.
func WithOS(goos string) Option {
	return func(l *Launcher) { l.goos = goos }
}

// WithGrace sets how long Open waits for the browser to fail.
func WithGrace(d time.Duration) Option {
	return func(l *Launcher) { l.grace = d }
}

// New returns a launcher with the browser the user chose, read once, now.
func New(opts ...Option) *Launcher {
	l := &Launcher{
		goos:     runtime.GOOS,
		getenv:   os.Getenv,
		ghConfig: ghBrowser,
		lookPath: exec.LookPath,
		start:    Start,
		grace:    grace,
	}
	for _, o := range opts {
		o(l)
	}
	l.launcher = l.resolve()
	return l
}

// resolve returns the browser the user chose, in gh's order, or "" for
// the platform's opener.
func (l *Launcher) resolve() string {
	if b := l.getenv("GH_BROWSER"); b != "" {
		return b
	}
	if b := l.ghConfig(); b != "" {
		return b
	}
	return l.getenv("BROWSER")
}

// ghBrowser reads the browser in gh's config, if gh has one.
func ghBrowser() string {
	cfg, err := config.Read(nil)
	if err != nil {
		return ""
	}
	b, _ := cfg.Get([]string{"browser"})
	return b
}

// Open opens u in the browser. A graphical browser starts detached and
// Open returns a nil command, once the browser has had a moment to fail.
// A browser that runs in the terminal isn't started: Open returns its
// command, for the caller to run with the terminal handed over.
func (l *Launcher) Open(u string) (*exec.Cmd, error) {
	if err := checkURL(u); err != nil {
		return nil, err
	}
	argv, err := l.argv()
	if err != nil {
		return nil, err
	}
	path, err := l.lookPath(argv[0])
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, append(argv[1:], u)...) //nolint:gosec // The program is the browser the user chose.
	if InTerminal(argv[0]) {
		return cmd, nil
	}
	detach(cmd)
	wait, err := l.start(cmd)
	if err != nil {
		return nil, err
	}
	// The goroutine reaps the browser whenever it ends, so it leaves no
	// zombie, and the buffer lets it end after Open has returned.
	done := make(chan error, 1)
	go func() { done <- wait() }()
	t := time.NewTimer(l.grace)
	defer t.Stop()
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(argv[0]), err)
		}
	case <-t.C:
	}
	return nil, nil
}

// argv returns the program and arguments that open a page, before the
// page itself.
func (l *Launcher) argv() ([]string, error) {
	if l.launcher != "" {
		argv, err := shlex.Split(l.launcher)
		if err != nil {
			return nil, fmt.Errorf("browser %q: %w", l.launcher, err)
		}
		if len(argv) == 0 {
			return nil, fmt.Errorf("browser %q names no program", l.launcher)
		}
		return argv, nil
	}
	switch l.goos {
	case "darwin":
		return []string{"open"}, nil
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler"}, nil
	case "linux":
		// WSL may have an xdg-open with nothing to open pages with,
		// while wslview opens them in the Windows browser.
		if l.getenv("WSL_DISTRO_NAME") != "" {
			if _, err := l.lookPath("wslview"); err == nil {
				return []string{"wslview"}, nil
			}
		}
		if _, err := l.lookPath("xdg-open"); err != nil {
			if _, werr := l.lookPath("wslview"); werr == nil {
				return []string{"wslview"}, nil
			}
		}
	}
	return []string{"xdg-open"}, nil
}

// textBrowsers are browsers that draw in the terminal, by the name of
// their program. Debian's www-browser names whichever of them is set up.
var textBrowsers = []string{"w3m", "lynx", "links", "links2", "elinks", "browsh", "carbonyl", "cha", "www-browser"}

// InTerminal reports whether program, a command or its path, is a browser
// that draws in the terminal.
func InTerminal(program string) bool {
	name := strings.ToLower(filepath.Base(program))
	return slices.Contains(textBrowsers, strings.TrimSuffix(name, ".exe"))
}

// errScheme is the error for a page that isn't on the web.
var errScheme = errors.New("only http and https pages open in the browser")

// checkURL refuses what isn't a web page, so the browser is never given
// a file or a program to open.
func checkURL(u string) error {
	p, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("open %q: %w", u, err)
	}
	if p.Scheme != "http" && p.Scheme != "https" {
		return fmt.Errorf("open %q: %w", u, errScheme)
	}
	return nil
}
