package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// UserPreviews reads what the preview of a user or an organization shows
// beyond its hit.
type UserPreviews interface {
	Profile(ctx context.Context, login string) (core.UserProfile, error)
}

// userReader returns the read of a user or organization hit's preview.
func userReader(up UserPreviews, h core.SearchHit) func(ctx context.Context) (string, error) {
	if up == nil {
		return nil
	}
	login := h.User.Login
	return func(ctx context.Context) (string, error) {
		p, err := up.Profile(ctx, login)
		if err != nil {
			return "", err
		}
		return userExtra(p), nil
	}
}

// userExtra is what a read of a profile adds under the hit's own text, as
// Markdown.
func userExtra(p core.UserProfile) string {
	var b strings.Builder
	if p.Name != "" {
		fmt.Fprintf(&b, "- Name: %s\n", p.Name)
	}
	fmt.Fprintf(&b, "- Followers: %d\n", p.Followers)
	if !p.CreatedAt.IsZero() {
		fmt.Fprintf(&b, "- Joined: %s\n", p.CreatedAt.Format("2006-01-02"))
	}
	if p.Bio != "" {
		b.WriteString("\n")
		b.WriteString(p.Bio)
		b.WriteString("\n")
	}
	return b.String()
}
