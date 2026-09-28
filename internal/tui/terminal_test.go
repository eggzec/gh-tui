package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

// terminalRecords feeds msgs to a terminal and returns the terminal
// records it logs.
func terminalRecords(t *testing.T, msgs ...tea.Msg) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	var term terminal
	for _, msg := range msgs {
		term.observe(context.Background(), msg)
	}
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		if m["msg"] == "terminal" {
			out = append(out, m)
		}
	}
	return out
}

func TestTerminalRecord(t *testing.T) {
	size := tea.WindowSizeMsg{Width: 120, Height: 40}
	profile := tea.ColorProfileMsg{Profile: colorprofile.TrueColor}
	tests := []struct {
		name string
		msgs []tea.Msg
		want map[string]any
	}{
		{
			name: "with its version",
			msgs: []tea.Msg{profile, size, tea.TerminalVersionMsg{Name: "ghostty 1.3.1"}, tea.WindowSizeMsg{Width: 80, Height: 24}, terminalWaitMsg{}},
			want: map[string]any{"width": 120.0, "height": 40.0, "color_profile": "TrueColor", "xtversion": "ghostty 1.3.1"},
		},
		{
			name: "silent about its version",
			msgs: []tea.Msg{size, profile, terminalWaitMsg{}, profile},
			want: map[string]any{"width": 120.0, "height": 40.0, "color_profile": "TrueColor"},
		},
		{name: "no size yet", msgs: []tea.Msg{profile, terminalWaitMsg{}}},
		{
			name: "a version of many lines",
			msgs: []tea.Msg{size, profile, tea.TerminalVersionMsg{Name: "term\x1b[31m\nx" + strings.Repeat("v", 300)}},
			want: map[string]any{"width": 120.0, "height": 40.0, "color_profile": "TrueColor"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := terminalRecords(t, tt.msgs...)
			if tt.want == nil {
				if len(recs) != 0 {
					t.Errorf("records = %v, want none", recs)
				}
				return
			}
			if len(recs) != 1 {
				t.Fatalf("records = %v, want one", recs)
			}
			for k, want := range tt.want {
				if recs[0][k] != want {
					t.Errorf("%s = %v, want %v", k, recs[0][k], want)
				}
			}
			v, _ := recs[0]["xtversion"].(string)
			if _, ok := tt.want["xtversion"]; !ok && strings.ContainsAny(v, "\x1b\n") {
				t.Errorf("xtversion = %q, want one line without escapes", v)
			}
			if len(v) > maxVersion {
				t.Errorf("xtversion is %d bytes, want at most %d", len(v), maxVersion)
			}
		})
	}
}
