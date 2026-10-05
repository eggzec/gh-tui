package owner

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
)

// sideFake serves what the panes beside the list read, by the logins in
// lower case: octocat's README, calendar and sponsors, and github's
// README and follower count.
type sideFake struct {
	mu        sync.Mutex
	readmes   map[string]core.Readme
	contribs  map[string]core.Contributions
	sponsors  map[string]core.Page[core.Person]
	followers map[string]int
	// fail, when set, fails the read of that kind: readme, contributions,
	// sponsors or followers.
	fail map[string]error
	// read holds what was read and is fresh, as "readme octocat".
	read  map[string]bool
	calls []string
}

func newSideFake() *sideFake {
	return &sideFake{
		readmes: map[string]core.Readme{
			"octocat": {Markdown: octocatReadme, Source: core.RepoRef{Owner: "octocat", Name: "octocat"}},
			"github":  {Markdown: githubReadme, Source: core.RepoRef{Owner: "github", Name: ".github"}},
		},
		contribs: map[string]core.Contributions{"octocat": contributions()},
		sponsors: map[string]core.Page[core.Person]{
			"sponsors octocat":   {Items: []core.Person{{Login: "mona"}, {Login: "hubot"}, {Login: "mona_corp"}}},
			"sponsoring octocat": {Items: []core.Person{{Login: "charmbracelet", Kind: core.OwnerOrg}}, Next: "x"},
		},
		followers: map[string]int{"github": 41200},
		fail:      map[string]error{},
		read:      map[string]bool{},
	}
}

const octocatReadme = `# Hi, I'm Mona

I build tools for the terminal: TUIs, gh extensions and the odd shell script.

- Working on [gh-tui](https://github.com/eggzec/gh-tui) and bubbletea
- My [notes](notes/README.md) and [talks](/talks)

> Talk is cheap. Show me the code.

![stats](https://github-readme-stats.example/api?username=octocat)
`

const githubReadme = `## Welcome to GitHub

We build the home for all developers.

<img src="banner.png" alt="banner">
`

// contributions returns a year of weeks, from Sunday, with counts that
// repeat so the levels vary.
func contributions() core.Contributions {
	start := time.Date(2025, 9, 21, 0, 0, 0, 0, time.UTC)
	var c core.Contributions
	for w := range 53 {
		var week []core.ContributionDay
		for d := range 7 {
			day := start.AddDate(0, 0, w*7+d)
			if day.After(now) {
				break
			}
			n := (w*3 + d*5) % 9
			week = append(week, core.ContributionDay{Date: day, Count: n, Level: min(n/2, 4)})
			c.Total += n
		}
		c.Weeks = append(c.Weeks, week)
	}
	return c
}

// served notes a read of what, by login, and returns the error it fails
// with, if any.
func (f *sideFake) served(what, login string) error {
	f.calls = append(f.calls, what+" "+strings.ToLower(login))
	if err := f.fail[what]; err != nil {
		return err
	}
	f.read[what+" "+strings.ToLower(login)] = true
	return nil
}

func (f *sideFake) fresh(what, login string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read[what+" "+strings.ToLower(login)]
}

func (f *sideFake) FreshReadme(q owners.ReadmeQuery) bool { return f.fresh("readme", q.Login) }

func (f *sideFake) Readme(_ context.Context, q owners.ReadmeQuery) (owners.Readme, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.served("readme", q.Login); err != nil {
		return owners.Readme{}, err
	}
	return owners.Readme{Readme: f.readmes[strings.ToLower(q.Login)]}, nil
}

func (f *sideFake) FreshContributions(login string) bool { return f.fresh("contributions", login) }

func (f *sideFake) Contributions(_ context.Context, q owners.ContributionsQuery) (core.Contributions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.served("contributions", q.Login); err != nil {
		return core.Contributions{}, err
	}
	return f.contribs[strings.ToLower(q.Login)], nil
}

func (f *sideFake) FreshOrgFollowers(login string) bool { return f.fresh("followers", login) }

func (f *sideFake) OrgFollowers(_ context.Context, q owners.OrgFollowersQuery) (owners.FollowerCount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.served("followers", q.Login); err != nil {
		return owners.FollowerCount{}, err
	}
	return owners.FollowerCount{Count: f.followers[strings.ToLower(q.Login)]}, nil
}

func (f *sideFake) FreshPeople(q owners.PeopleQuery) bool {
	return f.fresh(q.List.String(), q.Login)
}

func (f *sideFake) People(_ context.Context, q owners.PeopleQuery) (core.Page[core.Person], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	what := q.List.String()
	if err := f.fail["sponsors"]; err != nil && (q.List == owners.Sponsors || q.List == owners.Sponsoring) {
		f.calls = append(f.calls, what+" "+strings.ToLower(q.Login))
		return core.Page[core.Person]{}, err
	}
	if err := f.served(what, q.Login); err != nil {
		return core.Page[core.Person]{}, err
	}
	return f.sponsors[what+" "+strings.ToLower(q.Login)], nil
}

// sponsorList reports whether l is one of the lists of sponsors, which
// the side reads.
func sponsorList(l owners.PeopleList) bool {
	return l == owners.Sponsors || l == owners.Sponsoring
}
