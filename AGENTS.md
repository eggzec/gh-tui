# AGENTS.md

Guidance for anyone, human or agent, working on **gh-tui**, a GitHub client
for the terminal built on the Charm stack. Read this before you change code.

## Goals

In priority order:

1. **Readable and maintainable.** Code is read far more often than it is written.
2. **Reliable.** Services are tested and failures degrade gracefully.
3. **Snappy.** Data loads lazily, the app caches smartly, updates are
   optimistic, and View/Update are benchmarked.
4. **Configurable.** Users can change keybindings, colors and behavior.
5. **Pleasant.** The widgets are well styled and fun to use.

## Stack

Use the v2 Charm libraries and their `charm.land` import paths:

| Purpose    | Module                                                       |
| ---------- | ------------------------------------------------------------ |
| Runtime    | `charm.land/bubbletea/v2`                                    |
| Components | `charm.land/bubbles/v2`                                      |
| Styling    | `charm.land/lipgloss/v2`                                     |
| Forms      | `charm.land/huh/v2` (when needed)                            |
| TUI tests  | `github.com/charmbracelet/x/exp/teatest/v2`                  |
| GitHub     | `github.com/cli/go-gh/v2` (host and token discovery only)    |

Before you use a library API, check the current docs (Context7, pkg.go.dev,
or the upstream UPGRADE guides). Several APIs changed in v2: `View()` returns
`tea.View`, keys arrive as `tea.KeyPressMsg`, `AdaptiveColor` was replaced by
`lipgloss.LightDark` together with `tea.BackgroundColorMsg`, and `DefaultKeyMap()`
and `DefaultStyles(isDark)` are now functions.

## Layout

```
cmd/gh-tui/           main: parse flags, load config, wire services into the tui
internal/
  core/               domain value types (Repo, PullRequest, Issue, …) and errors; no I/O
  config/             loading, defaults, validation, and the keymap and theme schema
  github/             one transport client (REST, GraphQL, pagination, rate limits,
                      errors) plus domain methods in files by concept (pulls.go, …)
  cache/              in-memory LRU, optional disk layer, TTL and ETag metadata
  watch/              sync engine: polling, conditional requests, change events
  revalidate/         re-checks cached entries in the background within a budget
  buildinfo/          what the binary was built from, such as its version
  obs/                log/slog setup, trace and request ids, counters and summaries
  logfile/            the log file, rotated by size, shared by several processes
  service/<domain>/   business logic per domain (pulls, issues, repos, notifications…)
  tui/                root model: the dashboard, repo, notifications and search
                      screens and their panes, the header, help, toasts, modals,
                      and routing between them
  tui/ui/             what sections share: the Section interface, theme, keys, app messages
  tui/<section>/      one package per section, adapting a service to bubbles
pkg/bubbles/<name>/   reusable Elm-style components with no knowledge of gh-tui
third_party/<name>/   vendored upstream code that needed changes (see Vendoring)
```

Rules:

- **Dependencies point inward.** `tui` depends on interfaces for behavior. It
  may use the plain value types in `core`, which are the shared vocabulary,
  but it never imports `github`, `cache` or `watch` implementations. Only
  `cmd/` knows about concrete types and wires them together. A section may
  import its service package for query types such as `pulls.ListQuery`,
  while still declaring the small interface it calls.
- **`pkg/bubbles` imports nothing from `internal/`.** A bubble must be usable
  in another program without changes.
- **Consumers define interfaces**, and keep them small. They live next to the
  code that uses them. Producers return concrete types.
