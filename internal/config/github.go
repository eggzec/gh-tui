package config

import (
	"errors"
	"fmt"
	"time"
)

// GitHub configures how the app talks to GitHub.
type GitHub struct {
	// Timeout bounds each attempt at a request, from sending it until its
	// answer is read, where the request has no deadline of its own.
	Timeout time.Duration `yaml:"timeout"`
	// Concurrency is how many requests are in flight at once at most.
	// Two of them are kept for what the user waits for, so reads ahead
	// and background checks never hold those up.
	Concurrency int `yaml:"concurrency"`
}

// Bounds of the settings of GitHub. The client keeps two requests in
// flight for what the user waits for, so the least concurrency leaves one
// more for the rest.
const (
	minTimeout     = time.Second
	minConcurrency = 3
	maxConcurrency = 16
)

func (g GitHub) validate() error {
	var errs []error
	if g.Timeout < minTimeout {
		errs = append(errs, fmt.Errorf("github.timeout: must be at least %v, got %v", minTimeout, g.Timeout))
	}
	if g.Concurrency < minConcurrency || g.Concurrency > maxConcurrency {
		errs = append(errs, fmt.Errorf("github.concurrency: must be between %d and %d, got %d", minConcurrency, maxConcurrency, g.Concurrency))
	}
	return errors.Join(errs...)
}

// PageSize is how many items each page read from GitHub holds, by what
// the page lists. Every size is cached apart.
type PageSize struct {
	// Pulls sizes the pages of pull requests, and of their comments and
	// reviews.
	Pulls int `yaml:"pulls"`
	// Issues sizes the pages of issues, and of their comments.
	Issues        int `yaml:"issues"`
	Notifications int `yaml:"notifications"`
	// Repos sizes the pages of the user's repositories.
	Repos int `yaml:"repos"`
	// Runs sizes the pages of workflow runs of the Actions modal.
	Runs int `yaml:"runs"`
	// Commits sizes the pages of commits of the History modal.
	Commits int `yaml:"commits"`
	// Search sizes the pages of each kind of search result.
	Search int `yaml:"search"`
	// WaitingOnYou is how many of each list of the dashboard's Waiting on
	// you are read; the counts cover them all.
	WaitingOnYou int `yaml:"waiting_on_you"`
	// People sizes each list of people on a user or organization page.
	People int `yaml:"people"`
	// References sizes the pages of the items that mention a pull request
	// or issue.
	References int `yaml:"references"`
}

// Bounds of a page size: GitHub serves at most 100 items a page.
const (
	minPageSize = 10
	maxPageSize = 100
)

func (p PageSize) validate() error {
	var errs []error
	for _, s := range []struct {
		key  string
		size int
	}{
		{"pulls", p.Pulls}, {"issues", p.Issues}, {"notifications", p.Notifications}, {"repos", p.Repos},
		{"runs", p.Runs}, {"commits", p.Commits}, {"search", p.Search}, {"waiting_on_you", p.WaitingOnYou},
		{"people", p.People}, {"references", p.References},
	} {
		if s.size < minPageSize || s.size > maxPageSize {
			errs = append(errs, fmt.Errorf("page_size.%s: must be between %d and %d, got %d", s.key, minPageSize, maxPageSize, s.size))
		}
	}
	return errors.Join(errs...)
}
