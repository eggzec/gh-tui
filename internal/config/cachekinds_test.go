package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// oldTTL is how a file's cache.ttl is said to be renamed.
const oldTTL = "cache.ttl → cache.ttl.pulls, cache.ttl.issues, cache.ttl.notifications, cache.ttl.repos, cache.ttl.waiting_on_you, " +
	"cache.ttl.files, cache.ttl.history, cache.ttl.actions, cache.ttl.filters, cache.ttl.repo_info, cache.ttl.profile, " +
	"cache.ttl.contributions, cache.ttl.dashboard_repos (each kind of data has its own TTL now)"

func TestLoadOldCacheSettings(t *testing.T) {
	t.Setenv(EnvLog, "")
	tests := []struct {
		name, file string
		want       func(*Config)
		renamed    []string
	}{
		{
			name: "one TTL for the kinds that shared it",
			file: "cache:\n  ttl: 2m\n",
			want: func(c *Config) {
				k := &c.Cache.TTL
				for _, d := range []*time.Duration{&k.Pulls, &k.Issues, &k.Notifications, &k.Repos, &k.WaitingOnYou, &k.Files, &k.History, &k.Actions, &k.Filters} {
					*d = 2 * time.Minute
				}
			},
			renamed: []string{oldTTL},
		},
		{
			name: "one TTL longer than the kinds that took the longer",
			file: "cache:\n  ttl: 2h\n",
			want: func(c *Config) {
				k := &c.Cache.TTL
				for _, d := range []*time.Duration{&k.Pulls, &k.Issues, &k.Notifications, &k.Repos, &k.WaitingOnYou, &k.Files, &k.History, &k.Actions, &k.Filters, &k.RepoInfo, &k.Profile, &k.DashboardRepos} {
					*d = 2 * time.Hour
				}
			},
			renamed: []string{oldTTL},
		},
		{
			name: "the TTLs in their new form",
			file: "cache:\n  ttl:\n    pulls: 2m\n",
			want: func(c *Config) { c.Cache.TTL.Pulls = 2 * time.Minute },
		},
		{
			name:    "the budget of the checks",
			file:    "cache:\n  revalidate:\n    budget: 20\n",
			want:    func(c *Config) { c.Cache.Revalidate.PerMinute = 20 },
			renamed: []string{"cache.revalidate.budget → cache.revalidate.per_minute"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, renamed, err := loadBase(writeConfig(t, tt.file))
			if err != nil {
				t.Fatal(err)
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

// An old TTL that is wrong is named by the name the file has.
func TestOldTTLInvalid(t *testing.T) {
	t.Setenv(EnvLog, "")
	_, _, err := loadBase(writeConfig(t, "cache:\n  ttl: 0s\n"))
	if err == nil || !strings.Contains(err.Error(), "cache.ttl (now cache.ttl.pulls): must be positive, got 0s") {
		t.Errorf("Load error = %v, want it to name cache.ttl", err)
	}
}

// The old names are read for a host and a profile too.
func TestOldCacheSettingsLayered(t *testing.T) {
	t.Setenv(EnvLog, "")
	const file = "hosts:\n  ghe.corp.com:\n    cache:\n      ttl: 2m\n" +
		"profiles:\n  work:\n    accounts: [ali@github.com]\n    cache:\n      revalidate:\n        budget: 20\n"
	d := Default().Cache
	host, _ := resolveFile(t, file, "ghe.corp.com", "")
	if host.Cache.TTL.Pulls != 2*time.Minute || host.Cache.TTL.RepoInfo != d.TTL.RepoInfo || host.Cache.Revalidate.PerMinute != d.Revalidate.PerMinute {
		t.Errorf("for the host: ttl %+v, per_minute %d; want pulls at 2m and the rest as by default", host.Cache.TTL, host.Cache.Revalidate.PerMinute)
	}
	work, _ := resolveFile(t, file, "github.com", "ali")
	if work.Cache.Revalidate.PerMinute != 20 || work.Cache.TTL != d.TTL {
		t.Errorf("for the profile: ttl %+v, per_minute %d; want per_minute 20 and the TTLs as by default", work.Cache.TTL, work.Cache.Revalidate.PerMinute)
	}
	f, err := Load(writeConfig(t, file))
	if err != nil {
		t.Fatal(err)
	}
	renamed := f.Renamed()
	names := make([]string, 0, len(renamed))
	for _, r := range renamed {
		names = append(names, r.Old)
	}
	if !slices.Equal(names, []string{"hosts.ghe.corp.com.cache.ttl", "profiles.work.cache.revalidate.budget"}) {
		t.Errorf("renamed = %q, want cache.ttl of the host and cache.revalidate.budget of the profile", names)
	}
}
