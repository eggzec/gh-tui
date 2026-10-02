package tui

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termimg"
)

// probeTimeout is how long the app waits for the terminal to answer its
// probe, long enough for ssh; without an answer, no images are drawn.
const probeTimeout = 1500 * time.Millisecond

// probeTimeoutMsg ends the wait for the answers to the probe of id.
type probeTimeoutMsg struct{ id uint32 }

// tmuxAnsweredMsg carries what tmux said of itself and its client.
type tmuxAnsweredMsg struct{ tmux imgcaps.Tmux }

// tmuxRecheckedMsg carries what tmux said when it was asked again.
type tmuxRecheckedMsg struct{ tmux imgcaps.Tmux }

// tmuxMovedMsg says that tmux is attached from another terminal than
// before, which shows images too.
type tmuxMovedMsg struct{}

// cellsAgainMsg has the app ask the size of a cell again.
type cellsAgainMsg struct{}

// tmuxClientMsg carries what tmux said of its client, or why it couldn't,
// and recheck, which asks tmux everything the verdict needs again.
type tmuxClientMsg struct {
	client  imgcaps.TmuxClient
	err     error
	recheck tea.Cmd
}

// The stages of a probe, after none.
const (
	// stageName asks XTVERSION, with DA1 after it.
	stageName = 1 + iota
	// stageKitty asks kitty's graphics query, with DA1 after it.
	stageKitty
)

// imageProbe finds out whether the terminal shows images, once the color
// profile of the output is known. Inside tmux it asks tmux, and sends the
// terminal nothing, not even XTVERSION, which tmux may pass on to the
// pane as keys. Otherwise it asks the terminal its name with XTVERSION,
// with DA1 after it, which every terminal answers, and, only of a
// terminal that named itself one that may draw kitty placeholders,
// kitty's graphics query with DA1 again; it decides once the last DA1
// came, or after probeTimeout in all. The answers arrive as messages of
// their own, so one that comes late is dropped, never taken for keys.
// XTVERSION is the app's only one: the terminal record logs its answer.
type imageProbe struct {
	mode string
	// enabled is set by WithImageProbe; without it the probe asks nothing.
	enabled bool
	env     imgcaps.Env
	// tmux runs tmux; tests replace it.
	tmux func(ctx context.Context, args ...string) (string, error)
	// id is the image id of the kitty query, so that its answer is told
	// from others, and of the probe's timer.
	id uint32
	// planned is set once a profile started the probe, and fromPlan when
	// the profile, or what the environment said, decided alone.
	planned, fromPlan bool
	stage             int
	name              string
	// profile is the latest color profile, and rgbAsked is set once a
	// terminal known to have 24-bit color was asked to say so.
	profile  colorprofile.Profile
	rgbAsked bool
	reply    imgcaps.KittyReply
	sent     time.Time
	waited   time.Duration
	done     bool
	verdict  imgcaps.Verdict
	// asking is set while tmux is asked of its client or again of
	// everything, and again when the size of a cell was wanted meanwhile,
	// which the answer in flight may predate. client is what tmux said of
	// its client last, and seen what it said when the verdict was made.
	asking, again bool
	// moved is set while tmux is asked again because its client is
	// another terminal.
	moved  bool
	client imgcaps.TmuxClient
	seen   imgcaps.Tmux
}

// newImageProbe returns a probe in mode that asks nothing, until
// WithImageProbe gives it the environment and tmux.
func newImageProbe(mode string) imageProbe {
	return imageProbe{mode: mode, id: newProbeID()}
}

// WithImageProbe has the app find out at startup whether the terminal
// shows images, from env, what the environment says of the terminal, and
// with tmux, which runs tmux and returns what it printed. Without it the
// app sends the terminal nothing and draws no images, as tests want.
func WithImageProbe(env imgcaps.Env, tmux func(ctx context.Context, args ...string) (string, error)) Option {
	return func(m *Model) {
		m.images.env, m.images.tmux, m.images.enabled = env, tmux, true
	}
}

// newProbeID returns an id below those of images, whose high byte is
// never 0.
func newProbeID() uint32 {
	return 1 + rand.Uint32N(1<<24-1) //nolint:gosec // an id, not a secret
}

