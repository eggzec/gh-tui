# guard

A small Claude Code `PreToolUse` hook for the `Bash` and `Read` tools. Python 3,
standard library only. It reads the hook JSON on stdin and exits 0 to allow or
2 with a one-line reason on stderr to block. Malformed input or an internal
error allows (it fails open).

Rules:

| Rule | Blocks |
| --- | --- |
| `read-range` | Read of a file over 400 lines with no `limit`, or a `limit` over 200 |
| `subagent-all-packages` | `go test\|vet\|build ./...` from a subagent (payload has `agent_id` or `agent_type`) |
| `git-stash` | any `git stash` |
| `force-push` | `--force`, `-f`, `--force-with-lease` or `+ref` pushes to main or master; a push with no named branch is allowed |
| `ai-attribution` | `git commit` and `gh pr create\|edit` text (also a `--body-file`) with Co-Authored-By: Claude, "Generated with Claude Code" and the like |
| `shell-read` | `cat`, `head`, `tail`, `sed -n` of a file, unless in a pipe, following (`tail -f`) or a heredoc |
| `shell-write` | `cat`/`echo`/`printf` with `> file`, a heredoc written with `> file`, `tee file`; `>>`, `/dev/*`, `2>&1`, and build output redirects are allowed |

Commands are split into simple commands with a small quote-aware scanner, so
words inside quotes or heredoc bodies do not fire a rule. `bash -c '...'` is
checked recursively, and wrappers such as `env`, `sudo` and `.tools/cool` are
skipped. `git` and `gh` commands skip the shell-read and shell-write rules.

Try a command: `tools/guard/guard --explain "git stash"` (add `--agent` to act
as a subagent). Run the tests: `python3 tools/guard/guard_test.py`.
