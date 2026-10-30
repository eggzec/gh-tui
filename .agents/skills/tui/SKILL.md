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

## Reviewing a visual change with screenshots

Screenshots show colour and layout that a text capture hides, and cost fewer
tokens than a pasted screen. Review visual changes (and golden or snapshot
churn) by rendering the screen with [vhs](https://github.com/charmbracelet/vhs)
and opening the PNG.

1. Put the screen, as ANSI text, in a git-ignored scratch dir (never
   `/tmp`): `mkdir -p .tmp`. For golden or snapshot churn, the golden files
   under `testdata/` are already ANSI, so `cat` the old and new ones. For a
   live screen, run `tmux -L NAME capture-pane -e -p -t NAME >
   .tmp/after.ansi` (`-e` keeps the colours; `scripts/tui cap` drops them).
2. Render it. Use absolute paths. End the tape with a short `Sleep` after
   `Screenshot`: vhs 0.11.0 skips a screenshot that is the last command.
3. Open the PNG. Use the same settings before and after.

```sh
D=$PWD/.tmp
printf 'Output "%s/after.gif"\nSet Shell "bash"\nSet FontSize 14\nSet Padding 20\nSet Width 1140\nSet Height 820\nType "clear; cat %s/after.ansi"\nEnter\nSleep 2s\nScreenshot "%s/after.png"\nSleep 500ms\n' "$D" "$D" "$D" | vhs /dev/stdin
```

Width and Height are pixels: these fit a 120x40 screen (for 80x24, about
780x500). The gif is a by-product; ignore it. Run vhs outside the Bash
sandbox.
