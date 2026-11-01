package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// The view asks the terminal for the alternate keys and the text of each
// key, since with every key an escape code a terminal sends the unshifted
// code alone, and a shifted symbol would decode as its base key. These are
// the sequences kitty sends with those flags on.
func TestKittyShiftedSymbolsDecode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, seq, want string }{
		{"question mark", "\x1b[47:63;2;63u", "?"},
		{"colon", "\x1b[59:58;2;58u", ":"},
		{"bang", "\x1b[49:33;2;33u", "!"},
		{"at", "\x1b[50:64;2;64u", "@"},
		{"capital", "\x1b[103:71;2;71u", "G"},
		{"question mark without text", "\x1b[47:63;2u", "?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var d uv.EventDecoder
			_, ev := d.Decode([]byte(tc.seq))
			k, ok := ev.(uv.KeyPressEvent)
			if !ok {
				t.Fatalf("%q decoded as %T, want a key press", tc.seq, ev)
			}
			if got := tea.KeyPressMsg(k).String(); got != tc.want {
				t.Errorf("%q decoded as %q, want %q", tc.seq, got, tc.want)
			}
		})
	}
}

func TestViewAsksForAlternateKeys(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	k := m.View().KeyboardEnhancements
	if !k.ReportAllKeysAsEscapeCodes || !k.ReportAlternateKeys || !k.ReportAssociatedText {
		t.Errorf("keyboard enhancements = %+v, want all keys, alternate keys and text", k)
	}
}
