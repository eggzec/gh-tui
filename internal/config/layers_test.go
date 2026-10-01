package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// loadBase loads the config file at path, as Load does, and returns its
// top level resolved over the defaults, with the settings the file had
// under old names.
func loadBase(path string) (Config, []Renamed, error) {
	f, err := Load(path)
	if err != nil {
		return Config{}, nil, err
	}
	return f.Base(), f.Renamed(), nil
}

// resolveFile loads content as a config file and resolves it for host
// and login.
func resolveFile(t *testing.T, content, host, login string) (Config, Source) {
	t.Helper()
	f, err := Load(writeConfig(t, content))
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	cfg, src, err := f.Resolve(host, login)
	if err != nil {
		t.Fatalf("Resolve(%q, %q) error = %v", host, login, err)
	}
	return cfg, src
}

const layered = `repos: [base/one]
theme: mine
themes:
  mine:
    light: &palette {accent: "#111111", foreground: "#111111", muted: "#111111", subtle: "#111111",
      border: "#111111", success: "#111111", warning: "#111111", error: "#111111"}
    dark: *palette
  work: {light: *palette, dark: *palette}
sync:
  poll: {lists: 2m}
hosts:
  GHE.corp.com:
    repos: [platform/api]
    sync: {poll: {lists: 5m}}
profiles:
  work:
    accounts: [Ali-Corp@ghe.corp.com, ali-work@github.com]
    theme: work
`

// Each layer goes over the one before it: the defaults, the top level,
// the session's host, and the profile of its account.
func TestResolveLayers(t *testing.T) {
	t.Setenv(EnvLog, "")
	tests := []struct {
		name, host, login string
		repos             []string
		theme             string
		interval          time.Duration
		src               Source
	}{
		{"top level", "github.com", "someone", []string{"base/one"}, "mine", 2 * time.Minute,
			Source{Host: "github.com", Account: "someone@github.com"}},
		{"host", "ghe.corp.com", "someone", []string{"platform/api"}, "mine", 5 * time.Minute,
			Source{Host: "ghe.corp.com", HostLayer: true, Account: "someone@ghe.corp.com"}},
		{"host and profile", "ghe.corp.com", "ali-corp", []string{"platform/api"}, "work", 5 * time.Minute,
			Source{Host: "ghe.corp.com", HostLayer: true, Account: "ali-corp@ghe.corp.com", Profile: "work"}},
		{"profile, login in another case", "github.com", "Ali-Work", []string{"base/one"}, "work", 2 * time.Minute,
			Source{Host: "github.com", Account: "ali-work@github.com", Profile: "work"}},
		{"account of another host", "github.com", "ali-corp", []string{"base/one"}, "mine", 2 * time.Minute,
			Source{Host: "github.com", Account: "ali-corp@github.com"}},
		{"no account", "ghe.corp.com", "", []string{"platform/api"}, "mine", 5 * time.Minute,
			Source{Host: "ghe.corp.com", HostLayer: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, src := resolveFile(t, layered, tt.host, tt.login)
			if !slices.Equal(cfg.Repos, tt.repos) || cfg.Theme != tt.theme || cfg.Sync.Poll.Lists != tt.interval {
				t.Errorf("repos %v, theme %q, interval %v; want %v, %q, %v", cfg.Repos, cfg.Theme, cfg.Sync.Poll.Lists, tt.repos, tt.theme, tt.interval)
			}
			// Only the session's fields: the rest says where each value
			// is set, which other tests check.
			if got := (Source{Host: src.Host, HostLayer: src.HostLayer, Account: src.Account, Profile: src.Profile}); !reflect.DeepEqual(got, tt.src) {
				t.Errorf("source = %+v, want %+v", src, tt.src)
			}
		})
	}
}

