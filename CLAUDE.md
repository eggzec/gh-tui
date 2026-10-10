@AGENTS.md

# Claude Code

- **Navigate with the LSP tool (gopls)** before grep: go-to-definition,
  find-references, hover, implementations and diagnostics. If it is deferred,
  load it with ToolSearch `select:LSP`. Use grep for text, not symbols.
- **Never block on one command for more than about 4 minutes.** The prompt
  cache lasts 5 minutes, so a longer wait makes the next turn pay for the
  whole context again. Run long commands in the background and poll in short
  steps.
- **Make independent tool calls in parallel**, in one message. Read a file
  once, by range (see Tooling); don't re-read AGENTS.md.
- **Edit with the Edit tool**, not shell heredocs or sed, for code.
- **Long tasks:** commit, then hand off to a fresh agent at about 60 turns
  instead of growing one context.
- **Agents:** Explore or Haiku agents for lookups, Sonnet to implement from a
  detailed brief, and a reviewer only for a risky PR, one round. The reviewer
  reads the diff and the CI status and doesn't re-run the suite.
- **Driving the TUI** by hand: use the `tui` skill, which wraps `scripts/tui`.
  Skills live in `.agents/skills`; `.claude/skills` only links to them.
  Don't write your own tmux helper.