// update takes msg, if it is an answer to the probe, and returns what to
// do next, and whether msg was the probe's alone. The color profile,
// which starts the probe, is the app's to pass to plan. The terminal
// record has seen XTVERSION's answer before the probe takes it.
//
// The probe takes every KittyGraphicsEvent, its own or late. The phases
// that draw images will get replies of their own to a=t and a=p, when
// they don't send q=2, and must route those past the probe, by their
// image ids, before it drops them.
func (p *imageProbe) update(msg tea.Msg) (cmd tea.Cmd, handled bool) {
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if !p.done && p.stage == stageKitty && uint32(msg.Options.ID) == p.id { //nolint:gosec // an id the app chose
			p.reply = imgcaps.KittyError
			if string(msg.Payload) == "OK" {
				p.reply = imgcaps.KittyOK
			}
		}
		return nil, true
	case tea.TerminalVersionMsg:
		name := terminalName(msg.Name)
		if !p.done && p.stage == stageName {
			p.name = name
		}
		return p.upgrade(name), true
	case uv.PrimaryDeviceAttributesEvent:
		if p.done {
			return nil, true
		}
		switch p.stage {
		case stageName:
			ask, v := imgcaps.Named(p.mode, p.name)
			if !ask {
				return p.finish(v), true
			}
			p.stage = stageKitty
			return tea.Raw(termimg.Query(p.id) + ansi.RequestPrimaryDeviceAttributes), true
		case stageKitty:
			return p.finish(imgcaps.Decide(p.name, p.reply)), true
		}
		// A DA1 the probe didn't ask for.
		return nil, true
	case probeTimeoutMsg:
		if p.done || msg.id != p.id {
			return nil, true
		}
		return p.finish(imgcaps.TimedOut()), true
	case tmuxAnsweredMsg:
		if p.done {
			return nil, true
		}
		p.seen = msg.tmux
		return p.finish(imgcaps.DecideTmux(msg.tmux)), true
	case tmuxClientMsg:
		p.asking = false
		if msg.err != nil {
			return tea.Batch(p.tmuxCell(imgcaps.Cell{}), p.askedMeanwhile()), true
		}
		prev, c := p.client, msg.client
		p.client = c
		// A change of terminal, or of allow-passthrough, as when the user
		// sets it on as :images suggests and reloads tmux's config, may
		// change the verdict.
		moved := c.Termtype != p.seen.ClientTermtype || prev.TTY != "" && c.TTY != prev.TTY
		if moved || c.Passthrough != p.seen.Passthrough {
			p.asking, p.moved = true, moved
			return msg.recheck, true
		}
		return tea.Batch(p.tmuxCell(c.Cell), p.askedMeanwhile()), true
	case tmuxRecheckedMsg:
		p.asking = false
		p.seen = msg.tmux
		// The terminal tmux is now attached from has none of the images
		// sent to the one before, if that showed them too; one that only
		// now shows images gets them as the verdict turns them on.
		var moved tea.Cmd
		v := imgcaps.DecideTmux(msg.tmux)
		if p.moved && p.verdict.Images && v.Images {
			moved = func() tea.Msg { return tmuxMovedMsg{} }
		}
		p.moved = false
		if v != p.verdict {
			// The app asks the size of a cell anew once it has the
			// verdict, if images are drawn.
			p.sent, p.waited, p.again = time.Time{}, 0, false
			return tea.Batch(p.finish(v), moved), true
		}
		return tea.Batch(p.tmuxCell(p.client.Cell), p.askedMeanwhile(), moved), true
	}
	return nil, false
}

// askClient asks tmux of its client, once the verdict came from tmux:
// the size of its cells, and whether it is still the terminal the verdict
// was made for. The app asks when images need the size of a cell and
// when it gains focus, since client_termtype is that of the terminal
// tmux was last attached from: after a detach and an attach from another
// terminal, or a change of allow-passthrough, tmux is asked everything
// again, and a verdict that changes is told to the app as the first was.
// One question is in flight at a time: a focus meanwhile asks nothing
// more, and a size wanted meanwhile is asked once it is answered.
func (p *imageProbe) askClient(ctx context.Context) tea.Cmd {
	if !p.done || !p.verdict.Tmux || p.asking || p.tmux == nil {
		return nil
	}
	p.asking = true
	run := p.tmux
	recheck := func() tea.Msg { return tmuxRecheckedMsg{tmux: imgcaps.QueryTmux(ctx, run)} }
	return func() tea.Msg {
		c, err := imgcaps.QueryTmuxClient(ctx, run)
		return tmuxClientMsg{client: c, err: err, recheck: recheck}
	}
}

// askCells asks tmux the size of its client's cells, or, while a question
// is in flight, asks again once it is answered.
func (p *imageProbe) askCells(ctx context.Context) tea.Cmd {
	if p.asking {
		p.again = true
		return nil
	}
	return p.askClient(ctx)
}

// askedMeanwhile has the app ask the size of a cell again if it was
// wanted while tmux was being asked.
func (p *imageProbe) askedMeanwhile() tea.Cmd {
	if !p.again {
		return nil
	}
	p.again = false
	return func() tea.Msg { return cellsAgainMsg{} }
}

