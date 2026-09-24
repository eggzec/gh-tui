package main

import (
	"context"
	"fmt"
	"io"

	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	notifsvc "github.com/eggzec/gh-tui/internal/service/notifications"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	searchsvc "github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui"
	"github.com/eggzec/gh-tui/internal/tui/files"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/watch"
)

// build wires the client, the services, the sync engine and the sections
// into the app. arg is the repository named on the command line, if any.
// The app opens on that repository, or on the notifications when there is
// none.
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
	fileSvc := filesvc.New(client, filesvc.WithTTL(ttl),
		filesvc.WithMaxBlobSize(int64(cfg.Files.Preview.MaxSize)))
	// Search results keep the search service's own short TTL.
	searchSvc := searchsvc.New(client)

	var fileOpts []files.Option
	if p := cfg.Files.Prefetch; p.Enabled {
		fileOpts = append(fileOpts,
			files.WithPrefetch(int64(p.MaxSize)),
			// The file under the cursor is likely opened next, so it is
			// read up to the size the preview reads.
			files.WithHoverPrefetch(p.HoverDelay, int64(cfg.Files.Preview.MaxSize)),
		)
	}
	layout := tui.Layout{
		Files:         files.New(ctx, fileSvc, cfg.Keys, fileOpts...),
		Pulls:         pulls.New(ctx, pullSvc, cfg.Keys),
		Issues:        issues.New(ctx, issueSvc, cfg.Keys),
		Notifications: notifications.New(ctx, notifSvc, cfg.Keys),
	}

	b := browser.New("", io.Discard, io.Discard)
	opts := []tui.Option{
		tui.WithBrowser(b.Browse),
		tui.WithSearch(searchItems(searchSvc, repoSvc, pinned)),
		tui.WithRepoInfo(repoSvc.Get),
	}
	if repo != (core.RepoRef{}) {
		opts = append(opts, tui.WithRepo(repo))
	}
	if cfg.Sync.Enabled {
		engine := watch.New(watch.WithInterval(cfg.Sync.Interval))
		engine.Subscribe(notifications.SyncKey, notifSvc.Poll)
		repoPolls := &repoWatch{subscribe: engine.Subscribe, polls: []repoPoll{
			{key: pullsvc.SyncKey, poll: pullSvc.Poll},
			{key: issuesvc.SyncKey, poll: issueSvc.Poll},
		}}
		go func() { _ = engine.Run(ctx) }()
		opts = append(opts,
			tui.WithSync(syncEvents(engine)),
			tui.WithActivity(engine.SetActive),
			tui.WithRepoWatcher(repoPolls.set),
		)
	}
	return tui.New(ctx, cfg, layout, opts...), nil
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
