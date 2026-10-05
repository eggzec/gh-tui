package github

import (
	"context"
	"fmt"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// viewerContributionsQuery reads the calendar of the past year, which is
// what contributionsCollection covers when it is given no dates.
const viewerContributionsQuery = `query ViewerContributions {
  ` + rateLimitField + `
  viewer {
    contributionsCollection {
      contributionCalendar {
        totalContributions
        weeks { contributionDays { date contributionCount contributionLevel } }
      }
    }
  }
}`

type contributionDay struct {
	// Date is a GraphQL Date, such as 2026-09-24, which time.Time can't
	// decode.
	Date  string `json:"date"`
	Count int    `json:"contributionCount"`
	Level string `json:"contributionLevel"`
}

// contributionLevels ranks GitHub's contribution levels.
var contributionLevels = map[string]int{
	"NONE":            0,
	"FIRST_QUARTILE":  1,
	"SECOND_QUARTILE": 2,
	"THIRD_QUARTILE":  3,
	"FOURTH_QUARTILE": 4,
}

func (d contributionDay) core() (core.ContributionDay, error) {
	date, err := time.Parse(time.DateOnly, d.Date)
	if err != nil {
		return core.ContributionDay{}, fmt.Errorf("decode contribution date: %w", err)
	}
	// An unknown level reads as none rather than failing the calendar.
	return core.ContributionDay{Date: date, Count: d.Count, Level: contributionLevels[d.Level]}, nil
}

// contributionsCollection is the contributionsCollection field of a
// calendar query.
type contributionsCollection struct {
	ContributionCalendar struct {
		TotalContributions int `json:"totalContributions"`
		Weeks              []struct {
			Days []contributionDay `json:"contributionDays"`
		} `json:"weeks"`
	} `json:"contributionCalendar"`
}

func (cc *contributionsCollection) core() (core.Contributions, error) {
	cal := cc.ContributionCalendar
	out := core.Contributions{Total: cal.TotalContributions, Weeks: make([][]core.ContributionDay, len(cal.Weeks))}
	for i, w := range cal.Weeks {
		days := make([]core.ContributionDay, len(w.Days))
		for j, d := range w.Days {
			day, err := d.core()
			if err != nil {
				return core.Contributions{}, err
			}
			days[j] = day
		}
		out.Weeks[i] = days
	}
	return out, nil
}

// ViewerContributions returns the signed-in user's contribution calendar
// for the past year, week by week.
func (c *Client) ViewerContributions(ctx context.Context) (core.Contributions, error) {
	var data struct {
		Viewer struct {
			ContributionsCollection contributionsCollection `json:"contributionsCollection"`
		} `json:"viewer"`
	}
	if err := c.Query(ctx, viewerContributionsQuery, nil, &data); err != nil {
		return core.Contributions{}, fmt.Errorf("viewer contributions: %w", err)
	}
	out, err := data.Viewer.ContributionsCollection.core()
	if err != nil {
		return core.Contributions{}, fmt.Errorf("viewer contributions: %w", err)
	}
	return out, nil
}

// userContributionsQuery reads a user's calendar of the past year, as
// viewerContributionsQuery does the viewer's. Private contributions count
// as the user's profile settings say.
const userContributionsQuery = `query UserContributions($login: String!) {
  ` + rateLimitField + `
  user(login: $login) {
    contributionsCollection {
      contributionCalendar {
        totalContributions
        weeks { contributionDays { date contributionCount contributionLevel } }
      }
    }
  }
}`

// UserContributions returns the contribution calendar of the user login
// for the past year, week by week. It returns an error matching
// core.ErrNotFound if there is no such user.
func (c *Client) UserContributions(ctx context.Context, login string) (core.Contributions, error) {
	var data struct {
		User *struct {
			ContributionsCollection contributionsCollection `json:"contributionsCollection"`
		} `json:"user"`
	}
	if err := c.Query(ctx, userContributionsQuery, map[string]any{"login": login}, &data); err != nil {
		return core.Contributions{}, fmt.Errorf("contributions of %s: %w", login, err)
	}
	if data.User == nil {
		return core.Contributions{}, fmt.Errorf("contributions of %s: %w", login, core.ErrNotFound)
	}
	out, err := data.User.ContributionsCollection.core()
	if err != nil {
		return core.Contributions{}, fmt.Errorf("contributions of %s: %w", login, err)
	}
	return out, nil
}
