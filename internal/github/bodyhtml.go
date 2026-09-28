package github

import (
	"context"
	"fmt"
)

// maxBodyHTML is how many bodies one request reads the HTML of: as many
// nodes as GraphQL takes at once, for one point of the rate limit.
const maxBodyHTML = 100

// bodyHTMLQuery reads the HTML GitHub renders of bodies by their node IDs:
// issues, pull requests, reviews and comments all implement Comment.
// Images in it are at the addresses GitHub serves them at: those of a
// private repository signed for a few minutes, and those of other hosts
// through its proxy.
const bodyHTMLQuery = `query BodyHTML($ids: [ID!]!) {
  ` + rateLimitField + `
  nodes(ids: $ids) {
    ... on Comment { id bodyHTML }
  }
}
`

type bodyHTMLNode struct {
	ID       string `json:"id"`
	BodyHTML string `json:"bodyHTML"`
}

// BodyHTML returns the rendered HTML of the bodies named by their node
// IDs, by ID, in one request for each hundred of them. A node that isn't
// a body, or that GitHub doesn't show the token, is left out.
func (c *Client) BodyHTML(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for start := 0; start < len(ids); start += maxBodyHTML {
		var data struct {
			Nodes []*bodyHTMLNode `json:"nodes"`
		}
		vars := map[string]any{"ids": ids[start:min(start+maxBodyHTML, len(ids))]}
		if err := c.Query(ctx, bodyHTMLQuery, vars, &data); err != nil {
			return nil, fmt.Errorf("body html: %w", err)
		}
		for _, n := range data.Nodes {
			if n != nil && n.ID != "" {
				out[n.ID] = n.BodyHTML
			}
		}
	}
	return out, nil
}
