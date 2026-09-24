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

// ViewerContributions returns the signed-in user's contribution calendar
// for the past year, week by week.
func (c *Client) ViewerContributions(ctx context.Context) (core.Contributions, error) {
	var data struct {
		Viewer struct {
			ContributionsCollection struct {
				ContributionCalendar struct {
					TotalContributions int `json:"totalContributions"`
					Weeks              []struct {
						Days []contributionDay `json:"contributionDays"`
					} `json:"weeks"`
				} `json:"contributionCalendar"`
			} `json:"contributionsCollection"`
		} `json:"viewer"`
	}
	if err := c.Query(ctx, viewerContributionsQuery, nil, &data); err != nil {
		return core.Contributions{}, fmt.Errorf("viewer contributions: %w", err)
	}
	cal := data.Viewer.ContributionsCollection.ContributionCalendar
	out := core.Contributions{Total: cal.TotalContributions, Weeks: make([][]core.ContributionDay, len(cal.Weeks))}
	for i, w := range cal.Weeks {
		days := make([]core.ContributionDay, len(w.Days))
		for j, d := range w.Days {
			day, err := d.core()
			if err != nil {
				return core.Contributions{}, fmt.Errorf("viewer contributions: %w", err)
			}
			days[j] = day
		}
		out.Weeks[i] = days
	}
	return out, nil
}
