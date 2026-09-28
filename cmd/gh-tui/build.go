package main

import (
	"context"
	"fmt"
	"io"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cmdhist"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	accesssvc "github.com/eggzec/gh-tui/internal/service/access"
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
// into the app. The app talks to the host startRepos picks from hostname,
// the value of --hostname, if set, and shows logWarning, if any, once it
// starts.
func build(ctx context.Context, cfg config.Config, hostname, logWarning string) (*tui.Model, error) {
	pinned, err := parseRefs(cfg.Repos)
	if err != nil {
		return nil, err
	}
	st := startRepos(hostname, currentRepo, defaultHost)
	here := st.Here
	// The sync engine delivers the changes that its polls find, those
	// that the revalidator finds, and those of the rate limits, through
	// one subscription.
	engine := watch.New(watch.WithInterval(cfg.Sync.Interval))
	// The session talks to one host, so the pinned repositories are on it
	// too. The access service learns what the token may do from the
	// client's answers, and reads the token again from where it was found
	// after a refresh.
	token := findToken(st.Host)
	access := accesssvc.New(st.Host, token, accesssvc.WithLookup(findToken), accesssvc.WithChecks(cfg.Auth.Check))
	client, err := github.New(github.WithHost(st.Host), github.WithTokenSource(token.Value, token.Source),
		github.WithOnAccess(access.Set),
		github.WithRateNotify(func() { engine.Publish(core.SyncRateLimit) }))
	if err != nil {
		return nil, err
	}
	context.AfterFunc(ctx, client.Close)
	access.Bind(client)
	access.Start(ctx)

	ttl := cfg.Cache.TTL
	// What an account kept under the name of its token, before accounts
	// were named by their login, stays its own.
	moveAccount(cfg.Cache.Disk, client.Host(), client.TokenAccount(), client.Account())
	store, warning := openDisk(ctx, cfg.Cache.Disk, client.Host())
	// A nil store must stay a nil interface, which the services take for
	// none.
	var entries cache.Store
	if e := openEntries(cfg.Cache.Disk, store, client.Account()); e != nil {
		entries = e
	}
	// The services ask the access service before a change, and before
	// the notifications, which only some tokens may read, so what the
	// token may not do makes no request. Whether a repository is private,
	// which a change there needs a wider scope for, is what the
	// repositories service read of it.
	repoSvc := reposvc.New(client, reposvc.WithTTL(ttl), reposvc.WithStore(entries), reposvc.WithAccess(access))
	pullSvc := pullsvc.New(client, pullsvc.WithTTL(ttl), pullsvc.WithStore(entries),
		pullsvc.WithAccess(access), pullsvc.WithRepos(repoSvc))
	// The issues service tells what a number is, an issue or a pull
	// request, and a pull request whose detail is cached needs no request.
	issueSvc := issuesvc.New(client, issuesvc.WithTTL(ttl), issuesvc.WithStore(entries), issuesvc.WithPulls(pullSvc),
		issuesvc.WithAccess(access), issuesvc.WithRepos(repoSvc))
	notifSvc := notifsvc.New(client, notifsvc.WithTTL(ttl), notifsvc.WithStore(entries), notifsvc.WithAccess(access))
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
	actionSvc := actionssvc.New(client, actionssvc.WithTTL(ttl), actionssvc.WithStore(entries), actionssvc.WithAccess(access))
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
	// What the set command changes for the session, the modals read as
	// they open.
	live := &session{cfg: cfg}
	// Links go to the pages of the session's host.
	webHost := client.WebHost()
	icons := ui.NewIcons(cfg.UI.Icons)
	// What went wrong names the configured keys, and the log file while
	// the app logs to one.
	var logPath string
	if logWarning == "" {
		logPath, _ = cfg.Log.Path()
	}
	voice := ui.NewVoice(cfg.Keys, logPath)
	// The gates and the words of what went wrong follow what the token
	// may do, and point to the command that grants it more.
	voice.Token = ui.NewToken(access, cfg.Keys)
	fileOpts := []files.Option{
		files.WithOffline(offline), files.WithIcons(icons), files.WithFinderPreview(cfg.Files.Finder.Preview),
		files.WithHost(webHost), files.WithVoice(voice), files.WithEditor(cfg.Editor),
	}
	if p := cfg.Files.Prefetch; p.Enabled {
		fileOpts = append(fileOpts,
			files.WithPrefetch(int64(p.MaxSize)),
			// The file under the cursor is likely opened next, so it is
			// read up to the size the preview reads.
			files.WithHoverPrefetch(p.HoverDelay, int64(cfg.Files.Preview.MaxSize)),
		)
	}
	checkOpts := []checks.Option{checks.WithVoice(voice)}
	if cfg.Sync.Enabled {
		checkOpts = append(checkOpts,
			checks.WithWatch(watchChecks(engine.Subscribe, engine.Refresh, actionSvc.PollChecks)),
			checks.WithFollow(checks.Follow(followRuns(engine.Subscribe, engine.Refresh, actionSvc.Poll))))
	}
	// The login of the viewer, from the header the dashboard read.
	viewer := viewerLogin(dashSvc.CachedHeader, func(ctx context.Context) (core.Header, error) {
		// A kept header names the viewer as well as a fresh one.
		return dashSvc.Header(ctx, dashsvc.HeaderQuery{})
	})
	var (
		pullOpts = []pulls.Option{
			pulls.WithOffline(offline), pulls.WithVoice(voice), pulls.WithIcons(icons), pulls.WithFacets(facetSvc),
			pulls.WithChecks(actionSvc, checkOpts...), pulls.WithRepos(repoSvc),
		}
		issueOpts = []issues.Option{
			issues.WithOffline(offline), issues.WithVoice(voice), issues.WithIcons(icons), issues.WithFacets(facetSvc),
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
	// repository screen do, with one opener: whichever is on view reads
	// ahead, so one rate limit stops both and leaving the screen stops the
	// reads.
	threadOpts := []threads.Option{
		threads.WithPulls(pullSvc), threads.WithIssues(issueSvc), threads.WithReleases(releaseSvc),
		threads.WithMarkRead(cfg.Notifications.MarkReadOnOpen),
	}
	if p := cfg.Details.Prefetch; p.Enabled {
		threadOpts = append(threadOpts, threads.WithPrefetch(p.Rows, p.HoverDelay))
	}
	opener := threads.New(ctx, threadOpts...)
	dashOpts := []dashboard.Option{
		dashboard.WithOffline(offline),
		dashboard.WithVoice(voice),
		dashboard.WithInbox(notifSvc),
		dashboard.WithOpener(opener),
		dashboard.WithHere(here, repoSvc.Get),
		dashboard.WithGlyph(cfg.Dashboard.CalendarGlyph),
		dashboard.WithContributions(cfg.Dashboard.ContributionDays()),
		dashboard.WithIcons(icons),
		dashboard.WithHost(webHost),
		// So that the set command may turn reading ahead on.
		dashboard.WithDetails(pullSvc, issueSvc),
	}
	searchOpts := []searchpage.Option{
		searchpage.WithStart(searchStart(repoSvc, pinned)), searchpage.WithIcons(icons), searchpage.WithHost(webHost), searchpage.WithVoice(voice),
		searchpage.WithDetails(pullSvc, issueSvc),
	}
	if p := cfg.Details.Prefetch; p.Enabled {
		// A result is read once the cursor rests on it, as a row of a
		// list is; the first results are a guess, and not read.
		searchOpts = append(searchOpts, searchpage.WithPrefetch(pullSvc, issueSvc, p.HoverDelay))
	}
	if cfg.DashboardPrefetch() {
		// The work waiting on the viewer is what they open most from the
		// dashboard, as quickly as from the lists of a repository, and the
		// cursor rests as it does there.
		dashOpts = append(dashOpts, dashboard.WithPrefetch(pullSvc, issueSvc, cfg.Details.Prefetch.HoverDelay))
	}
	layout := tui.Layout{
		Files:  files.New(ctx, fileSvc, cfg.Keys, fileOpts...),
		Pulls:  pulls.New(ctx, pullSvc, cfg.Keys, pullOpts...),
		Issues: issues.New(ctx, issueSvc, cfg.Keys, issueOpts...),
		Notifications: notifications.New(ctx, notifSvc, cfg.Keys,
			notifications.WithOffline(offline), notifications.WithVoice(voice), notifications.WithOpener(opener)),
		Search:    searchpage.New(ctx, searchSvc, cfg.Keys, searchOpts...),
		Dashboard: dashboard.New(ctx, dashSvc, cfg.Keys, dashOpts...),
	}

	// The history reads the settings of the session, which the set
	// command may have changed since the start, each time it opens.
	historyOpts := func(c config.Config) []history.Option {
		return []history.Option{
			history.WithConfig(c.History), history.WithOffline(offline), history.WithHost(webHost), history.WithVoice(voice),
			history.WithEditor(c.Editor),
		}
	}
	b := browser.New("", io.Discard, io.Discard)
	opts := []tui.Option{
		tui.WithBrowser(b.Browse),
		tui.WithRepoInfo(repoSvc.Get),
		// goto opens a repository only once it is known to exist, a number
		// once it knows whether it is an issue or a pull request, and links
		// to the user's host.
		tui.WithRepos(repoSvc),
		tui.WithKinds(issueSvc),
		tui.WithRecall(recall{pinned: pinned, here: here, dash: dashSvc, repos: repoSvc, pulls: pullSvc, issues: issueSvc}),
		tui.WithHost(webHost),
		tui.WithVoice(voice),
		tui.WithHistory(func(ctx context.Context, repo core.RepoRef, defaultBranch string, base ui.BaseMsg) (ui.Modal, tea.Cmd) {
			return history.Opener(historySvc, cfg.Keys, historyOpts(live.cfg)...)(ctx, repo, defaultBranch, base)
		}),
		tui.WithCommit(func(ctx context.Context, repo core.RepoRef, sha, defaultBranch string) (ui.Modal, tea.Cmd) {
			return history.CommitOpener(historySvc, cfg.Keys, historyOpts(live.cfg)...)(ctx, repo, sha, defaultBranch)
		}),
		tui.WithRelease(releases.Opener(releaseSvc, cfg.Keys, releases.WithVoice(voice))),
		tui.WithRateStatus(client),
		// The status bar names the account gh stores the token for; a
		// token from elsewhere may be anyone's.
		tui.WithLogin(token.Login),
		// The app tells what the token can't do, and :auth grants it more.
		tui.WithAccess(access),
	}
	if path, err := historyPath(cfg.Cache.Disk, client.Host(), client.Account()); err == nil && path != "" {
		opts = append(opts, tui.WithCommandHistory(cmdhist.New(path, cmdhist.DefaultLimit)))
	}
	for _, w := range []string{logWarning, warning} {
		if w != "" {
			opts = append(opts, tui.WithWarning(w))
		}
	}
	actionOpts := []actions.Option{
		actions.WithOffline(offline), actions.WithVoice(voice),
		actions.WithViewer(viewer), actions.WithRepos(repoSvc),
	}
	if cfg.Sync.Enabled {
		actionOpts = append(actionOpts, actions.WithFollow(followRuns(engine.Subscribe, engine.Refresh, actionSvc.Poll)))
	}
	opts = append(opts, tui.WithActions(func(ctx context.Context, repo core.RepoRef, f core.RunFilter) (ui.Modal, tea.Cmd) {
		// The icons are those of the session, which the set command may
		// have changed since the start.
		o := append(slices.Clip(actionOpts), actions.WithIcons(ui.NewIcons(live.cfg.UI.Icons)))
		return actions.Opener(actionSvc, cfg.Keys, o...)(ctx, repo, f)
	}), tui.WithSettings(func(c config.Config) {
		live.set(c)
		engine.SetInterval(c.Sync.Interval)
	}))
	var (
		activity []func(bool)
		watchers []func(core.RepoRef)
	)
	if cfg.Sync.Enabled {
		engine.Subscribe(notifications.SyncKey, notifSvc.Poll)
		// The inbox isn't polled while the token may not read it, and is
		// polled at once when what the token may do changes, so that it
		// catches up as soon as the token may.
		refreshOn(ctx, access.Changes(), engine.Refresh, notifications.SyncKey)
		repoPolls := &repoWatch{subscribe: engine.Subscribe, polls: []repoPoll{
			{key: pullsvc.SyncKey, poll: pullSvc.Poll},
			{key: issuesvc.SyncKey, poll: unless(issuesOff(repoSvc.CachedGet), issueSvc.Poll)},
		}}
		activity = append(activity, engine.SetActive)
		watchers = append(watchers, repoPolls.set)
	}
	if store != nil {
		if r := newRevalidator(cfg.Cache, engine.Publish, issueSvc.Kept, pullSvc.Kept, notifSvc.Kept, fileSvc.Kept, historySvc.Kept, actionSvc.Kept); r != nil {
			go func() { _ = r.Run(ctx) }()
			activity = append(activity, r.SetActive)
			watchers = append(watchers, r.SetRepo)
		}
	}
	// The engine runs with nothing to poll too, for the rate limits.
	go func() { _ = engine.Run(ctx) }()
	opts = append(opts, tui.WithSync(syncEvents(engine)))
	if len(watchers) > 0 {
		opts = append(opts,
			tui.WithActivity(fanOut(activity...)),
			tui.WithRepoWatcher(fanOut(watchers...)),
		)
	}
	return tui.New(ctx, cfg, layout, opts...), nil
}

// findToken finds the token of host the way gh does, with where it
// found it and the account gh stores it for.
func findToken(host string) accesssvc.Token {
	token, source, login := github.FindToken(host)
	return accesssvc.Token{Value: token, Source: source, Login: login}
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
