package config

import (
	"fmt"
	"reflect"
	"slices"
	"time"

	"go.yaml.in/yaml/v3"
)

// TTL is how long what was read of each kind of data is shown without
// asking GitHub again. After that it is read again, usually with a
// conditional request that costs nothing against the rate limit when
// nothing changed. Every kind has its own, and none takes another's.
type TTL struct {
	// Pulls covers the lists and details of pull requests, and their
	// comments.
	Pulls time.Duration `yaml:"pulls"`
	// Issues covers the lists and details of issues, and their comments.
	Issues        time.Duration `yaml:"issues"`
	Notifications time.Duration `yaml:"notifications"`
	// Repos covers the lists of repositories: the search screen's start,
	// an owner's repositories and the completion of :goto.
	Repos time.Duration `yaml:"repos"`
	// DashboardRepos covers the dashboard's Repositories pane, which lists
	// every owner's repositories and costs more to read again.
	DashboardRepos time.Duration `yaml:"dashboard_repos"`
	// WaitingOnYou covers the dashboard's pull requests and issues that
	// wait on you.
	WaitingOnYou time.Duration `yaml:"waiting_on_you"`
	// RepoInfo covers one repository's details, such as its default
	// branch and whether it is private.
	RepoInfo time.Duration `yaml:"repo_info"`
	// Files covers where branches point. The files of a commit never go
	// stale.
	Files time.Duration `yaml:"files"`
	// History covers branches and the commits of a branch. A commit
	// itself never goes stale.
	History time.Duration `yaml:"history"`
	// Compare covers comparisons of two refs.
	Compare time.Duration `yaml:"compare"`
	// Actions covers workflow runs, finished jobs and checks, and
	// ActionsRunning the jobs of a run that is still going.
	Actions        time.Duration `yaml:"actions"`
	ActionsRunning time.Duration `yaml:"actions_running"`
	// Filters covers the labels, milestones and people the filters offer.
	Filters    time.Duration `yaml:"filters"`
	Search     time.Duration `yaml:"search"`
	CodeSearch time.Duration `yaml:"code_search"`
	Releases   time.Duration `yaml:"releases"`
	// Profile covers the dashboard's header: you, your organizations and
	// your pinned repositories; and the header of a user or organization
	// page.
	Profile       time.Duration `yaml:"profile"`
	Contributions time.Duration `yaml:"contributions"`
	// People covers the people of a user or organization page:
	// followers, following, organizations, members and teams.
	People time.Duration `yaml:"people"`
	// Readme covers profile READMEs, which have ETags.
	Readme time.Duration `yaml:"readme"`
}

// Memory is what the process keeps in memory.
type Memory struct {
	// Entries is how many entries each kind of data keeps, such as the
	// pages of pull requests, or of files read.
	Entries int `yaml:"entries"`
	// Files, Trees, Diffs and Logs bound the memory that the contents of
	// files, listings of files, the changes of commits and the logs of
	// jobs take.
	Files Size `yaml:"files"`
	Trees Size `yaml:"trees"`
	Diffs Size `yaml:"diffs"`
	Logs  Size `yaml:"logs"`
}

func (t TTL) validate() []error {
	var errs []error
	v := reflect.ValueOf(t)
	for i, f := range reflect.VisibleFields(v.Type()) {
		if d := v.Field(i).Interface().(time.Duration); d <= 0 {
			errs = append(errs, fmt.Errorf("cache.ttl.%s: must be positive, got %v", yamlName(f), d))
		}
	}
	return errs
}

func (m Memory) validate() []error {
	var errs []error
	if m.Entries < 1 {
		errs = append(errs, fmt.Errorf("cache.memory.entries: must be at least 1, got %d", m.Entries))
	}
	for _, s := range []struct {
		name string
		size Size
	}{{"files", m.Files}, {"trees", m.Trees}, {"diffs", m.Diffs}, {"logs", m.Logs}} {
		if s.size <= 0 {
			errs = append(errs, fmt.Errorf("cache.memory.%s: must be positive, got %v", s.name, s.size))
		}
	}
	return errs
}

// ttlKinds are the kinds of data that the single TTL of older releases,
// cache.ttl, covered. The others had TTLs of their own, and longerKinds
// took the longer of theirs and cache.ttl.
var (
	ttlKinds    = []string{"pulls", "issues", "notifications", "repos", "waiting_on_you", "files", "history", "actions", "filters"}
	longerKinds = []string{"repo_info", "profile", "contributions", "dashboard_repos"}
)

// cacheRenames are the settings of the cache that moved.
var cacheRenames = []rename{
	{
		old:  []string{"cache.ttl"},
		new:  prefixed("cache.ttl.", slices.Concat(ttlKinds, longerKinds)),
		note: "each kind of data has its own TTL now",
		move: func(old map[string]*yaml.Node) (map[string]*yaml.Node, error) {
			n := old["cache.ttl"]
			out := map[string]*yaml.Node{}
			for _, k := range ttlKinds {
				out["cache.ttl."+k] = n
			}
			// A value that isn't a duration is refused as one of the
			// kinds above.
			if d, err := time.ParseDuration(n.Value); err == nil {
				for _, k := range longerKinds {
					if d > defaultTTL(k) {
						out["cache.ttl."+k] = n
					}
				}
			}
			return out, nil
		},
	},
	renameTo("cache.revalidate.budget", "cache.revalidate.per_minute"),
}

// defaultTTL returns the default of cache.ttl.<kind>.
func defaultTTL(kind string) time.Duration {
	v := reflect.ValueOf(Default().Cache.TTL)
	for i, f := range reflect.VisibleFields(v.Type()) {
		if yamlName(f) == kind {
			return v.Field(i).Interface().(time.Duration)
		}
	}
	return 0
}

// prefixed returns each of names after prefix.
func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
