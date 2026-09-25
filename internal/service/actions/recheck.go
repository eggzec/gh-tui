package actions

import (
	"context"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// Kept lists the first pages of runs and the workflows that the service
// keeps with validators, for a revalidator to check in the background.
// Each check is one conditional request, which costs no rate limit when
// nothing moved. What changed is cached and kept, and reports SyncKey of
// its repository. The jobs and logs kept for good never change, so they
// aren't listed. It reads the store, so call it where I/O is fine.
func (s *Service) Kept() []revalidate.Entry {
	return slices.Concat(
		recheck.Entries(s.keptRuns, kindRuns, s.runsTarget),
		recheck.Entries(s.keptWorkflows, kindWorkflows, s.workflowsTarget),
	)
}

func (s *Service) runsTarget(key string) (recheck.Target, bool) {
	q, ok := parseRunsKey(key)
	if !ok || q.Cursor != "" {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(s.loadRuns(q), func(core.Page[core.Run]) []string { return []string{repoTag(q.Repo)} })
		_, res := recheck.Check(ctx, s.runs, s.keptRuns, key, SyncKey(q.Repo), load)
		return res
	}}, true
}

func (s *Service) workflowsTarget(key string) (recheck.Target, bool) {
	repo, err := core.ParseRepoRef(strings.TrimPrefix(key, "workflows:"))
	if err != nil || !strings.HasPrefix(key, "workflows:") {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(s.loadWorkflows(repo), func(core.Page[core.Workflow]) []string { return []string{repoTag(repo)} })
		_, res := recheck.Check(ctx, s.workflows, s.keptWorkflows, key, SyncKey(repo), load)
		return res
	}}, true
}
