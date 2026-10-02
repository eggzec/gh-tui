package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cmdhist"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/internal/obs"
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
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// build wires the client, the services, the sync engine and the sections
// into the app. The app talks to the host startRepos picks from hostname,
// the value of --hostname, if set, with the config file resolves for that
// host and the account of its token, at logLevel, the level the log was
// opened at, and shows logWarning and configWarning, if any, once it
// starts. The token may still be on its way when build returns, as when
// gh reads it from the system keyring, which takes tens of milliseconds the
// app can start in; found then says whether it came, and is closed or sends
// nil once it did. The account is known before, from gh's hosts file.
func build(ctx context.Context, file *config.File, logLevel, hostname, logWarning, configWarning string) (app *tui.Model, found <-chan error, err error) {
	st := startRepos(hostname, currentRepo, defaultHost)
	logHost(st.Host)
	here := st.Here
	// The session talks to one host, so the pinned repositories are on it
	// too. The access service learns what the token may do from the
	// client's answers, and reads the token again from where it was found
	// after a refresh.
	token, later := quickToken(st.Host)
	// The login of a token from the environment is kept in the disk cache
	// of the host, as the host's settings place it.
	hostCfg, _, err := file.Resolve(st.Host, "")
	if err != nil {
		return nil, nil, fmt.Errorf("config for %s: %w", st.Host, err)
	}
	login := startLogin(hostCfg.Cache.Disk, st.Host, token, time.Now())
	cfg, src, err := sessionConfig(file, st.Host, login.login, logLevel)
	if err != nil {
		return nil, nil, err
	}
	pinned, err := parseRefs(cfg.Repos)
	if err != nil {
		return nil, nil, err
	}
	// The sync engine delivers the changes that its polls find, those
	// that the revalidator finds, and those of the rate limits, through
	// one subscription.
	engine := newEngine(cfg.Sync)
	// Reads ahead may spend only a share of each quota, from the first.
	obs.SetPrefetchBudget(cfg.Prefetch.Budget)
	access := accesssvc.New(st.Host, token, accesssvc.WithLookup(findToken), accesssvc.WithChecks(cfg.Auth.Check))
	// The app tells once of an Enterprise Server older than supported.
	oldEnterprise := make(chan string, 1)
	tokenOpt := github.WithTokenSource(token.Value, token.Source)
	if later {
		tokenOpt = github.WithTokenLater(token.Source)
	}
	client, err := github.New(github.WithHost(st.Host), tokenOpt, github.WithLogin(login.login),
		github.WithHTTPClient(&http.Client{Timeout: cfg.GitHub.Timeout}), github.WithConcurrency(cfg.GitHub.Concurrency),
		github.WithOnAccess(access.Set),
		github.WithOnOldEnterprise(func(v string) { oldEnterprise <- v }),
		github.WithRateNotify(func() { engine.Publish(core.SyncRateLimit) }))
	if err != nil {
		return nil, nil, err
	}
	context.AfterFunc(ctx, client.Close)
	go logGHVersion(ctx, accesssvc.GHPath())
	// The session's record names the token's kind, and comes before the
	// records of any request it sends; what the token may do starts from
	// its kind once the client has it.
	logStart := func(token accesssvc.Token) {
		logSession(newSessionInfo(st, token, login, client, cfg.Cache.Disk))
		// After the session record, so that it carries the host and the
		// account the config was resolved for.
		logConfig(cfg, src)
	}
	start := func() {
		access.Bind(client)
		access.Start(ctx)
	}
	done := make(chan error, 1)
	switch {
	case !later:
		logStart(token)
		start()
		// What an account kept under the name of its token, before
		// accounts were named by their login, stays its own.
		moveAccount(cfg.Cache.Disk, client.Host(), client.TokenAccount(), client.Account())
		close(done)
	case accountKept(cfg.Cache.Disk, client.Host(), client.Account()):
		// Nothing kept is named by the token, so the app starts while gh
		// reads it.
		go func() { done <- authorize(st.Host, client, access, logStart, start) }()
	default:
		// What the account kept may still be named by its token, which
		// moving it needs.
		if err := authorize(st.Host, client, access, logStart, start); err != nil {
			return nil, nil, err
		}
		moveAccount(cfg.Cache.Disk, client.Host(), client.TokenAccount(), client.Account())
		close(done)
	}
	// What applies is settled already; the answer is for the next start.
	// Only a token from the environment is asked about, which is there at
	// once.
	lateWarning := make(chan string, 1)
	go learnLogin(ctx, login, client.UserLogin, func(l string) string {
		_, src, _ := file.Resolve(st.Host, l)
		return src.Profile
	}, func(w string) { lateWarning <- w }, time.Now)

	// Each kind of data stays fresh for its own TTL, and each cache keeps
	// as many entries.
	ttl, mem := cfg.Cache.TTL, cfg.Cache.Memory
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
	size := cfg.PageSize
	repoSvc := reposvc.New(client, reposvc.WithTTL(ttl.Repos), reposvc.WithInfoTTL(ttl.RepoInfo), reposvc.WithCapacity(mem.Entries),
		reposvc.WithStore(entries), reposvc.WithAccess(access), reposvc.WithPageSize(size.Repos))
	pullSvc := pullsvc.New(client, pullsvc.WithTTL(ttl.Pulls), pullsvc.WithCapacity(mem.Entries), pullsvc.WithStore(entries),
		pullsvc.WithAccess(access), pullsvc.WithRepos(repoSvc), pullsvc.WithPageSize(size.Pulls))
	// The issues service tells what a number is, an issue or a pull
	// request, and a pull request whose detail is cached needs no request.
	issueSvc := issuesvc.New(client, issuesvc.WithTTL(ttl.Issues), issuesvc.WithCapacity(mem.Entries), issuesvc.WithStore(entries),
		issuesvc.WithPulls(pullSvc), issuesvc.WithAccess(access), issuesvc.WithRepos(repoSvc), issuesvc.WithPageSize(size.Issues))
	notifSvc := notifsvc.New(client, notifsvc.WithTTL(ttl.Notifications), notifsvc.WithCapacity(mem.Entries),
		notifsvc.WithStore(entries), notifsvc.WithAccess(access), notifsvc.WithPageSize(size.Notifications))
	dashSvc := dashsvc.New(client, dashsvc.WithTTLs(dashsvc.TTLs{
		Header: ttl.Profile, Work: ttl.WaitingOnYou, Repos: ttl.DashboardRepos, Contributions: ttl.Contributions,
	}), dashsvc.WithCapacity(mem.Entries), dashsvc.WithStore(entries), dashsvc.WithWorkSize(size.WaitingOnYou))
	fileSvcOpts := []filesvc.Option{
		filesvc.WithTTL(ttl.Files), filesvc.WithCapacity(mem.Entries),
		filesvc.WithBlobCapacity(mem.Entries), filesvc.WithBlobMemory(int64(mem.Files)),
		filesvc.WithMaxBlobSize(int64(cfg.Files.Preview.MaxSize)),
	}
	if store != nil {
		fileSvcOpts = append(fileSvcOpts, filesvc.WithStore(store))
	}
	fileSvc := filesvc.New(client, fileSvcOpts...)
	// What a commit SHA names never changes, so it is kept with the files'
	// objects, which accounts on a host share.
	historySvcOpts := []historysvc.Option{
		historysvc.WithTTL(ttl.History), historysvc.WithCompareTTL(ttl.Compare), historysvc.WithCapacity(mem.Entries),
		historysvc.WithDiffMemory(int64(mem.Diffs)), historysvc.WithStore(entries), historysvc.WithCommitPageSize(size.Commits),
	}
	if store != nil {
		historySvcOpts = append(historySvcOpts, historysvc.WithObjects(store))
	}
	historySvc := historysvc.New(client, historySvcOpts...)
	actionSvc := actionssvc.New(client, actionssvc.WithTTL(ttl.Actions), actionssvc.WithLiveTTL(ttl.ActionsRunning),
		actionssvc.WithCapacity(mem.Entries), actionssvc.WithLogMemory(int64(mem.Logs)),
		actionssvc.WithStore(entries), actionssvc.WithAccess(access), actionssvc.WithRunPageSize(size.Runs))
	releaseSvc := releasesvc.New(client, releasesvc.WithTTL(ttl.Releases), releasesvc.WithCapacity(mem.Entries), releasesvc.WithStore(entries))
	searchSvc := searchsvc.New(client, searchsvc.WithTTL(ttl.Search), searchsvc.WithCodeTTL(ttl.CodeSearch), searchsvc.WithCapacity(mem.Entries),
		searchsvc.WithPageSize(size.Search))
	// The filters of the pull requests and issues offer the labels,
	// milestones and people of the repository.
	facetSvc := facetsvc.New(client, facetsvc.WithTTL(ttl.Filters), facetsvc.WithCapacity(mem.Entries))

	// What the set command changes for the session, the modals read as
	// they open.
	live := &session{cfg: cfg}
	// Links go to the pages of the session's host, over plain http where
	// it serves them so, and to no other host so.
	webHost := client.WebHost()
	if core.WebScheme(webHost) == "http" {
		termtext.AllowPlainHTTP(webHost)
	}
	icons, dates := ui.NewIcons(cfg.UI.Icons), ui.NewDates(cfg.UI.DateFormat)
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
		files.WithIcons(icons), files.WithFinderPreview(cfg.Files.Finder.Preview),
		files.WithHost(webHost), files.WithVoice(voice), files.WithEditor(cfg.Editor),
		files.WithPrefetch(cfg.Prefetch, cfg.Files.Preview.MaxSize),
	}
	checkOpts := []checks.Option{checks.WithVoice(voice)}
	if cfg.Sync.Enabled {
		checkOpts = append(checkOpts,
			checks.WithWatch(watchChecks(subscriber(engine, pollChecks), engine.Refresh, actionSvc.PollChecks)),
			checks.WithFollow(checks.Follow(followRuns(subscriber(engine, pollActions), engine.Refresh, actionSvc.Poll))))
	}
	// The login of the viewer, from the header the dashboard read.
	viewer := viewerLogin(dashSvc.CachedHeader, func(ctx context.Context) (core.Header, error) {
		// A kept header names the viewer as well as a fresh one.
		return dashSvc.Header(ctx, dashsvc.HeaderQuery{})
	})
	var (
		pullOpts = []pulls.Option{
			pulls.WithVoice(voice), pulls.WithIcons(icons), pulls.WithDates(dates), pulls.WithFacets(facetSvc),
			pulls.WithChecks(actionSvc, checkOpts...), pulls.WithRepos(repoSvc), pulls.WithViewer(pulls.Viewer(viewer)),
			pulls.WithPrefetch(cfg.Prefetch),
		}
		issueOpts = []issues.Option{
			issues.WithVoice(voice), issues.WithIcons(icons), issues.WithDates(dates), issues.WithFacets(facetSvc),
			issues.WithRepos(repoSvc), issues.WithViewer(issues.Viewer(viewer)),
			issues.WithPrefetch(cfg.Prefetch),
		}
	)
	// The notifications screen and the dashboard's inbox open what each
	// thread is about in its modal, and read it ahead as the lists of the
	// repository screen do, with one opener: whichever is on view reads
	// ahead, so one rate limit stops both and leaving the screen stops the
	// reads.
	threadOpts := []threads.Option{
		threads.WithPulls(pullSvc), threads.WithIssues(issueSvc), threads.WithReleases(releaseSvc),
		threads.WithMarkRead(cfg.Notifications.MarkReadOnOpen), threads.WithPrefetch(cfg.Prefetch),
	}
	opener := threads.New(ctx, threadOpts...)
	dashOpts := []dashboard.Option{
		dashboard.WithVoice(voice),
		dashboard.WithInbox(notifSvc),
		dashboard.WithOpener(opener),
		dashboard.WithHere(here, repoSvc),
		dashboard.WithGlyph(cfg.Dashboard.CalendarGlyph),
		dashboard.WithContributions(cfg.Dashboard.ContributionDays()),
		dashboard.WithIcons(icons),
		dashboard.WithDates(dates),
		dashboard.WithHost(webHost),
		dashboard.WithDetails(pullSvc, issueSvc),
		dashboard.WithLanding(landing{repos: repoSvc, files: fileSvc}),
		dashboard.WithPrefetch(cfg.Prefetch),
	}
	searchOpts := []searchpage.Option{
		searchpage.WithStart(searchStart(repoSvc, pinned)), searchpage.WithIcons(icons), searchpage.WithDates(dates), searchpage.WithHost(webHost), searchpage.WithVoice(voice),
		searchpage.WithDetails(pullSvc, issueSvc), searchpage.WithPrefetch(cfg.Prefetch),
	}
	layout := tui.Layout{
		Files:  files.New(ctx, fileSvc, cfg.Keys, fileOpts...),
		Pulls:  pulls.New(ctx, pullSvc, cfg.Keys, pullOpts...),
		Issues: issues.New(ctx, issueSvc, cfg.Keys, issueOpts...),
		Notifications: notifications.New(ctx, notifSvc, cfg.Keys,
			notifications.WithVoice(voice), notifications.WithOpener(opener),
			notifications.WithIcons(icons), notifications.WithDates(dates)),
		Search:    searchpage.New(ctx, searchSvc, cfg.Keys, searchOpts...),
		Dashboard: dashboard.New(ctx, dashSvc, cfg.Keys, dashOpts...),
	}

	// The history and the releases read the settings of the session,
	// which the set command may have changed since the start, each time
	// they open.
	historyOpts := func(c config.Config) []history.Option {
		return []history.Option{
			history.WithConfig(c.History), history.WithPrefetch(c.Prefetch), history.WithHost(webHost), history.WithVoice(voice),
			history.WithEditor(c.Editor), history.WithIcons(ui.NewIcons(c.UI.Icons)), history.WithDates(ui.NewDates(c.UI.DateFormat)),
		}
	}
	b := browser.New("", io.Discard, io.Discard)
	opts := []tui.Option{
		tui.WithBrowser(b.Browse),
		// The images probe reads the real environment and runs tmux.
		tui.WithImageProbe(imgcaps.EnvFrom(os.Getenv), imgcaps.RunTmux),
		tui.WithRepoInfo(repoSvc.Get),
		// goto opens a repository only once it is known to exist, a number
		// once it knows whether it is an issue or a pull request, and links
		// to the user's host.
		tui.WithRepos(repoSvc),
		tui.WithKinds(issueSvc),
		tui.WithRecall(recall{pinned: pinned, here: here, dash: dashSvc, repos: repoSvc, pulls: pullSvc, issues: issueSvc}),
		tui.WithHost(webHost),
		tui.WithVoice(voice),
		tui.WithSource(src),
		tui.WithHistory(func(ctx context.Context, repo core.RepoRef, defaultBranch string, base ui.BaseMsg) (ui.Modal, tea.Cmd) {
			return history.Opener(historySvc, cfg.Keys, historyOpts(live.cfg)...)(ctx, repo, defaultBranch, base)
		}),
		tui.WithCommit(func(ctx context.Context, repo core.RepoRef, sha, defaultBranch string) (ui.Modal, tea.Cmd) {
			return history.CommitOpener(historySvc, cfg.Keys, historyOpts(live.cfg)...)(ctx, repo, sha, defaultBranch)
		}),
		tui.WithRelease(func(ctx context.Context, repo core.RepoRef, id int64, url string) (ui.Modal, tea.Cmd) {
			o := []releases.Option{
				releases.WithVoice(voice), releases.WithIcons(ui.NewIcons(live.cfg.UI.Icons)), releases.WithDates(ui.NewDates(live.cfg.UI.DateFormat)),
			}
			return releases.Opener(releaseSvc, cfg.Keys, o...)(ctx, repo, id, url)
		}),
		tui.WithRateStatus(client),
		// The status bar names the account the token is for, once gh or an
		// earlier start's answer of GitHub named it.
		tui.WithLogin(login.login),
		// The app tells what the token can't do, and :auth grants it more.
		tui.WithAccess(access),
		tui.WithOldEnterprise(oldEnterprise),
		tui.WithLateWarning(lateWarning),
	}
	if path, err := historyPath(cfg.Cache.Disk, client.Host(), client.Account()); err == nil && path != "" {
		opts = append(opts, tui.WithCommandHistory(cmdhist.New(path, cfg.Commands.History)))
	}
	for _, w := range []string{logWarning, configWarning, warning} {
		if w != "" {
			opts = append(opts, tui.WithWarning(w))
		}
	}
	actionOpts := []actions.Option{
		actions.WithVoice(voice),
		actions.WithViewer(viewer), actions.WithRepos(repoSvc),
	}
	if cfg.Sync.Enabled {
		actionOpts = append(actionOpts, actions.WithFollow(followRuns(subscriber(engine, pollActions), engine.Refresh, actionSvc.Poll)))
	}
	opts = append(opts, tui.WithActions(func(ctx context.Context, repo core.RepoRef, f core.RunFilter) (ui.Modal, tea.Cmd) {
		// The icons, the dates and the reads ahead are those of the
		// session, which the set command may have changed since the start.
		o := append(slices.Clip(actionOpts), actions.WithIcons(ui.NewIcons(live.cfg.UI.Icons)), actions.WithDates(ui.NewDates(live.cfg.UI.DateFormat)),
			actions.WithPrefetch(live.cfg.Prefetch))
		return actions.Opener(actionSvc, cfg.Keys, o...)(ctx, repo, f)
	}), tui.WithSettings(func(c config.Config) {
		live.set(c)
		engine.SetIntervals(pollIntervals(c.Sync.Poll))
		obs.SetPrefetchBudget(c.Prefetch.Budget)
		setLogLevel(c.Log.Level)
	}))
	var (
		activity []func(bool)
		watchers []func(core.RepoRef)
		// online wakes what backed off while GitHub couldn't be reached,
		// once it answers again.
		online = []func(){engine.Online}
	)
	if cfg.Sync.Enabled {
		engine.SubscribeKind(pollNotifications, notifications.SyncKey, notifSvc.Poll)
		// The inbox isn't polled while the token may not read it, and is
		// polled at once when what the token may do changes, so that it
		// catches up as soon as the token may.
		refreshOn(ctx, access.Changes(), engine.Refresh, notifications.SyncKey)
		repoPolls := &repoWatch{subscribe: subscriber(engine, pollLists), polls: []repoPoll{
			{key: pullsvc.SyncKey, poll: pullSvc.Poll},
			{key: issuesvc.SyncKey, poll: unless(issuesOff(repoSvc.CachedGet), issueSvc.Poll)},
		}}
		activity = append(activity, engine.SetActive)
		watchers = append(watchers, repoPolls.set)
	}
	if store != nil {
		if r := newRevalidator(cfg.Cache, cfg.Sync.UnfocusedSlowdown, engine.Publish, issueSvc.Kept, pullSvc.Kept, notifSvc.Kept, fileSvc.Kept, historySvc.Kept, actionSvc.Kept); r != nil {
			go func() { _ = r.Run(ctx) }()
			activity = append(activity, r.SetActive)
			watchers = append(watchers, r.SetRepo)
			online = append(online, r.Online)
		}
	}
	// The engine runs with nothing to poll too, for the rate limits.
	go func() { _ = engine.Run(ctx) }()
	opts = append(opts, tui.WithSync(syncEvents(engine)), tui.WithOnline(func() {
		for _, fn := range online {
			fn()
		}
	}))
	if len(watchers) > 0 {
		opts = append(opts,
			tui.WithActivity(fanOut(activity...)),
			tui.WithRepoWatcher(fanOut(watchers...)),
		)
	}
	return tui.New(ctx, cfg, layout, opts...), done, nil
}

