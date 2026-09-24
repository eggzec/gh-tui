package main

import (
	"context"
	"fmt"
	"io"

	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	notifsvc "github.com/eggzec/gh-tui/internal/service/notifications"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	searchsvc "github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui"
	"github.com/eggzec/gh-tui/internal/tui/files"
	"github.com/eggzec/gh-tui/internal/tui/history"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/watch"
)

// build wires the client, the services, the sync engine and the sections
// into the app. arg is the repository named on the command line, if any.
// The app opens on that repository, or on the notifications when there is
// none. The app shows logWarning, if any, once it starts.
func build(ctx context.Context, cfg config.Config, arg, logWarning string) (*tui.Model, error) {
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
	store, warning := openDisk(ctx, cfg.Cache.Disk, client.Host())
	// A nil store must stay a nil interface, which the services take for
	// none.
	var entries cache.Store
	if e := openEntries(cfg.Cache.Disk, store, client.Account()); e != nil {
		entries = e
	}
	pullSvc := pullsvc.New(client, pullsvc.WithTTL(ttl), pullsvc.WithStore(entries))
	issueSvc := issuesvc.New(client, issuesvc.WithTTL(ttl), issuesvc.WithStore(entries))
	notifSvc := notifsvc.New(client, notifsvc.WithTTL(ttl), notifsvc.WithStore(entries))
	repoSvc := reposvc.New(client, reposvc.WithTTL(ttl), reposvc.WithStore(entries))
	fileSvcOpts := []filesvc.Option{filesvc.WithTTL(ttl), filesvc.WithMaxBlobSize(int64(cfg.Files.Preview.MaxSize))}
	if store != nil {
		fileSvcOpts = append(fileSvcOpts, filesvc.WithStore(store))
	}
	fileSvc := filesvc.New(client, fileSvcOpts...)
	// What a commit SHA names never changes, so it is kept with the files'
	// objects, which accounts on a host share.
	historySvcOpts := []historysvc.Option{historysvc.WithTTL(ttl), historysvc.WithStore(entries)}
	if store != nil {
		historySvcOpts = append(historySvcOpts, historysvc.WithObjects(store))
	}
	historySvc := historysvc.New(client, historySvcOpts...)
	// Search results keep the search service's own short TTL.
	searchSvc := searchsvc.New(client)

	// The sections share whether GitHub can't be reached, so the user is
	// told once.
	offline := new(ui.Offline)
	fileOpts := []files.Option{files.WithOffline(offline)}
	if p := cfg.Files.Prefetch; p.Enabled {
		fileOpts = append(fileOpts,
			files.WithPrefetch(int64(p.MaxSize)),
			// The file under the cursor is likely opened next, so it is
			// read up to the size the preview reads.
			files.WithHoverPrefetch(p.HoverDelay, int64(cfg.Files.Preview.MaxSize)),
		)
	}
	var (
		pullOpts  = []pulls.Option{pulls.WithOffline(offline)}
		issueOpts = []issues.Option{issues.WithOffline(offline)}
	)
	if p := cfg.Details.Prefetch; p.Enabled {
		pullOpts = append(pullOpts, pulls.WithPrefetch(p.Rows, p.HoverDelay))
		issueOpts = append(issueOpts, issues.WithPrefetch(p.Rows, p.HoverDelay))
	}
	layout := tui.Layout{
		Files:         files.New(ctx, fileSvc, cfg.Keys, fileOpts...),
		Pulls:         pulls.New(ctx, pullSvc, cfg.Keys, pullOpts...),
		Issues:        issues.New(ctx, issueSvc, cfg.Keys, issueOpts...),
		Notifications: notifications.New(ctx, notifSvc, cfg.Keys, notifications.WithOffline(offline)),
	}

	b := browser.New("", io.Discard, io.Discard)
	opts := []tui.Option{
		tui.WithBrowser(b.Browse),
		tui.WithSearch(searchItems(searchSvc, repoSvc, pinned)),
		tui.WithRepoInfo(repoSvc.Get),
		tui.WithHistory(history.Opener(historySvc, cfg.Keys,
			history.WithConfig(cfg.History), history.WithOffline(offline))),
	}
	if repo != (core.RepoRef{}) {
		opts = append(opts, tui.WithRepo(repo))
	}
	for _, w := range []string{logWarning, warning} {
		if w != "" {
			opts = append(opts, tui.WithWarning(w))
		}
	}
	// The sync engine delivers the changes that its polls find, and those
	// that the revalidator finds, through one subscription.
	engine := watch.New(watch.WithInterval(cfg.Sync.Interval))
	var (
		activity []func(bool)
		watchers []func(core.RepoRef)
	)
	if cfg.Sync.Enabled {
		engine.Subscribe(notifications.SyncKey, notifSvc.Poll)
		repoPolls := &repoWatch{subscribe: engine.Subscribe, polls: []repoPoll{
			{key: pullsvc.SyncKey, poll: pullSvc.Poll},
			{key: issuesvc.SyncKey, poll: issueSvc.Poll},
		}}
		activity = append(activity, engine.SetActive)
		watchers = append(watchers, repoPolls.set)
	}
	if store != nil {
		if r := newRevalidator(cfg.Cache, engine.Publish, issueSvc.Kept, notifSvc.Kept, fileSvc.Kept, historySvc.Kept); r != nil {
			go func() { _ = r.Run(ctx) }()
			activity = append(activity, r.SetActive)
			watchers = append(watchers, r.SetRepo)
		}
	}
	if len(watchers) > 0 {
		go func() { _ = engine.Run(ctx) }()
		opts = append(opts,
			tui.WithSync(syncEvents(engine)),
			tui.WithActivity(fanOut(activity...)),
			tui.WithRepoWatcher(fanOut(watchers...)),
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
