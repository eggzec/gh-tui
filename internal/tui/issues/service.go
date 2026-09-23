package issues

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
)

// Service is what the section needs from the issues service.
type Service interface {
	List(ctx context.Context, q issuesvc.ListQuery) (core.Page[core.Issue], error)
}