// tmuxCell tells the app cell, the size of a cell tmux said, if images are
// drawn.
func (p *imageProbe) tmuxCell(cell imgcaps.Cell) tea.Cmd {
	if !p.verdict.Images {
		return nil
	}
	msg := sized(cell, cellFromTmux)
	return func() tea.Msg { return msg }
}

// plan starts finding out once profile, that of the output, is known.
// A later profile that lets images be drawn where the one before didn't,
// as when the terminal says it has more colors, starts again.
func (p *imageProbe) plan(ctx context.Context, profile colorprofile.Profile) tea.Cmd {
	p.profile = profile
	if !p.enabled {
		if p.planned {
			return nil
		}
		p.planned = true
		return p.finish(imgcaps.Verdict{Reason: "the app doesn't probe the terminal"})
	}
	path, v := imgcaps.Plan(p.mode, p.env, profile)
	if p.planned {
		if !p.fromPlan || path == imgcaps.Decided {
			return nil
		}
		*p = imageProbe{mode: p.mode, enabled: true, env: p.env, tmux: p.tmux, id: newProbeID(), profile: profile, rgbAsked: p.rgbAsked}
	}
	p.planned = true
	switch path {
	case imgcaps.Decided:
		p.fromPlan = true
		cmd := p.finish(v)
		// XTVERSION is asked for the terminal record all the same,
		// except through a multiplexer, which may pass the answer on
		// as keys, and of a terminal with no graphics, which is sent
		// no probe at all.
		if p.env.Tmux || p.env.Screen || p.env.Zellij || p.env.Term == "" || p.env.Term == "dumb" || p.env.Term == "linux" {
			return cmd
		}
		return tea.Batch(tea.Raw(ansi.RequestNameVersion), cmd)
	case imgcaps.AskTmux:
		p.sent = time.Now()
		run := p.tmux
		return func() tea.Msg { return tmuxAnsweredMsg{tmux: imgcaps.QueryTmux(ctx, run)} }
	case imgcaps.Probe:
	}
	p.sent, p.stage = time.Now(), stageName
	id := p.id
	return tea.Batch(
		tea.Raw(ansi.RequestNameVersion+ansi.RequestPrimaryDeviceAttributes),
		tea.Tick(probeTimeout, func(time.Time) tea.Msg { return probeTimeoutMsg{id: id} }),
	)
}

// upgrade asks the terminal that named itself name for its RGB and Tc
// capabilities, once, if it is one known to draw kitty placeholders, all
// of which have 24-bit color, and the profile says fewer, as it does
// when TERM and COLORTERM didn't reach the app, over ssh or sudo. The
// program raises the profile on either answer, and the better profile
// starts the probe again if the first had turned images off. NO_COLOR
// keeps the profile the user asked for, and images.enabled off keeps the
// colors the app had before it drew images.
func (p *imageProbe) upgrade(name string) tea.Cmd {
	if p.rgbAsked || p.env.NoColor || p.mode == imgcaps.ModeOff || !p.planned || p.profile >= colorprofile.TrueColor {
		return nil
	}
	if _, ok := imgcaps.Allowed(name); !ok {
		return nil
	}
	p.rgbAsked = true
	return tea.Batch(tea.RequestCapability("RGB"), tea.RequestCapability("Tc"))
}

// graphicsDecidedMsg carries the verdict to the app, which logs attrs
// with the terminal and tells the sections graphics.
type graphicsDecidedMsg struct {
	graphics ui.Graphics
	attrs    []slog.Attr
}

// finish records v, and tells the app.
func (p *imageProbe) finish(v imgcaps.Verdict) tea.Cmd {
	p.done, p.verdict = true, v
	if !p.sent.IsZero() {
		p.waited = time.Since(p.sent)
	}
	msg := graphicsDecidedMsg{graphics: ui.Graphics{Images: v.Images, Tmux: v.Tmux}, attrs: p.logAttrs()}
	return func() tea.Msg { return msg }
}

// logAttrs are the fields of the verdict in the terminal record. Through
// tmux, which isn't asked XTVERSION, they name the terminal of its
// client as tmux does.
func (p *imageProbe) logAttrs() []slog.Attr {
	v := p.verdict
	attrs := []slog.Attr{
		slog.Bool("images", v.Images),
		slog.String("images_mode", p.mode),
		slog.String("images_reason", v.Reason),
	}
	if v.Tmux {
		attrs = append(attrs, slog.Bool("images_tmux", true))
		if v.Terminal != "" {
			attrs = append(attrs, slog.String("client_termtype", terminalName(v.Terminal)))
		}
	}
	if p.waited > 0 {
		attrs = append(attrs, slog.Float64("images_waited_ms", float64(p.waited.Microseconds())/1000))
	}
	return attrs
}