// A file without hosts or profiles resolves to its top level for every
// session, as it did before either existed.
func TestResolveWithoutLayers(t *testing.T) {
	t.Setenv(EnvLog, "")
	const file = "sync:\n  interval: 2m\n"
	base, _, err := loadBase(writeConfig(t, file))
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"github.com", "ghe.corp.com"} {
		if cfg, _ := resolveFile(t, file, host, "octocat"); !reflect.DeepEqual(cfg, base) {
			t.Errorf("config for %s differs from the top level's", host)
		}
	}
	if cfg, _ := resolveFile(t, "", "github.com", "octocat"); !reflect.DeepEqual(cfg, Default()) {
		t.Error("an empty file doesn't resolve to the defaults")
	}
}

// GH_TUI_LOG goes over the log level of every session.
func TestResolveLogLevelFromEnv(t *testing.T) {
	t.Setenv(EnvLog, "DEBUG")
	if cfg, _ := resolveFile(t, layered, "ghe.corp.com", "ali-corp"); cfg.Log.Level != LevelDebug {
		t.Errorf("log.level = %q, want debug", cfg.Log.Level)
	}
}

// What can't differ between hosts or accounts, and what the file lays out
// wrong, is refused at load, under whichever host or profile it is, with
// its line.
func TestLoadLayersErrors(t *testing.T) {
	t.Setenv(EnvLog, "")
	tests := []struct{ name, file, want string }{
		{"global under a host", "hosts:\n  ghe.corp.com:\n    ui:\n      icons: ascii\n",
			"hosts.ghe.corp.com: line 4: ui.icons can only be set at the top level"},
		{"keys under a profile", "profiles:\n  work:\n    accounts: [a@github.com]\n    keys:\n      quit: [x]\n",
			"profiles.work: line 4: keys can only be set at the top level"},
		{"log under a host", "hosts:\n  github.com:\n    log:\n      level: debug\n",
			"hosts.github.com: line 3: log can only be set at the top level"},
		{"disk dir under a host", "hosts:\n  github.com:\n    cache:\n      disk:\n        dir: /tmp/x\n",
			"line 5: cache.disk.dir can only be set at the top level"},
		{"images under a host", "hosts:\n  github.com:\n    images:\n      enabled: off\n",
			"hosts.github.com: line 3: images can only be set at the top level"},
		{"a theme's colour under a profile", "profiles:\n  work:\n    accounts: [a@github.com]\n    themes:\n      default:\n        dark: {accent: \"#000000\"}\n",
			"profiles.work: line 4: themes can only be set at the top level"},
		{"global through a merge key", "hosts:\n  github.com:\n    <<: {log: {level: debug}}\n",
			"hosts.github.com: line 3: log can only be set at the top level"},
		{"global through an anchor", "log: &l {level: debug}\nhosts:\n  github.com:\n    log: *l\n",
			"hosts.github.com: line 4: log can only be set at the top level"},
		{"account with two @", "profiles:\n  work:\n    accounts: [a@b@github.com]\n",
			`line 3: profiles.work: "a@b@github.com" isn't an account`},
		{"account host with a space", "profiles:\n  work:\n    accounts: [\"a@git hub.com\"]\n",
			`line 3: profiles.work: "a@git hub.com" isn't an account`},
		{"account host with a slash", "profiles:\n  work:\n    accounts: [a@github.com/x]\n",
			`line 3: profiles.work: "a@github.com/x" isn't an account`},
		{"account host only a dot", "profiles:\n  work:\n    accounts: [a@.]\n",
			`line 3: profiles.work: "a@." isn't an account`},
		{"unknown under a host", "hosts:\n  ghe.corp.com:\n    sink: {interval: 1m}\n",
			"hosts.ghe.corp.com: line 3: unknown setting sink"},
		{"hosts not a mapping", "hosts: [github.com]\n", "line 1: hosts must hold a group of settings"},
		{"host listed twice", "hosts:\n  github.com: {theme: default}\n  GitHub.com: {theme: default}\n",
			"line 3: hosts.GitHub.com is the host of line 2 too"},
		{"profile without accounts", "profiles:\n  work:\n    theme: default\n",
			"line 3: profiles.work.accounts must list the accounts"},
		{"account without a host", "profiles:\n  work:\n    accounts: [octocat]\n",
			`line 3: profiles.work: "octocat" isn't an account`},
		{"account in two profiles", "profiles:\n  a:\n    accounts: [x@github.com]\n  b:\n    accounts: [X@GitHub.com]\n",
			"line 5: profiles.b lists x@github.com, which profile a lists too"},
		{"empty value under a profile", "profiles:\n  work:\n    accounts: [x@github.com]\n    theme:\n",
			"profiles.work.theme is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.file))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %v, want it to say %q", err, tt.want)
			}
		})
	}
}