// quickToken returns the token of host when it needn't run gh for it, and
// otherwise, when gh keeps it in the system keyring, only where it is and
// the login it is for, and reports that it comes later, from authorize.
func quickToken(host string) (token accesssvc.Token, later bool) {
	value, source, login := github.QuickToken(host)
	if value == "" && source != "" {
		return accesssvc.Token{Source: source, Login: login}, true
	}
	return findToken(host), false
}

// authorize reads the token of host that gh keeps, logs the session of
// it, gives it to access and client, which sends the requests waiting
// for it, and then starts what the token may do. Without one, the
// requests waiting for it fail, and so does the app.
func authorize(host string, client *github.Client, access *accesssvc.Service, logStart func(accesssvc.Token), start func()) error {
	token := findToken(host)
	if token.Value == "" {
		client.Authorize("")
		return github.NoTokenError(host)
	}
	access.Found(token)
	logStart(token)
	client.Authorize(token.Value)
	start()
	return nil
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

// sessionConfig returns the config file resolved for the session on host
// as login, the account gh stores its token for, with the log at
// logLevel, the level it was opened at, which --debug and GH_DEBUG may
// have raised. The app takes this config, not the top level of the file.
func sessionConfig(file *config.File, host, login, logLevel string) (config.Config, config.Source, error) {
	cfg, src, err := file.Resolve(host, login)
	if err != nil {
		return config.Config{}, src, fmt.Errorf("config for %s: %w", host, err)
	}
	cfg.Log.Level = logLevel
	return cfg, src, nil
}
