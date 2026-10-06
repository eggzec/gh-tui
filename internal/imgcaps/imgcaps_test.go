package imgcaps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
)

var (
	kittyEnv   = Env{Term: "xterm-kitty"}
	ghosttyEnv = Env{Term: "xterm-ghostty"}
	plainEnv   = Env{Term: "xterm-256color"}
	tmuxEnv    = Env{Term: "tmux-256color", Tmux: true}
	tmuxKitty  = Tmux{Version: "3.5a", Passthrough: "on", ClientTermtype: "kitty(0.43.1)"}
)

// probe runs a probe of the answers name, to XTVERSION, and reply, to the
// kitty query, and returns the verdict and whether the query was asked.
func probe(mode, name string, reply KittyReply) (Verdict, bool) {
	ask, v := Named(mode, name)
	if !ask {
		return v, false
	}
	return Decide(name, reply), true
}

// TestVerdicts goes through the terminals, tmux, the color profiles and
// the modes, from the environment to the verdict.
func TestVerdicts(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		env     Env
		profile colorprofile.Profile
		// path is the route the probe takes. A probe answers XTVERSION with xtversion
		// and the kitty query, if asked, with reply; tmux answers with
		// tmux.
		path      Path
		xtversion string
		reply     KittyReply
		tmux      Tmux
		// asked says that the probe asks the kitty query.
		asked  bool
		want   bool
		reason string
	}{
		{name: "kitty", mode: ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "kitty(0.43.1)", reply: KittyOK, asked: true, want: true},
		{name: "ghostty", mode: ModeAuto, env: ghosttyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "ghostty 1.2.0", reply: KittyOK, asked: true, want: true},
		{name: "ghostty on 256 colors", mode: ModeAuto, env: ghosttyEnv, profile: colorprofile.ANSI256, path: Probe,
			xtversion: "ghostty 1.2.0", reply: KittyOK, asked: true, want: true},
		{name: "old kitty", mode: ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "kitty(0.27.1)", reason: "before 0.28"},
		{name: "kitty without a version", mode: ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "kitty", reason: "didn't say its version"},
		{name: "konsole isn't asked the query", mode: ModeAuto, env: plainEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "Konsole 25.04.0", reason: "isn't known"},
		{name: "wezterm isn't asked the query", mode: ModeAuto, env: plainEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "WezTerm 20240203-110809-5046fc22", reason: "isn't known"},
		{name: "konsole in kitty's environment", mode: ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "Konsole 25.04.0", reason: "isn't known"},
		{name: "konsole, on", mode: ModeOn, env: plainEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "Konsole 25.04.0", reason: "isn't known"},
		{name: "kitty, on, skips the query", mode: ModeOn, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "kitty(0.43.1)", want: true},
		{name: "on, only the environment", mode: ModeOn, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			reason: "didn't say its name"},
		{name: "kitty graphics refused", mode: ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "kitty(0.43.1)", reply: KittyError, asked: true, reason: "refused"},
		{name: "kitty query unanswered", mode: ModeAuto, env: ghosttyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "ghostty 1.2.0", asked: true, reason: "no kitty graphics"},
		{name: "no XTVERSION, kitty's environment", mode: ModeAuto, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			reply: KittyOK, reason: "didn't say its name"},
		{name: "no XTVERSION, ghostty's environment", mode: ModeAuto, env: ghosttyEnv, profile: colorprofile.TrueColor, path: Probe,
			reply: KittyOK, reason: "didn't say its name"},
		{name: "no XTVERSION, another's environment", mode: ModeAuto, env: plainEnv, profile: colorprofile.TrueColor, path: Probe,
			reason: "didn't say its name"},
		{name: "kitty without a version, on", mode: ModeOn, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "kitty", reason: "didn't say its version"},
		{name: "old kitty, on", mode: ModeOn, env: kittyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "kitty(0.27.1)", reason: "before 0.28"},
		{name: "ghostty, on, skips the query", mode: ModeOn, env: ghosttyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "ghostty 1.2.0", want: true},
		{name: "wezterm in ghostty's environment, on", mode: ModeOn, env: ghosttyEnv, profile: colorprofile.TrueColor, path: Probe,
			xtversion: "WezTerm 20240203-110809-5046fc22", reason: "isn't known"},
		{name: "off with 256 colors", mode: ModeOff, env: ghosttyEnv, profile: colorprofile.ANSI256, path: Decided, reason: "images are turned off in the settings"},
		{name: "empty TERM", mode: ModeAuto, env: Env{}, profile: colorprofile.TrueColor, path: Decided, reason: "no graphics"},
		{name: "no color profile", mode: ModeAuto, env: kittyEnv, profile: colorprofile.NoTTY, path: Decided, reason: "fewer than 256"},
		{name: "off", mode: ModeOff, env: kittyEnv, profile: colorprofile.TrueColor, path: Decided, reason: "images are turned off in the settings"},
		{name: "NO_COLOR", mode: ModeOn, env: Env{Term: "xterm-kitty", NoColor: true}, profile: colorprofile.Ascii, path: Decided, reason: "NO_COLOR"},
		{name: "16 colors", mode: ModeAuto, env: kittyEnv, profile: colorprofile.ANSI, path: Decided, reason: "fewer than 256"},
		{name: "dumb", mode: ModeAuto, env: Env{Term: "dumb"}, profile: colorprofile.TrueColor, path: Decided, reason: "no graphics"},
		{name: "linux console", mode: ModeAuto, env: Env{Term: "linux"}, profile: colorprofile.ANSI256, path: Decided, reason: "no graphics"},
		{name: "screen", mode: ModeAuto, env: Env{Term: "screen-256color", Screen: true}, profile: colorprofile.ANSI256, path: Decided, reason: "screen"},
		{name: "zellij", mode: ModeAuto, env: Env{Term: "xterm-256color", Zellij: true}, profile: colorprofile.TrueColor, path: Decided, reason: "zellij"},
		{name: "tmux, kitty", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux, tmux: tmuxKitty, want: true},
		{name: "tmux, ghostty, all", mode: ModeOn, env: tmuxEnv, profile: colorprofile.TrueColor, path: AskTmux,
			tmux: Tmux{Version: "next-3.6", Passthrough: "all", ClientTermtype: "ghostty 1.2.0"}, want: true},
		{name: "tmux 3.2", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.2a", Passthrough: "on", ClientTermtype: "kitty(0.43.1)"}, reason: "older than 3.3"},
		{name: "tmux without passthrough", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.4", Passthrough: "off", ClientTermtype: "kitty(0.43.1)"}, reason: "allow-passthrough is off"},
		{name: "tmux can't say its passthrough", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux:   Tmux{Version: "3.4", PassthroughErr: errors.New("invalid option"), ClientTermtype: "kitty(0.43.1)"},
			reason: "couldn't say its allow-passthrough: invalid option"},
		{name: "tmux in konsole", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.4", Passthrough: "on", ClientTermtype: "Konsole 25.04.0"}, reason: "isn't known"},
		{name: "tmux, client unknown", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.4", Passthrough: "on"}, reason: "doesn't know"},
		{name: "tmux not asked", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{VersionErr: errors.New("no server running")}, reason: "no server running"},
		{name: "tmux, kitty without a version", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.4", Passthrough: "on", ClientTermtype: "kitty"}, reason: "didn't say its version"},
		{name: "tmux, old kitty", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.4", Passthrough: "on", ClientTermtype: "kitty(0.26.5)"}, reason: "before 0.28"},
		{name: "tmux 3.3", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.3", Passthrough: "on", ClientTermtype: "ghostty 1.2.0"}, want: true},
		{name: "tmux, one client", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.4", Passthrough: "on", ClientTermtype: "kitty(0.43.1)", Attached: 1}, want: true},
		{name: "tmux, two clients", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "3.4", Passthrough: "on", ClientTermtype: "kitty(0.43.1)", Attached: 2}, reason: "more than one terminal"},
		{name: "tmux with an unreadable version", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux: Tmux{Version: "master", Passthrough: "on", ClientTermtype: "ghostty 1.2.0"}, reason: "older than 3.3"},
		{name: "tmux can't say its client", mode: ModeAuto, env: tmuxEnv, profile: colorprofile.ANSI256, path: AskTmux,
			tmux:   Tmux{Version: "3.4", Passthrough: "on", TermtypeErr: errors.New("unknown format")},
			reason: "couldn't say the terminal of its client"},
		{name: "tmux, off", mode: ModeOff, env: tmuxEnv, profile: colorprofile.ANSI256, path: Decided, reason: "images are turned off in the settings"},
		{name: "tmux on 16 colors", mode: ModeAuto, env: Env{Term: "tmux", Tmux: true}, profile: colorprofile.ANSI, path: Decided, reason: "fewer than 256"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, v := Plan(tt.mode, tt.env, tt.profile)
			if path != tt.path {
				t.Fatalf("Plan path = %v, want %v", path, tt.path)
			}
			switch path {
			case Decided:
			case Probe:
				var asked bool
				v, asked = probe(tt.mode, tt.xtversion, tt.reply)
				if asked != tt.asked {
					t.Errorf("the kitty query asked %v, want %v", asked, tt.asked)
				}
			case AskTmux:
				v = DecideTmux(tt.tmux)
				if !v.Tmux {
					t.Error("a verdict in tmux doesn't say tmux")
				}
			}
			if v.Images != tt.want || tt.reason != "" && !strings.Contains(v.Reason, tt.reason) {
				t.Errorf("verdict = %+v, want images %v, a reason with %q", v, tt.want, tt.reason)
			}
		})
	}
}

func TestAllowed(t *testing.T) {
	for name, want := range map[string]bool{
		"kitty(0.43.1)": true, "kitty(0.28.0)": true, "kitty(0.27.9)": false, "kitty(1.0.0)": true, "kitty": false,
		"ghostty 1.2.0": true, "Ghostty 1.0.1": true, "Konsole 25.04.0": false, "WezTerm 20240203": false,
		"iTerm2 3.5.0": false, "foot(1.16.2)": false, "": false,
		"kitty()": false, "kitty(0.x)": false, "kitty(0.28)": true, " kitty(0.43.1)\n": true, "KITTY(0.43.1)": true,
		"tmux 3.4": false, "xterm(390)": false,
	} {
		if _, ok := Allowed(name); ok != want {
			t.Errorf("Allowed(%q) = %v, want %v", name, ok, want)
		}
	}
}

// Only kitty plays animations; the other terminals that draw kitty's
// placeholders show their first frame.
func TestAnimates(t *testing.T) {
	for name, want := range map[string]bool{
		"kitty(0.43.1)": true, " KITTY(0.28.0)\n": true, "ghostty 1.2.0": false, "WezTerm 20240203": false, "": false,
	} {
		if got := Animates(name); got != want {
			t.Errorf("Animates(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestVersion(t *testing.T) {
	tests := []struct {
		in   string
		want [2]int
		ok   bool
	}{
		{"3.4", [2]int{3, 4}, true},
		{"3.3a", [2]int{3, 3}, true},
		{"next-3.6", [2]int{3, 6}, true},
		{"(0.43.1)", [2]int{0, 43}, true},
		{"(0.28)", [2]int{0, 28}, true},
		{"4", [2]int{4, 0}, true},
		{"master", [2]int{}, false},
		{"", [2]int{}, false},
	}
	for _, tt := range tests {
		if got, ok := version(tt.in); got != tt.want || ok != tt.ok {
			t.Errorf("version(%q) = %v, %v, want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

// QueryTmux asks tmux its questions apart, and keeps the answers of
// those it could answer, and why the others failed.
func TestQueryTmux(t *testing.T) {
	var asked [][]string
	run := func(_ context.Context, args ...string) (string, error) {
		asked = append(asked, args)
		switch args[len(args)-1] {
		case "#{version}":
			return "3.4\n", nil
		case "allow-passthrough":
			return "", errors.New("invalid option")
		case sessionAttached:
			return "2\n", nil
		}
		return "ghostty 1.2.0\n", nil
	}
	got := QueryTmux(context.Background(), run)
	if got.Version != "3.4" || got.ClientTermtype != "ghostty 1.2.0" || got.Passthrough != "" || got.PassthroughErr == nil || got.Attached != 2 {
		t.Errorf("QueryTmux = %+v, want the version, the client, the passthrough's error and two clients", got)
	}
	want := [][]string{
		{"display-message", "-p", "#{version}"},
		{"show-options", "-Apv", "allow-passthrough"},
		{"display-message", "-p", "#{client_termtype}"},
		{"display-message", "-p", sessionAttached},
	}
	if !slices.EqualFunc(asked, want, slices.Equal) {
		t.Errorf("asked %q, want %q", asked, want)
	}
}

// A tmux that can't say how many clients show the session draws images
// as one with one client: it said all the verdict needs of the client.
func TestQueryTmuxAttachedFails(t *testing.T) {
	run := func(_ context.Context, args ...string) (string, error) {
		switch args[len(args)-1] {
		case "#{version}":
			return "3.4\n", nil
		case "allow-passthrough":
			return "on\n", nil
		case sessionAttached:
			return "", errors.New("unknown format")
		}
		return "kitty(0.43.1)\n", nil
	}
	got := QueryTmux(context.Background(), run)
	if got.Attached != 0 || got.Shared() {
		t.Errorf("QueryTmux = %+v, want no count and not shared", got)
	}
	if v := DecideTmux(got); !v.Images || v.Shared {
		t.Errorf("verdict = %+v, want images", v)
	}
}

// A tmux that doesn't answer holds the questions for TmuxTimeout at most.
func TestRunTmuxDeadline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nexec sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	start := time.Now()
	got := QueryTmux(context.Background(), RunTmux)
	if d := time.Since(start); d > TmuxTimeout+time.Second {
		t.Errorf("QueryTmux took %v, want about %v", d, TmuxTimeout)
	}
	if got.VersionErr == nil || got.PassthroughErr == nil || got.TermtypeErr == nil {
		t.Errorf("QueryTmux = %+v, want each question failed", got)
	}
	if v := DecideTmux(got); v.Images {
		t.Errorf("verdict = %+v, want none", v)
	}
}

// Only a fix the user can make is offered: tmux's passthrough, set
// off, not a terminal that draws no placeholders.
func TestTmuxFix(t *testing.T) {
	if v := DecideTmux(Tmux{Version: "3.4", Passthrough: "off", ClientTermtype: "kitty(0.43.1)"}); v.Fix != "set -g allow-passthrough on" {
		t.Errorf("passthrough off: fix = %q", v.Fix)
	}
	for _, tm := range []Tmux{tmuxKitty, {Version: "3.4", Passthrough: "on", ClientTermtype: "Konsole 25.04.0"}} {
		if v := DecideTmux(tm); v.Fix != "" {
			t.Errorf("%+v: fix = %q, want none", tm, v.Fix)
		}
	}
}