// A value that is bad only with a host or a profile over the top level is
// reported at load, whatever the session's host, once for the layer
// where it appears.
func TestLoadValidatesEveryCombination(t *testing.T) {
	t.Setenv(EnvLog, "")
	file := "hosts:\n  ghe.corp.com:\n    sync: {poll: {lists: 1s}}\n" +
		"profiles:\n  work:\n    accounts: [a@ghe.corp.com, b@github.com]\n    theme: nowhere\n"
	_, err := Load(writeConfig(t, file))
	if err == nil {
		t.Fatal("Load succeeded")
	}
	msg := err.Error()
	for _, want := range []string{
		"with hosts.ghe.corp.com:\nline 3: sync.poll.lists: must be at least 10s",
		"with profile work for b@github.com:\nline 7: theme: unknown theme \"nowhere\"",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error = %q, want it to say %q", msg, want)
		}
	}
	if strings.Contains(msg, "for a@ghe.corp.com") || strings.Count(msg, "sync.poll.lists") != 1 {
		t.Errorf("error = %q, want each problem once, where it appears", msg)
	}
}

// A profile's problem is told once, at its first account, however many
// accounts on healthy hosts it lists.
func TestLoadProfileProblemOnce(t *testing.T) {
	t.Setenv(EnvLog, "")
	hosts := "hosts:\n  github.com: {theme: default}\n  ghe.corp.com: {theme: default}\n"
	for name, tt := range map[string]struct{ setting, want string }{
		"bad value": {"theme: nowhere", `unknown theme "nowhere"`},
		"bad type":  {"sync: {interval: soon}", "soon"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeConfig(t, hosts+"profiles:\n  work:\n    accounts: [a@github.com, b@ghe.corp.com]\n    "+tt.setting+"\n"))
			if err == nil {
				t.Fatal("Load succeeded")
			}
			msg := err.Error()
			if strings.Count(msg, tt.want) != 1 || !strings.Contains(msg, "with profile work for a@github.com:\nline 7: ") || strings.Contains(msg, "b@ghe.corp.com") {
				t.Errorf("error = %q, want %q once, at a@github.com, with its line", msg, tt.want)
			}
		})
	}
}

// A host is matched as gh names it, whatever its case, spaces or
// trailing dot, in the file or in the session, and a port is part of it.
func TestResolveHostForms(t *testing.T) {
	t.Setenv(EnvLog, "")
	const file = "hosts:\n  GHE.corp.com.:\n    repos: [platform/api]\n  \"ghe.corp.com:8443\":\n    repos: [platform/web]\n" +
		"profiles:\n  work:\n    accounts: [a@GHE.corp.com.]\n    theme: default\n"
	for _, tt := range []struct{ host, repo, profile string }{
		{"ghe.corp.com", "platform/api", "work"},
		{" GHE.corp.com. ", "platform/api", "work"},
		{"ghe.corp.com:8443", "platform/web", ""},
	} {
		cfg, src := resolveFile(t, file, tt.host, "A")
		if !slices.Equal(cfg.Repos, []string{tt.repo}) || src.Profile != tt.profile {
			t.Errorf("Resolve(%q): repos %v, profile %q; want [%s], %q", tt.host, cfg.Repos, src.Profile, tt.repo, tt.profile)
		}
	}
}

