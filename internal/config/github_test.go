package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTransportDefaults pins the defaults of the settings that were
// constants in Go, so moving them into default.yaml changed nothing.
func TestTransportDefaults(t *testing.T) {
	c := Default()
	if c.GitHub != (GitHub{Timeout: 30 * time.Second, Concurrency: 8}) {
		t.Errorf("github = %+v", c.GitHub)
	}
	want := PageSize{Pulls: 30, Issues: 30, Notifications: 30, Repos: 30, Runs: 30, Commits: 50, Search: 20, WaitingOnYou: 10}
	if c.PageSize != want {
		t.Errorf("page_size = %+v, want %+v", c.PageSize, want)
	}
	if c.UI.Toast != (Toast{Info: 4 * time.Second, Error: 8 * time.Second}) {
		t.Errorf("ui.toast = %+v", c.UI.Toast)
	}
	if c.Commands.History != 100 {
		t.Errorf("commands.history = %d, want 100", c.Commands.History)
	}
}

func TestTransportFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := `github: {timeout: 1m, concurrency: 4}
page_size: {pulls: 50, waiting_on_you: 20}
ui: {toast: {error: 20s}}
commands: {history: 500}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvLog, "")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.GitHub != (GitHub{Timeout: time.Minute, Concurrency: 4}) {
		t.Errorf("github = %+v", c.GitHub)
	}
	if c.PageSize.Pulls != 50 || c.PageSize.WaitingOnYou != 20 || c.PageSize.Issues != 30 {
		t.Errorf("page_size = %+v, want pulls 50, waiting_on_you 20 and the rest as by default", c.PageSize)
	}
	if c.UI.Toast != (Toast{Info: 4 * time.Second, Error: 20 * time.Second}) {
		t.Errorf("ui.toast = %+v", c.UI.Toast)
	}
	if c.Commands.History != 500 {
		t.Errorf("commands.history = %d, want 500", c.Commands.History)
	}
}

func TestTransportValidate(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(*Config)
		// err is in the error, or empty for none.
		err string
	}{
		{"timeout", func(c *Config) { c.GitHub.Timeout = time.Second }, ""},
		{"short timeout", func(c *Config) { c.GitHub.Timeout = 500 * time.Millisecond }, "github.timeout: must be at least 1s, got 500ms"},
		{"concurrency", func(c *Config) { c.GitHub.Concurrency = 3 }, ""},
		{"little concurrency", func(c *Config) { c.GitHub.Concurrency = 2 }, "github.concurrency: must be between 3 and 16, got 2"},
		{"much concurrency", func(c *Config) { c.GitHub.Concurrency = 17 }, "github.concurrency: must be between 3 and 16, got 17"},
		{"pages", func(c *Config) { c.PageSize.Pulls, c.PageSize.Search = 10, 100 }, ""},
		{"small page", func(c *Config) { c.PageSize.Search = 9 }, "page_size.search: must be between 10 and 100, got 9"},
		{"large page", func(c *Config) { c.PageSize.WaitingOnYou = 101 }, "page_size.waiting_on_you: must be between 10 and 100, got 101"},
		{"toast", func(c *Config) { c.UI.Toast.Info = time.Second }, ""},
		{"short toast", func(c *Config) { c.UI.Toast.Info = 0 }, "ui.toast.info: must be at least 1s, got 0s"},
		{"short error", func(c *Config) { c.UI.Toast.Error = -time.Second }, "ui.toast.error: must be at least 1s, got -1s"},
		{"history", func(c *Config) { c.Commands.History = 1 }, ""},
		{"no history", func(c *Config) { c.Commands.History = 0 }, "commands.history: must be between 1 and 10000, got 0"},
		{"long history", func(c *Config) { c.Commands.History = 10001 }, "commands.history: must be between 1 and 10000, got 10001"},
	} {
		c := Default()
		tt.set(&c)
		err := c.Validate()
		switch {
		case tt.err == "" && err != nil:
			t.Errorf("%s: Validate() = %v, want nil", tt.name, err)
		case tt.err != "" && (err == nil || !strings.Contains(err.Error(), tt.err)):
			t.Errorf("%s: Validate() = %v, want %q", tt.name, err, tt.err)
		}
	}
}

func TestSetToast(t *testing.T) {
	c, err := Default().Set("ui.toast.error", "30s")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, _ := c.Get("ui.toast.error"); got != "30s" {
		t.Errorf("ui.toast.error = %s, want 30s", got)
	}
	if _, err := Default().Set("ui.toast.info", "0s"); err == nil || !strings.Contains(err.Error(), "ui.toast.info") {
		t.Errorf("Set ui.toast.info=0s = %v, want it refused", err)
	}
}