- **Services expose reads the same way:** `CachedList(q ListQuery)` and
  `List(ctx, q)`, `CachedGet(…)` and `Get(ctx, …)`, and for paged sub-lists
  such as comments `CachedComments(q CommentsQuery)` and `Comments(ctx, q)`.
  Query structs carry `Cursor` (the previous page's `Next`) and `PageSize`
  (0 for the default), and both are part of the cache key. `Cached…` reads
  never do I/O.
- **Name unexported helpers in `internal/github` after their domain**
  (`restIssue`, `pullComment`), since several branches add to that package
  at once. Shared shapes live in `types.go`.
- **Split packages and files by concept, not by size.** If a file needs a table
  of contents, split it up. Typical bubble files are `model.go`, `update.go`,
  `view.go`, `keys.go`, `styles.go` and `options.go`.

## Bubbles (`pkg/bubbles`)

Each bubble is a self-contained Elm component: model, `Init`, `Update`, and `View`.

- **Build with options.** Construct with `New(opts ...Option)` using `WithX`
  options, the same way `bubbles/table` does. After construction, use
  `SetX`/`X()` accessors. Don't export mutable fields.
- **Producers are injected.** Any data a bubble needs arrives as a function
  type that the bubble itself declares. The bubble calls the function inside a
  `tea.Cmd`, so its Update never blocks:

  ```go
  // Fetch returns the page after cursor. An empty next cursor means the end.
  type Fetch func(ctx context.Context, cursor string) (items []Item, next string, err error)

  func New(fetch Fetch, opts ...Option) Model
  ```

  The tui adapts service calls to these signatures. Bubbles never see services.
- **Customization follows Charm conventions.** Expose `KeyMap` and
  `DefaultKeyMap()`, `Styles` and `DefaultStyles(isDark bool)`,
  `SetKeyMap`, `SetStyles` and `SetSize(width, height)`. Implement
  `help.KeyMap` (`ShortHelp`/`FullHelp`).
- **Full help lists every binding.** `KeyMap.FullHelp()` returns every
  binding exactly once. State shows through `Enabled()`, never by leaving a
  binding out, and `Model.FullHelp()` returns the same set with the model's
  state applied, so the help can tell a disabled key from a missing one.
  Check each key map with `keytest.Complete` and `keytest.NoConflicts`.
  In the tui, sections and modals list their keys as `ui.Keyed` layers in
  the order they match keys, a composite joining its children's, and the
  help reads them from there.
- **Components render strings.** A bubble's `View()` returns a string that
  fits its size exactly; only the root returns a `tea.View`.
- **Interactive bubbles start blurred**, and the parent focuses the one in
  use. A resize that reveals rows not yet loaded fetches them on the next
  `Update`.
- **Copy slices you keep or change.** Models are values, so copies share
  their slices.
- **Messages are scoped.** Each instance gets an ID, and its messages carry
  that ID, so two instances of the same bubble never react to each other's
  messages.
- **Reuse before you build.** Start from `bubbles` (list, table, viewport,
  textinput, textarea, spinner, help, paginator), then from other Charm
  projects such as `glamour` for markdown and `huh`. Wrapping or composing
  an existing bubble is better than writing a new one.

### Vendoring

If an upstream component can't be customized through its public API, vendor
it into `third_party/<name>/`. Keep its LICENSE and add a `README.md` with the
upstream module, the commit, and a short list of local changes. Keep the
changes minimal so that pulling in new upstream versions stays easy.

## Data

- **Lazy by default.** Fetch only what is on screen and prefetch the next page
  when the user nears the end. A detail never embeds an unbounded list such
  as comments; those are paged reads of their own.
- **Fetch in fixed chunks, render a window.** Services fetch pages of a fixed
  size per resource (their default, or the user's config), never sized to the
  terminal, so cache keys and cursors survive a resize. A bubble keeps the
  chunks around its viewport, renders only the visible rows, and prefetches
  the next chunk near the end. Resizing only moves the window.
- **Choose the right API for each call.** Use GraphQL when you need nested or
  batched data in one round trip, such as a PR with its reviews, checks and
  labels. Use REST where it is simpler or cheaper, for example conditional
  requests with `ETag`/`If-None-Match` (a 304 doesn't count against the rate
  limit), notifications (`Last-Modified`, `X-Poll-Interval`), and endpoints
  that have no GraphQL equivalent.
- **Respect rate limits.** Read the rate-limit headers or the GraphQL
  `rateLimit` field, then back off and surface the limit to the user.
- **Cancel with context.** Every call takes a `context.Context`. Navigating
  away cancels the work in flight.

### Cache

- The in-memory LRU sits in front of every read. The disk layer
  (`cache/disk`) is optional and makes cold starts faster; a service reads
  memory, then disk, then the network, and writes what it fetches through.
  `Cached…` reads stay in memory.
- Put on disk what a hash names, since it never needs revalidating: git
  blobs by their SHA, and listings by the commit their ref resolved to. Keep
  a ref's resolved SHA and validators on disk too, so a new session
  revalidates it with a free 304. Fall back to the disk copy only when
  GitHub can't be reached, never on a 401, 403 or 404.
- Keep lists, details and comment pages on a `cache.Shelf`, in a directory
  per account (`<host>/entry/<account>`, a hash of host and the login gh
  is logged in as, or of host and token when the token comes from the
  environment), so no account reads another's. A read that misses memory warms it from the
  shelf in its `tea.Cmd`. A kept list page, dashboard value or page of
  branches comes back at once with `Stale` set, to every reader until
  any write replaces it: the revalidator, a poll, a read with `Again` set,
  or a change's stored response. So views that read it at once all show it
  at once. A read that follows a value that came back `Stale` must set
  `Again` on its query, or it is served the kept value again; feeds do
  (`feed.ErrStale`), and their reads again share one request. A kept
  detail counts as cached, so the list's update time still vouches for it.
  A kept entry fetched or revalidated within the TTL is fresh, like one
  this session fetched.
- Keep only what GitHub sent. Optimistic changes stay in memory until
  GitHub confirms them, and what is kept after a change has no validators.
  An outage serves the kept entry with `Offline` set, and a rate limit
  with `Limited` set, until any answer, a 304 too; a refusal drops it
  (`service/fallback`). Bump a shelf's schema when its value type changes
  shape.
- Use stale-while-revalidate: serve cached data at once, refresh it in the
  background, and emit an update message if the data changed.
- Store the `ETag`/`Last-Modified` for each entry so that revalidation costs
  almost nothing. GraphQL has no validators, so GraphQL reads refetch stale
  entries in full; prefer REST when a free 304 matters more than one round
  trip.
- Key entries by the query plus its variables. A mutation invalidates only the
  keys it affects.

### Sync engine

- Owns polling for the views that are currently subscribed. Honor
  `X-Poll-Interval` and slow down when the app is idle or unfocused.
- Publishes typed change events. The tui turns these into `tea.Msg` through
  one long-lived `tea.Cmd` subscription.
- Deduplicates and coalesces events so a burst of changes produces one
  re-render.

### Revalidation

- `internal/revalidate` re-checks kept entries in the background, one
  conditional request each, so a load finds them fresh. It knows nothing of
  GitHub: each service lists what it can check (`Kept()`, built with
  `service/recheck` over `cache.Shelf.Kept` and `Recheck`) and says what a
  check found. Only entries with validators are listed; content named by a
  SHA never changes, and GraphQL entries are left to the probes (`Poll`,
  whose ETags are kept too) and the update times (`service/seen`).
- A 304 marks the entry fetched now, in memory and on disk, without a
  re-render. A 200 stores the new value and returns a sync key, which the
  revalidator publishes through the sync engine (`watch.Engine.Publish`),
  once per key per batch, so the views read the cache again.
- It keeps to a budget of requests per minute (a 304 is free against the
  primary rate limit, not the secondary ones), checks the selected
  repository first and then what was used most recently, pauses while
  offline or rate limited, and slows down while unfocused. Its `Pass`
  reports carry the counts to log.

### Optimistic updates

User actions such as merging, closing, labeling, starring or marking as read
follow the same four steps:

1. Apply the change to local state and the cache right away.
2. Send the mutation.
3. When it succeeds, reconcile with the server's response.
4. When it fails, roll back to the previous snapshot and show a non-blocking
   error toast.

Keep the snapshot and rollback logic in the service layer so the tui only
reacts to messages. Concretely:

- A service mutation method changes the cache at once (`cache.Mutate` or
  `cache.MutateTag`, which returns rollbacks) and returns an
  `*optimistic.Op`. It does no I/O.
- The tui calls it in `Update`, re-renders from the cache, and runs `Op.Do`
  in a `tea.Cmd`. `Do` sends the mutation and rolls the cache back if it
  fails; on success the send function stores the server's response.
- Prefer mutation endpoints that return the updated resource, so there is
  something to reconcile with. When an endpoint returns nothing, reconcile
  by applying the change again, or by marking the affected entries stale.

## Logging

- Logs are JSON Lines through `log/slog`, written to `log.file` (default
  `$XDG_STATE_HOME/gh-tui/gh-tui.log`, else `~/.local/state/gh-tui/gh-tui.log`,
  on macOS too; `%LocalAppData%` on Windows), mode 0600, rotated at
  `log.max_size` keeping `log.keep` files. Nothing goes to stdout or stderr
  while the app runs. `GH_TUI_LOG=debug`, `--debug` or gh's `GH_DEBUG` raise
  the level for one run.
- Every record has a `session_id`. The `start` record says what the binary
  was built from, how it is set up and the terminal, and the `session`
  record, once the host is picked, who the session is; every record after
  it carries `host` and `account`. Start a trace where a user action or a
  background job starts (`obs.WithTrace` or `obs.Begin`, which also logs the
  end and the error) and pass its context down; records logged with it carry
  `trace_id` and `trace`. Each HTTP attempt gets a `request_id`, and
  GitHub's `X-GitHub-Request-Id` as `gh_request_id`. `span` names the layer.
- Log with the context: `slog.InfoContext(ctx, "msg", "span", "service.x",
  …)`. Levels: debug for per-item detail (cache lookups, checks, reads
  ahead), info for requests and decisions, warn for what degrades
  (rollbacks, rate limits, 4xx), error for failures with `err`.
- Count what a summary should report in `obs.Stats` (`obs.CountCache`,
  `obs.CountPrefetch`, …); the summary is logged every `log.summary` and on
  exit.
- Never log tokens, headers, request or response bodies, comment text or
  file contents. Keep logging out of `View` and per-frame `Update`, and
  guard the arguments of hot debug records with `obs.Enabled`.
  `pkg/bubbles` doesn't log.

## Performance

- `View` is pure and cheap. Build lipgloss styles once, when the styles
  change, not on every render. Cache rendered fragments that are expensive to
  produce, such as markdown or long lists, and invalidate them by width or
  content.
- `Update` never blocks. All I/O runs in a `tea.Cmd`.
- Render only the visible rows.
- Every bubble and the root model have `BenchmarkView` and `BenchmarkUpdate`,
  written with `b.Loop()` and `b.ReportAllocs()`. Record the before and after
  numbers (`benchstat`) in a PR when you optimize.

## Testing

- **Bubbles** are tested with plain function producers, so they need no
  services and no mocks. Use table-driven tests for Update, golden files for
  View, and include benchmarks.
- **Tests that drive the program** (keys in, output and final model out) use
  `teatest`. Refresh golden files with `go test ./... -update`.
- **Services** are tested with small hand-written fakes of the interfaces they
  consume. For the API clients, use `httptest.Server` with recorded fixtures.
  Cover error paths, pagination, cache hits and misses, and rollback.
- **Time-dependent code**, such as the sync engine, is tested with
  `testing/synctest`, so it uses a fake clock and no real sleeps.
- Run `go test -race ./...` before every commit.

## Configuration

- The config file is `$GH_TUI_CONFIG`, or `gh-tui/config.yaml` in
  `os.UserConfigDir()` (`$XDG_CONFIG_HOME` on Linux). It is validated on load
  and every field has a sensible default.
- Keybindings map action names to keys and are applied through each bubble's
  `SetKeyMap`. Action names are registered in `internal/config/keys.go`;
  unknown names are rejected so typos don't pass silently.
- The only command-line flags are `--debug`, `--hostname` and
  `--version`; gh-tui takes no arguments, and `:goto` opens a repository,
  pull request or issue. The host is `--hostname`, else the current repository's
  (`GH_REPO` or the git remotes), else `GH_HOST` or gh's default host, as
  gh picks it, and one session talks to one host.
- Hex colors must be quoted in YAML, since an unquoted `#` starts a comment.
- Themes are named palettes that each have a light and a dark variant. They
  are resolved once after `tea.BackgroundColorMsg` and applied through
  `SetStyles`.

## Styling

- Aim for a consistent, calm palette with one accent color. Use borders
  sparingly. Keep spacing and alignment consistent across views.
- Loading uses spinners or skeletons, empty states tell the user what to do,
  and errors are inline and recoverable.
- One modal at a time. Opening a modal replaces the open one; a modal that
  needs several views, such as a list and its detail, shows them as panes or
  steps inside its own frame, and esc steps back before it closes.
- Everything must stay legible in both light and dark terminals and at 80
  columns.

## Code style

- Follow [Effective Go](https://go.dev/doc/effective_go), the
  [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments), and the
  [Google Go Style Guide](https://google.github.io/styleguide/go/).
- Keep names short and clear, with no stutter (`pulls.Service`, not
  `pulls.PullsService`). Return early and keep the happy path unindented.
- Use modern Go: generics where they reduce duplication, `iter`, `slices`,
  `maps`, `min`/`max`, and `for range n`. Run the gopls modernize analyzers
  (`go fix ./...`) and apply what they suggest.
- Comments explain *why*, in plain language, and only when the code can't say
  it. Every exported identifier has a doc comment that starts with its name.
  Don't narrate what the code does.
- Wrap errors with context (`fmt.Errorf("list pulls: %w", err)`). Don't panic
  in library code.

### Tooling

```sh
go vet ./...
golangci-lint run ./...          # config in .golangci.yml (v2)
golangci-lint fmt --diff ./...   # gofmt and goimports
go test -race ./...
make bench                       # every benchmark in the module
```

gopls diagnostics and golangci-lint must be clean before you commit. To check
that every commit on a branch passes on its own:

```sh
chk="$TMPDIR/chk-$(git branch --show-current | tr / -)"   # unique per branch
for c in $(git rev-list --reverse origin/main..HEAD); do
  git worktree add -q --detach "$chk" "$c"
  (cd "$chk" && go build ./... && go vet ./... && go test -race ./... &&
    golangci-lint run --allow-parallel-runners ./... && golangci-lint fmt --diff ./...)
  git worktree remove --force "$chk"
done
```

## Git workflow

### Worktrees

The repository is a bare clone, and every branch is checked out in its own
worktree next to it:

```
gh-tui/
  .bare/            bare repository
  .git              "gitdir: ./.bare"
  main/             worktree for main (read-only by convention)
  <branch-name>/    one worktree per feature branch
```

```sh
git fetch origin
git worktree add -b feat/cache-lru cache-lru origin/main
# after merge
git worktree remove cache-lru && git branch -D feat/cache-lru
```

### Branches and PRs

- **Never commit to `main` directly.** All changes land through PRs.
- **One branch per feature or component**, for example `feat/prlist-bubble`
  or `feat/cache-lru`. Keep PRs small enough to review in one sitting.
- **PR bodies are short and meant for people.** Say what changed and anything
  a reviewer must know, in a few lines. Skip file-by-file summaries,
  boilerplate sections and long rationale.
- **Verify before you open a PR.** Run the checks under Tooling, then wait for
  CI to pass before you merge.
- **Merge with rebase and merge only.**
- **Use `gh` with care.** Run explicit, single-purpose commands, check the
  result of each one, and don't take destructive or bulk actions without
  asking first.
- **Use [Conventional Commits](https://www.conventionalcommits.org/)**:
  `type(scope): summary`. Add a body only when the reason for the change
  isn't obvious. Keep it short and never force one.
- **Every commit is atomic and passes on its own**, meaning it builds, passes
  lint and passes tests. The history should tell the story of the work. If
  you find something you left out of an earlier commit, fold it into that
  commit. Don't add "fix typo" commits:

  ```sh
  git commit --fixup=<sha>
  # merge-base keeps the branch on its current base
  GIT_SEQUENCE_EDITOR=: git rebase -i --autosquash $(git merge-base HEAD origin/main)
  ```
- **No AI attribution** in commit messages, trailers or PR descriptions.
