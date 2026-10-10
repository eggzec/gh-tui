---
name: tui
description: Run gh-tui in tmux and drive it by hand. Use to check a change in the real TUI, for visual review, or to look at a screen at a given size.
---

# Checking gh-tui by hand

Use `scripts/tui` (from the worktree root). It needs tmux, which means
running the commands outside the Bash sandbox.

- `scripts/tui start NAME [COLSxROWS]` builds this worktree to
  `$TMPDIR/gh-tui-NAME` and runs it in a tmux server named NAME (120x40 by
  default).
- `scripts/tui keys NAME KEY...` sends tmux key names (`Enter`, `Escape`,
  `C-c`); `keys NAME -l 'text'` sends literal text. It returns once the
  screen stops changing (5 s at most), so don't add sleeps.
- `scripts/tui wait NAME REGEX [SECS]` waits for text on screen.
- `scripts/tui cap NAME` prints the screen; `scripts/tui stop NAME` ends it.

Rules:

- The CLI takes no arguments. Navigate with the command line: `keys NAME -l
  ':goto owner/name'`, then `keys NAME Enter`.
- Never confirm a mutation (merge, close, comment, star) against real GitHub.
  Stop at the confirmation prompt and `cap` it.
- Check both 80x24 and 120x40, with a separate `start` for each.
- Rebuild with `start` again after a rebase or an edit, and always `stop`.

Example:

```sh
scripts/tui start pr 80x24 && scripts/tui wait pr 'Dashboard'
scripts/tui keys pr -l ':goto cli/cli#1' && scripts/tui keys pr Enter
scripts/tui wait pr 'cli/cli' && scripts/tui cap pr
scripts/tui stop pr
```
