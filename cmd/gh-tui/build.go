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
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	dashsvc "github.com/eggzec/gh-tui/internal/service/dashboard"
	facetsvc "github.com/eggzec/gh-tui/internal/service/facets"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	notifsvc "github.com/eggzec/gh-tui/internal/service/notifications"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	releasesvc "github.com/eggzec/gh-tui/internal/service/releases"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	searchsvc "github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui"
	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/files"
	"github.com/eggzec/gh-tui/internal/tui/history"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/releases"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/watch"
)

// build wires the client, the services, the sync engine and the sections
// into the app. arg is the repository named on the command line, if any.
// The app opens on that repository, or on the dashboard when there is
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
	repo, here, err := startRepos(arg, currentRepo)
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
	// The issues service tells what a number is, an issue or a pull
	// request, and a pull request whose detail is cached needs no request.
	issueSvc := issuesvc.New(client, issuesvc.WithTTL(ttl), issuesvc.WithStore(entries), issuesvc.WithPulls(pullSvc))
	notifSvc := notifsvc.New(client, notifsvc.WithTTL(ttl), notifsvc.WithStore(entries))
	repoSvc := reposvc.New(client, reposvc.WithTTL(ttl), reposvc.WithStore(entries))
	dashSvc := dashsvc.New(client, dashsvc.WithTTL(ttl), dashsvc.WithStore(entries))
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
	actionSvc := actionssvc.New(client, actionssvc.WithTTL(ttl), actionssvc.WithStore(entries))
	// A release seldom changes once published, so it keeps the service's
	// long TTL.
	releaseSvc := releasesvc.New(client, releasesvc.WithStore(entries))
	// Search results keep the search service's own short TTL.
	searchSvc := searchsvc.New(client)
	// The filters of the pull requests and issues offer the labels,
	// milestones and people of the repository.
	facetSvc := facetsvc.New(client, facetsvc.WithTTL(ttl))

	// The sections share whether GitHub can't be reached, so the user is
	// told once.
	offline := new(ui.Offline)
	icons := ui.NewIcons(cfg.UI.Icons)
	fileOpts := []files.Option{files.WithOffline(offline), files.WithFinderPreview(cfg.Files.Finder.Preview)}
	if p := cfg.Files.Prefetch; p.Enabled {
		fileOpts = append(fileOpts,
			files.WithPrefetch(int64(p.MaxSize)),
			// The file under the cursor is likely opened next, so it is
			// read up to the size the preview reads.
			files.WithHoverPrefetch(p.HoverDelay, int64(cfg.Files.Preview.MaxSize)),
		)
	}
	// The sync engine delivers the changes that its polls find, and those
	// that the revalidator finds, through one subscription.
	engine := watch.New(watch.WithInterval(cfg.Sync.Interval))
	var checkOpts []checks.Option
	if cfg.Sync.Enabled {
		checkOpts = append(checkOpts,
			checks.WithWatch(watchChecks(engine.Subscribe, engine.Refresh, actionSvc.PollChecks)),
			checks.WithFollow(checks.Follow(followRuns(engine.Subscribe, engine.Refresh, actionSvc.Poll))))
	}
	// The login of the viewer, from the header the dashboard read.
	viewer := viewerLogin(dashSvc.CachedHeader, dashSvc.Header)
	var (
		pullOpts = []pulls.Option{
			pulls.WithOffline(offline), pulls.WithIcons(icons), pulls.WithFacets(facetSvc),
			pulls.WithChecks(actionSvc, checkOpts...), pulls.WithRepos(repoSvc),
		}
		issueOpts = []issues.Option{
			issues.WithOffline(offline), issues.WithIcons(icons), issues.WithFacets(facetSvc),
			issues.WithRepos(repoSvc), issues.WithViewer(issues.Viewer(viewer)),
		}
	)
	if p := cfg.Details.Prefetch; p.Enabled {
		pullOpts = append(pullOpts, pulls.WithPrefetch(p.Rows, p.HoverDelay))
		issueOpts = append(issueOpts, issues.WithPrefetch(p.Rows, p.HoverDelay))
		if p.Filters {
			pullOpts = append(pullOpts, pulls.WithFilterPrefetch())
			issueOpts = append(issueOpts, issues.WithFilterPrefetch())
		}
	}
	// The notifications screen and the dashboard's inbox open what each
	// thread is about in its modal, and read it ahead as the lists of the
	// repository screen do, each with an opener of its own.
	threadOpts := []threads.Option{
		threads.WithPulls(pullSvc), threads.WithIssues(issueSvc), threads.WithReleases(releaseSvc),
		threads.WithMarkRead(cfg.Notifications.MarkReadOnOpen),
	}
	if p := cfg.Details.Prefetch; p.Enabled {
		threadOpts = append(threadOpts, threads.WithPrefetch(p.Rows, p.HoverDelay))
	}
	layout := tui.Layout{
		Files:  files.New(ctx, fileSvc, cfg.Keys, fileOpts...),
		Pulls:  pulls.New(ctx, pullSvc, cfg.Keys, pullOpts...),
		Issues: issues.New(ctx, issueSvc, cfg.Keys, issueOpts...),
		Notifications: notifications.New(ctx, notifSvc, cfg.Keys,
			notifications.WithOffline(offline), notifications.WithOpener(threads.New(ctx, threadOpts...))),
		Search: searchpage.New(ctx, searchSvc, cfg.Keys, searchpage.WithStart(searchStart(repoSvc, pinned)), searchpage.WithIcons(icons)),
		Dashboard: dashboard.New(ctx, dashSvc, cfg.Keys,
			dashboard.WithOffline(offline),
			dashboard.WithInbox(notifSvc),
			dashboard.WithOpener(threads.New(ctx, threadOpts...)),
			dashboard.WithHere(here, repoSvc.Get),
			dashboard.WithGlyph(cfg.Dashboard.CalendarGlyph),
			dashboard.WithContributions(cfg.Dashboard.ContributionDays()),
			dashboard.WithIcons(icons),
		),
	}

	b := browser.New("", io.Discard, io.Discard)
	opts := []tui.Option{
		tui.WithBrowser(b.Browse),
		tui.WithRepoInfo(repoSvc.Get),
		tui.WithHistory(history.Opener(historySvc, cfg.Keys,
			history.WithConfig(cfg.History), history.WithOffline(offline))),
		tui.WithCommit(history.CommitOpener(historySvc, cfg.Keys,
			history.WithConfig(cfg.History), history.WithOffline(offline))),
		tui.WithRelease(releases.Opener(releaseSvc, cfg.Keys)),
	}
	if repo != (core.RepoRef{}) {
		opts = append(opts, tui.WithRepo(repo))
	}
	for _, w := range []string{logWarning, warning} {
		if w != "" {
			opts = append(opts, tui.WithWarning(w))
		}
	}
	actionOpts := []actions.Option{
		actions.WithOffline(offline), actions.WithIcons(icons),
		actions.WithViewer(viewer), actions.WithRepos(repoSvc),
	}
	if cfg.Sync.Enabled {
		actionOpts = append(actionOpts, actions.WithFollow(followRuns(engine.Subscribe, engine.Refresh, actionSvc.Poll)))
	}
	opts = append(opts, tui.WithActions(actions.Opener(actionSvc, cfg.Keys, actionOpts...)))
	var (
		activity []func(bool)
		watchers []func(core.RepoRef)
	)
	if cfg.Sync.Enabled {
		engine.Subscribe(notifications.SyncKey, notifSvc.Poll)
		repoPolls := &repoWatch{subscribe: engine.Subscribe, polls: []repoPoll{
			{key: pullsvc.SyncKey, poll: pullSvc.Poll},
			{key: issuesvc.SyncKey, poll: unless(issuesOff(repoSvc.CachedGet), issueSvc.Poll)},
		}}
		activity = append(activity, engine.SetActive)
		watchers = append(watchers, repoPolls.set)
	}
	if store != nil {
		if r := newRevalidator(cfg.Cache, engine.Publish, issueSvc.Kept, notifSvc.Kept, fileSvc.Kept, historySvc.Kept, actionSvc.Kept); r != nil {
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
