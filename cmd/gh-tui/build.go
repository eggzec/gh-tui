package main

import (
	"context"
	"fmt"
	"io"

	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	notifsvc "github.com/eggzec/gh-tui/internal/service/notifications"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	"github.com/eggzec/gh-tui/internal/tui"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/repos"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/watch"
)

// build wires the client, the services, the sync engine and the sections
// into the app. arg is the repository named on the command line, if any.
func build(ctx context.Context, cfg config.Config, arg string) (*tui.Model, error) {
	client, err := github.New()
	if err != nil {
		return nil, err
	}

	pinned, err := parseRefs(cfg.Repos)
	if err != nil {
		return nil, err
	}
	repo, err := startRepo(arg, currentRepo, pinned)
	if err != nil {
		return nil, err
	}

	ttl := cfg.Cache.TTL
	pullSvc := pullsvc.New(client, pullsvc.WithTTL(ttl))
	issueSvc := issuesvc.New(client, issuesvc.WithTTL(ttl))
	notifSvc := notifsvc.New(client, notifsvc.WithTTL(ttl))
	repoSvc := reposvc.New(client, reposvc.WithTTL(ttl))

	repoOpts := []repos.Option{repos.WithPinned(pinned)}
	if repo != (core.RepoRef{}) {
		repoOpts = append(repoOpts, repos.WithCurrent(repo))
	}
	sections := []ui.Section{
		pulls.New(ctx, pullSvc, cfg.Keys),
		issues.New(ctx, issueSvc, cfg.Keys),
		notifications.New(ctx, notifSvc, cfg.Keys),
		repos.New(ctx, repoSvc, cfg.Keys, repoOpts...),
	}
	if repo != (core.RepoRef{}) {
		// Sections take a repository before they start, so none of them
		// needs to be running yet.
		for _, s := range sections {
			s.Update(ui.RepoMsg{Repo: repo})
		}
	}

	b := browser.New("", io.Discard, io.Discard)
	opts := []tui.Option{tui.WithBrowser(b.Browse)}
	if cfg.Sync.Enabled {
		engine := watch.New(watch.WithInterval(cfg.Sync.Interval))
		engine.Subscribe(notifications.SyncKey, notifSvc.Poll)
		repoPolls := &repoWatch{subscribe: engine.Subscribe, polls: []repoPoll{
			{key: pullsvc.SyncKey, poll: pullSvc.Poll},
			{key: issuesvc.SyncKey, poll: issueSvc.Poll},
		}}
		repoPolls.set(repo)
		go func() { _ = engine.Run(ctx) }()
		opts = append(opts,
			tui.WithSync(syncEvents(engine)),
			tui.WithActivity(engine.SetActive),
			tui.WithRepoWatcher(repoPolls.set),
		)
	}
	return tui.New(ctx, cfg, sections, opts...), nil
}

// syncEvents turns the engine's events into the app's sync messages.
func syncEvents(e *watch.Engine) func(ctx context.Context) (ui.SyncMsg, bool) {
	events := e.Events()
	return func(ctx context.Context) (ui.SyncMsg, bool) {
		select {
		case ev, ok := <-events:
			if !ok {
				return ui.SyncMsg{}, false
			}
			return ui.SyncMsg{Key: ev.Key, Err: ev.Err}, true
		case <-ctx.Done():
			return ui.SyncMsg{}, false
		}
	}
}

func parseRefs(names []string) ([]core.RepoRef, error) {
	refs := make([]core.RepoRef, 0, len(names))
	for _, name := range names {
		ref, err := core.ParseRepoRef(name)
		if err != nil {
			return nil, fmt.Errorf("pinned repository: %w", err)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