// A global setting under its old name below the top level is refused by
// that name.
func TestLoadGlobalUnderOldName(t *testing.T) {
	withRenames(t, []rename{same("ui.old_icons", "ui.icons")})
	t.Setenv(EnvLog, "")
	_, err := Load(writeConfig(t, "hosts:\n  github.com:\n    ui:\n      old_icons: ascii\n"))
	want := "hosts.github.com: line 4: ui.old_icons (now ui.icons) can only be set at the top level"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Load error = %v, want it to say %q", err, want)
	}
}

// A setting under its old name is read under its new one in every layer,
// and named with its layer.
func TestLoadRenamedInLayers(t *testing.T) {
	withRenames(t, testRenames)
	t.Setenv(EnvLog, "")
	f, err := Load(writeConfig(t, "hosts:\n  ghe.corp.com:\n    sync:\n      every: 2m\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg, _, _ := f.Resolve("ghe.corp.com", ""); cfg.Sync.Poll.Lists != 2*time.Minute {
		t.Errorf("sync.poll.lists = %v, want 2m", cfg.Sync.Poll.Lists)
	}
	if r := f.Renamed(); len(r) != 1 || r[0].Old != "hosts.ghe.corp.com.sync.every" {
		t.Errorf("renamed = %+v, want the host's sync.every", r)
	}
}

// A bad value under its old name in a layer is named by that name, with
// its layer, and an old name of another layer isn't.
func TestLoadRenamedErrorsInLayers(t *testing.T) {
	withRenames(t, testRenames)
	t.Setenv(EnvLog, "")
	_, err := Load(writeConfig(t, "sync:\n  every: 2m\nhosts:\n  ghe.corp.com:\n    sync:\n      every: 1s\n"))
	want := "with hosts.ghe.corp.com:\nline 6: hosts.ghe.corp.com.sync.every (now sync.poll.lists): must be at least 10s"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Load error = %v, want it to say %q", err, want)
	}
	if strings.Contains(err.Error(), "sync.every, ") {
		t.Errorf("Load error = %v, want only the host's old name", err)
	}
}

func TestSourceHeader(t *testing.T) {
	tests := []struct {
		src  Source
		want []string
	}{
		{Source{Host: "ghe.corp.com", HostLayer: true, Account: "a@ghe.corp.com", Profile: "work"},
			[]string{"file: c.yaml", "host: ghe.corp.com (hosts.ghe.corp.com applies)", "account: a@ghe.corp.com", "profile: work"}},
		{Source{Host: "github.com", Account: "a@github.com"},
			[]string{"file: c.yaml", "host: github.com (the file has no settings under hosts.github.com)", "account: a@github.com", "profile: none lists a@github.com"}},
		{Source{Host: "github.com"},
			[]string{"file: c.yaml", "host: github.com (the file has no settings under hosts.github.com)", "account: none: the token names no account", "profile: none: no account to pick one by"}},
	}
	for _, tt := range tests {
		if got := tt.src.Header("c.yaml"); !slices.Equal(got, tt.want) {
			t.Errorf("Header = %q, want %q", got, tt.want)
		}
	}
}

// The polls may differ per host, a slow Enterprise Server polled less
// often, and per account.
func TestResolveSyncPerHost(t *testing.T) {
	t.Setenv(EnvLog, "")
	const file = "hosts:\n  ghe.corp.com:\n    sync: {poll: {checks: 30s}, unfocused_slowdown: 8}\n" +
		"profiles:\n  work:\n    accounts: [ali@ghe.corp.com]\n    sync: {poll: {lists: 5m}}\n"
	cfg, _ := resolveFile(t, file, "ghe.corp.com", "ali")
	want := Default().Sync
	want.Poll.Checks, want.Poll.Lists, want.UnfocusedSlowdown = 30*time.Second, 5*time.Minute, 8
	if cfg.Sync != want {
		t.Errorf("sync = %+v, want %+v", cfg.Sync, want)
	}
	if cfg, _ := resolveFile(t, file, "github.com", "ali"); cfg.Sync != Default().Sync {
		t.Errorf("sync on github.com = %+v, want the defaults", cfg.Sync)
	}
}
