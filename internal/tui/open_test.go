package tui

import (
	"slices"
	"testing"
)

// browser records the links the app opens, in place of the browser.
type browser struct{ urls []string }

func (b *browser) open(url string) error {
	b.urls = append(b.urls, url)
	return nil
}

func TestOpenCommand(t *testing.T) {
	tests := []struct {
		name string
		// dash opens the app on the dashboard, with no repository.
		dash bool
		host string
		line string
		// want is the link opened, if any, and toast the text of the
		// toast.
		want  string
		toast string
	}{
		{name: "repository", line: "open charmbracelet/bubbletea", want: "https://github.com/charmbracelet/bubbletea"},
		{name: "clone name", line: "open charmbracelet/bubbletea.git", want: "https://github.com/charmbracelet/bubbletea"},
		{name: "number", line: "open charmbracelet/bubbletea#1698", want: "https://github.com/charmbracelet/bubbletea/issues/1698"},
		{name: "pull request in memory", line: "open eggzec/gh-tui#7", want: "https://github.com/eggzec/gh-tui/pull/7"},
		{name: "number of the repository", line: "open #12", want: "https://github.com/eggzec/gh-tui/issues/12"},
		{name: "number without a repository", dash: true, line: "open #12", toast: "Open a repository first, or use open owner/name#12."},
		{name: "link", line: "open https://github.com/cli/cli/pull/3/files", want: "https://github.com/cli/cli/pull/3/files"},
		{name: "link to a file", line: "open github.com/cli/cli/blob/trunk/go.mod#L3", want: "https://github.com/cli/cli/blob/trunk/go.mod#L3"},
		{name: "enterprise", host: "ghe.example.com:8443", line: "open cli/cli#5", want: "https://ghe.example.com:8443/cli/cli/issues/5"},
		{name: "enterprise link", host: "ghe.example.com", line: "open https://ghe.example.com/cli/cli/tree/main", want: "https://ghe.example.com/cli/cli/tree/main"},
		{name: "http link", line: "open http://github.com/cli/cli/pull/3", want: "https://github.com/cli/cli/pull/3"},
		{name: "http link with its port", line: "open http://github.com:80/cli/cli", want: "https://github.com/cli/cli"},
		{name: "enterprise http link", host: "ghe.example.com:8443", line: "open http://ghe.example.com:8443/cli/cli/pull/3", want: "https://ghe.example.com:8443/cli/cli/pull/3"},
		{name: "user in a link", line: "open https://me@github.com/cli/cli", want: "https://github.com/cli/cli"},
		{name: "another host", line: "open https://gitlab.com/a/b", toast: "Can't open https://gitlab.com/a/b: not a link to github.com."},
		{name: "not a repository", line: "open bubbletea", toast: "Can't open bubbletea: want owner/name."},
		{name: "profile", line: "open @octocat", want: "https://github.com/octocat"},
		{name: "enterprise profile", host: "ghe.example.com", line: "open @octocat", want: "https://ghe.example.com/octocat"},
		{name: "link to a profile", line: "open https://github.com/orgs/charmbracelet/people", want: "https://github.com/orgs/charmbracelet/people"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &browser{}
			opts := []Option{WithBrowser(b.open), WithKinds(newGotoKinds())}
			if tt.host != "" {
				opts = append(opts, WithHost(tt.host))
			}
			if !tt.dash {
				opts = append(opts, WithRepo(testRepo))
			}
			m, fakes := newGotoApp(t, newGotoRepos(), opts...)
			runCommand(t, m, tt.line)
			var want []string
			if tt.want != "" {
				want = []string{tt.want}
			}
			if !slices.Equal(b.urls, want) {
				t.Errorf("opened %q, want %q", b.urls, want)
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
			for _, f := range fakes {
				if f.got(isKey("o")) {
					t.Errorf("%s got the open key", f.title)
				}
			}
		})
	}
}

// TestOpenCommandRefuses checks that open opens no link that could lead
// off the user's host, or out of the browser's web pages.
func TestOpenCommandRefuses(t *testing.T) {
	for _, line := range []string{
		"open https://github.com@evil.com/o/n",
		"open https://github.com:8443/o/n",
		"open http://github.com:443/o/n",
		"open javascript:alert(1)",
		"open javascript://github.com/o/n%0aalert(1)",
		"open file:///etc/passwd",
		"open file://github.com/o/n",
		"open data:text/html,<script>alert(1)</script>",
		"open data://github.com/o/n",
	} {
		t.Run(line, func(t *testing.T) {
			b := &browser{}
			m, _ := newGotoApp(t, newGotoRepos(), WithBrowser(b.open), WithRepo(testRepo))
			runCommand(t, m, line)
			if len(b.urls) > 0 {
				t.Errorf("opened %q", b.urls)
			}
			if toasted(m) == "" {
				t.Error("no toast says why not")
			}
		})
	}
}

// TestOpenCommandPressesTheKey checks that open alone opens what is
// selected by pressing the open key, where the key goes.
func TestOpenCommandPressesTheKey(t *testing.T) {
	b := &browser{}
	m, fakes := newGotoApp(t, newGotoRepos(), WithBrowser(b.open), WithRepo(testRepo))
	drive(m, m.key(press("3")))
	runCommand(t, m, "open")
	if !fakes[2].got(isKey("o")) || fakes[0].got(isKey("o")) {
		t.Error("open didn't press o in the focused pane alone")
	}
	if len(b.urls) != 0 {
		t.Errorf("opened %q, want what the pane asks for alone", b.urls)
	}
}
