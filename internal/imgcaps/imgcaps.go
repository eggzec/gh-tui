// Package imgcaps decides whether the terminal shows the images the app
// draws: with kitty's graphics protocol and its Unicode placeholders,
// which only some terminals implement. It asks nothing itself, beyond
// running tmux: the app sends the queries and passes on the replies.
package imgcaps

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

// The modes of images.enabled, as the config names them.
const (
	ModeAuto = "auto"
	ModeOn   = "on"
	ModeOff  = "off"
)

// Verdict is whether images are drawn, and why.
type Verdict struct {
	// Images says to draw them.
	Images bool
	// Tmux says that the terminal is reached through tmux, so what the
	// app sends of an image is wrapped in tmux's passthrough.
	Tmux bool
	// Terminal names the terminal as it named itself, if it did.
	Terminal string
	// Reason says why, in a few words, for the log.
	Reason string
	// Fix, when set, is a line of tmux.conf that would have images
	// drawn, for the user to add.
	Fix string
}

// Env is what the environment says of the terminal.
type Env struct {
	Term string
	// Tmux, Screen and Zellij are set inside those multiplexers.
	Tmux, Screen, Zellij bool
	NoColor              bool
}

// EnvFrom reads the environment with getenv.
func EnvFrom(getenv func(string) string) Env {
	return Env{
		Term:    getenv("TERM"),
		Tmux:    getenv("TMUX") != "",
		Screen:  getenv("STY") != "",
		Zellij:  getenv("ZELLIJ") != "",
		NoColor: getenv("NO_COLOR") != "",
	}
}

// Path is how the app finds out.
type Path int

// The paths.
const (
	// Decided needs nothing more: the verdict is known.
	Decided Path = iota
	// Probe asks the terminal its name, XTVERSION, with DA1 after it,
	// which every terminal answers, and then, only of a terminal that
	// may draw kitty placeholders, kitty's graphics query with DA1 again:
	// a terminal with a parser of its own may show the query's bytes.
	Probe
	// AskTmux asks tmux for its version, its passthrough and the
	// terminal its client runs in, and sends the terminal nothing.
	AskTmux
)

// Plan says how to find out whether to draw images in mode, a mode of
// images.enabled, from env and profile, the color profile of the output,
// and the verdict when it needs nothing more.
func Plan(mode string, env Env, profile colorprofile.Profile) (Path, Verdict) {
	switch {
	case mode == ModeOff:
		return Decided, off("images are turned off in the settings")
	case env.NoColor:
		return Decided, off("NO_COLOR is set")
	case env.Term == "" || env.Term == "dumb" || env.Term == "linux":
		return Decided, off("TERM " + strconv.Quote(env.Term) + " has no graphics")
	case profile < colorprofile.ANSI256:
		// The color of a placeholder names its image, and fewer colors
		// lose the name.
		return Decided, off("the terminal has fewer than 256 colors (" + profile.String() + ")")
	case env.Tmux:
		return AskTmux, Verdict{}
	case env.Screen:
		return Decided, off("GNU screen passes no graphics through")
	case env.Zellij:
		return Decided, off("zellij passes no graphics through")
	}
	return Probe, Verdict{}
}

// KittyReply is what the terminal answered the kitty graphics query.
type KittyReply int

// The replies.
const (
	// KittyNone is no answer, as terminals without the protocol give.
	KittyNone KittyReply = iota
	KittyOK
	KittyError
)

// Named gives what the name the terminal answered XTVERSION with, or ""
// when it didn't answer, decides in mode: the verdict, or that the kitty
// query is to be asked, with ask set. Only a name that may draw kitty
// placeholders goes on: in auto mode to the query, and in on mode
// straight to images. The environment never stands in for the name, in
// either mode: a terminal started from kitty's or Ghostty's shell
// inherits it, and TERM passes through ssh and sudo.
func Named(mode, name string) (ask bool, v Verdict) {
	if name == "" {
		return false, off("the terminal didn't say its name")
	}
	why, ok := Allowed(name)
	switch {
	case !ok:
		return false, Verdict{Terminal: name, Reason: why}
	case mode == ModeOn:
		return false, Verdict{Images: true, Terminal: name, Reason: "the terminal is known to draw kitty placeholders, so it wasn't asked"}
	}
	return true, Verdict{}
}

// Decide gives the verdict once the terminal, which answered XTVERSION
// with name, answered the kitty query with reply: only an answer OK
// turns images on.
func Decide(name string, reply KittyReply) Verdict {
	switch reply {
	case KittyOK:
		return Verdict{Images: true, Terminal: name, Reason: "the terminal draws kitty placeholders"}
	case KittyError:
		return Verdict{Terminal: name, Reason: "the terminal refused kitty graphics"}
	case KittyNone:
	}
	return Verdict{Terminal: name, Reason: "the terminal has no kitty graphics"}
}

// TimedOut is the verdict when the terminal didn't answer in time.
func TimedOut() Verdict { return off("the terminal didn't answer in time") }

// minKitty is the first kitty with Unicode placeholders.
var minKitty = [2]int{0, 28}

// Allowed reports whether the terminal that names itself name, as
// XTVERSION answers, such as "kitty(0.43.1)" or "ghostty 1.2.0", draws
// kitty's Unicode placeholders, and else says why not. A kitty whose
// version can't be read may be one before them.
func Allowed(name string) (why string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "ghostty"):
		return "", true
	case strings.HasPrefix(lower, "kitty"):
		v, read := version(strings.TrimPrefix(lower, "kitty"))
		switch {
		case !read:
			return "kitty didn't say its version, and before 0.28 it has no Unicode placeholders", false
		case less(v, minKitty):
			return "kitty before 0.28 has no Unicode placeholders", false
		}
		return "", true
	}
	return "the terminal isn't known to draw kitty placeholders", false
}

// version reads the first major.minor in s, such as 3.3 in "(3.3a)".
func version(s string) ([2]int, bool) {
	i := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' })
	if i < 0 {
		return [2]int{}, false
	}
	s = s[i:]
	majorText, rest, _ := strings.Cut(s, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil {
		return [2]int{}, false
	}
	end := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if end < 0 {
		end = len(rest)
	}
	minor, err := strconv.Atoi(rest[:end])
	if err != nil {
		return [2]int{major, 0}, true
	}
	return [2]int{major, minor}, true
}

func less(a, b [2]int) bool { return a[0] < b[0] || a[0] == b[0] && a[1] < b[1] }

func off(why string) Verdict { return Verdict{Reason: why} }
