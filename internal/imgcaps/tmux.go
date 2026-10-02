package imgcaps

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Tmux is what tmux says of itself and of the terminal its client runs
// in.
type Tmux struct {
	// Version is tmux's, such as 3.4 or 3.3a.
	Version string
	// Passthrough is allow-passthrough of the pane: on, all or off.
	Passthrough string
	// ClientTermtype is client_termtype: what the terminal of the client
	// answered tmux's XTVERSION, as it answered, such as "kitty(0.43.1)"
	// or "ghostty 1.2.0", or "" when it didn't.
	ClientTermtype string
	// VersionErr, PassthroughErr and TermtypeErr say why tmux couldn't
	// answer each question, if it couldn't.
	VersionErr, PassthroughErr, TermtypeErr error
}

// TmuxTimeout bounds the questions to tmux.
const TmuxTimeout = 500 * time.Millisecond

// minTmux is the first tmux with allow-passthrough.
var minTmux = [2]int{3, 3}

// QueryTmux asks the tmux of this pane, with run, which runs tmux with
// its arguments and returns what it printed. The questions are asked
// apart, so that one tmux can't answer doesn't lose the others, within
// TmuxTimeout for all of them.
func QueryTmux(ctx context.Context, run func(ctx context.Context, args ...string) (string, error)) Tmux {
	ctx, cancel := context.WithTimeout(ctx, TmuxTimeout)
	defer cancel()
	var t Tmux
	for _, q := range []struct {
		to   *string
		err  *error
		args []string
	}{
		{&t.Version, &t.VersionErr, []string{"display-message", "-p", "#{version}"}},
		{&t.Passthrough, &t.PassthroughErr, []string{"show-options", "-Apv", "allow-passthrough"}},
		{&t.ClientTermtype, &t.TermtypeErr, []string{"display-message", "-p", "#{client_termtype}"}},
	} {
		out, err := run(ctx, q.args...)
		if err != nil {
			*q.err = err
			continue
		}
		*q.to = strings.TrimSpace(out)
	}
	return t
}

// RunTmux runs tmux with args and returns its output.
func RunTmux(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	return string(out), err
}

// DecideTmux gives the verdict inside tmux, from what tmux said. The mode
// doesn't matter: nothing is asked of the terminal through tmux, and the
// terminal must be one known to draw kitty placeholders either way.
func DecideTmux(t Tmux) Verdict {
	v := Verdict{Tmux: true, Terminal: t.ClientTermtype}
	ver, read := version(t.Version)
	switch {
	case t.VersionErr != nil:
		v.Reason = "tmux couldn't say its version: " + t.VersionErr.Error()
	case !read || less(ver, minTmux):
		v.Reason = "tmux " + t.Version + " is older than 3.3, which passes graphics through"
	case t.PassthroughErr != nil:
		v.Reason = "tmux couldn't say its allow-passthrough: " + t.PassthroughErr.Error()
	case t.Passthrough != "on" && t.Passthrough != "all":
		v.Reason = "tmux's allow-passthrough is off: set -g allow-passthrough on"
	case t.TermtypeErr != nil:
		v.Reason = "tmux couldn't say the terminal of its client: " + t.TermtypeErr.Error()
	case t.ClientTermtype == "":
		v.Reason = "tmux doesn't know the terminal of its client"
	default:
		if why, ok := Allowed(t.ClientTermtype); !ok {
			v.Reason = why
			return v
		}
		v.Images, v.Reason = true, "the terminal draws kitty placeholders, through tmux"
	}
	return v
}

// TmuxClient is what tmux says of the client it shows the app's pane on:
// the terminal tmux was last attached from.
type TmuxClient struct {
	// TTY is the client's terminal device, which changes with each
	// attach from another terminal.
	TTY string
	// Termtype is client_termtype, as Tmux.ClientTermtype.
	Termtype string
	// Passthrough is allow-passthrough of the pane, as Tmux.Passthrough.
	Passthrough string
	// Cell is the size of the client's cells in pixels, which tmux reads
	// from the terminal's window size, or no size when the terminal gives
	// none, as over ssh.
	Cell Cell
}

// tmuxClientFormat asks for every field of TmuxClient at once, tab apart,
// since a terminal's name may hold spaces. A format names an option by
// its name, and gives the pane's value as show-options -Apv does.
const tmuxClientFormat = "#{client_tty}\t#{client_termtype}\t#{client_cell_width}\t#{client_cell_height}\t#{allow-passthrough}"

// QueryTmuxClient asks the tmux of this pane, with run, of its client,
// in one question, cheap enough to ask whenever the app gains focus.
// tmux 3.3, the oldest that draws images, has all these formats.
func QueryTmuxClient(ctx context.Context, run func(ctx context.Context, args ...string) (string, error)) (TmuxClient, error) {
	ctx, cancel := context.WithTimeout(ctx, TmuxTimeout)
	defer cancel()
	out, err := run(ctx, "display-message", "-p", tmuxClientFormat)
	if err != nil {
		return TmuxClient{}, err
	}
	f := strings.Split(strings.TrimRight(out, "\r\n"), "\t")
	if len(f) != 5 {
		return TmuxClient{}, fmt.Errorf("tmux answered %d fields of its client, not 5", len(f))
	}
	c := TmuxClient{TTY: strings.TrimSpace(f[0]), Termtype: strings.TrimSpace(f[1]), Passthrough: strings.TrimSpace(f[4])}
	w, werr := strconv.Atoi(strings.TrimSpace(f[2]))
	h, herr := strconv.Atoi(strings.TrimSpace(f[3]))
	if werr == nil && herr == nil {
		c.Cell = Cell{Width: w, Height: h}
	}
	return c, nil
}
