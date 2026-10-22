package pulls

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// described returns the modal with body as the description, focused on it and
// drawn once.
func described(t *testing.T, body string) *panesModal {
	t.Helper()
	m := openedPanes(t, 160)
	m.detail.Body = body
	m.focus = paneDescription
	m.View()
	return m
}

func paneText(m *panesModal) string { return ansi.Strip(m.View()) }

const withDetails = "<details><summary>Notes</summary>\n\nhidden words\n\n</details>\n\nafter the block"

const goldenBody = "## Summary\n\nThe pane shows the body as the page does.\n\n- first point\n- second point\n\n" +
	"<details><summary>Notes</summary>\n\nhidden words\n\n</details>\n\nafter the block"

// TestDescriptionView holds the modal with the description focused, and with
// another pane focused.
func TestDescriptionView(t *testing.T) {
	t.Parallel()
	for name, focus := range map[string]panes.Pane{"focused": paneDescription, "blurred": paneConversation} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := described(t, goldenBody)
			m.focus = focus
			golden.RequireEqual(t, m.View())
		})
	}
}

func TestDescriptionFolds(t *testing.T) {
	t.Parallel()
	m := described(t, withDetails)
	if s := paneText(m); !strings.Contains(s, "Notes") || !strings.Contains(s, "1 line") || strings.Contains(s, "hidden words") {
		t.Fatalf("closed block drew:\n%s", s)
	}
	m.press(keyMsg("enter"))
	if s := paneText(m); !strings.Contains(s, "hidden words") {
		t.Errorf("enter on the summary left the block closed:\n%s", s)
	}
	m.press(keyMsg("h"))
	if s := paneText(m); strings.Contains(s, "hidden words") {
		t.Errorf("h on the open summary left the block open:\n%s", s)
	}
	m.press(keyMsg("l"))
	if s := paneText(m); !strings.Contains(s, "hidden words") {
		t.Errorf("l on the closed summary left the block closed:\n%s", s)
	}
}

func TestDescriptionEnterElsewhereAndStar(t *testing.T) {
	t.Parallel()
	m := described(t, "one\n\ntwo\n\nthree")
	before := paneText(m)
	m.press(keyMsg("enter"))
	m.press(keyMsg("*"))
	if after := paneText(m); after != before {
		t.Errorf("enter and * on a plain line changed the pane:\n%s\nwas:\n%s", after, before)
	}
}

func TestDescriptionKeepsFoldsOnNewBody(t *testing.T) {
	t.Parallel()
	m := described(t, withDetails)
	m.press(keyMsg("enter"))
	m.detail.Body = withDetails + "\n\nmore"
	if s := paneText(m); !strings.Contains(s, "hidden words") || !strings.Contains(s, "more") {
		t.Errorf("a changed body closed the open block or was not drawn:\n%s", s)
	}
}

func TestDescriptionInfo(t *testing.T) {
	t.Parallel()
	m := described(t, "short")
	if s := paneText(m); strings.Contains(s, "edited") || strings.Contains(s, "%") {
		t.Errorf("a short unedited body has info:\n%s", s)
	}
	m.detail.BodyEditedAt = time.Now()
	if s := paneText(m); !strings.Contains(s, "edited") {
		t.Errorf("an edited body shows no edited:\n%s", s)
	}
	long := make([]string, 0, 160)
	for range 80 {
		long = append(long, "line", "")
	}
	m.detail.Body = strings.Join(long, "\n")
	if s := paneText(m); !strings.Contains(s, "edited · ") || !strings.Contains(s, "%") {
		t.Errorf("an overflowing body shows no percentage:\n%s", s)
	}
	m.press(keyMsg("space"))
	if s := paneText(m); strings.Contains(s, " 0%") {
		t.Errorf("space didn't page down:\n%s", s)
	}
}

// * and the arrows have no use on a details block: only enter, l and h open
// and close one.
func TestDescriptionKeysThatDoNothing(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"*", "right", "left"} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			m := described(t, withDetails)
			m.press(keyMsg(k))
			if s := paneText(m); strings.Contains(s, "hidden words") {
				t.Errorf("%s opened the block:\n%s", k, s)
			}
		})
	}
}

// A key the tree has no use for leaves the drawn panes as they are.
func TestDescriptionUnboundKeyKeepsDrawn(t *testing.T) {
	t.Parallel()
	m := described(t, withDetails)
	before := m.desc.version
	if _, ok := m.pressDescription(keyMsg("x")); ok || m.desc.version != before {
		t.Errorf("an unbound key was taken (ok %v) or bumped the version %d -> %d", ok, before, m.desc.version)
	}
	if _, ok := m.pressDescription(keyMsg("j")); !ok || m.desc.version == before {
		t.Errorf("a bound key was left (ok %v) or didn't bump the version", ok)
	}
}

// A body that went empty leaves no tree to take keys, and a new body starts
// one again.
func TestDescriptionEmptiedBody(t *testing.T) {
	t.Parallel()
	m := described(t, withDetails)
	m.detail.Body = ""
	m.View()
	if _, ok := m.pressDescription(keyMsg("j")); ok {
		t.Error("a key reached the tree of an empty body")
	}
	m.detail.Body = withDetails
	if s := paneText(m); !strings.Contains(s, "Notes") {
		t.Errorf("the body came back as:\n%s", s)
	}
}

// The percentage follows the cursor through the text, counting a closed block
// as the line of its summary.
func TestDescriptionPercent(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("line\n\n", 80)
	m := described(t, long+withDetails)
	if s := paneText(m); !strings.Contains(s, " 0%") {
		t.Errorf("at the top the pane doesn't show 0%%:\n%s", s)
	}
	m.press(keyMsg("G"))
	if s := paneText(m); !strings.Contains(s, "100%") {
		t.Errorf("at the bottom the pane doesn't show 100%%:\n%s", s)
	}
	if at, total := m.desc.position(); at != total {
		t.Errorf("the cursor is on line %d of %d, want the last", at, total)
	}
}

const withImage = "before\n\n![the logo](https://example.com/logo.png)\n\nafter"

// Images follow the app's images setting: they draw when it is on, and the
// body shows as without them when it is off.
func TestDescriptionImages(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{true, false} {
		t.Run(strconv.FormatBool(on), func(t *testing.T) {
			t.Parallel()
			src := &uitest.ImageHost{}
			m := openedPanes(t, 160)
			m.pics = uitest.Avatars(src, on)
			m.detail.Body = withImage
			m.focus = paneDescription
			m.View()
			uitest.LoadAvatars(t, m.pics)
			m.Update(ui.ImagesMsg{})
			n := uitest.Placeholders(t, m.View())
			if on && n == 0 {
				t.Errorf("with images on, the body draws no image:\n%s", paneText(m))
			}
			if !on && (n != 0 || !strings.Contains(paneText(m), "the logo")) {
				t.Errorf("with images off, the body draws images (%d) or lacks its alt text:\n%s", n, paneText(m))
			}
		})
	}
}

func TestDescriptionEmpty(t *testing.T) {
	t.Parallel()
	m := described(t, "  \n")
	if s := paneText(m); !strings.Contains(s, "No description") {
		t.Errorf("an empty body shows:\n%s", s)
	}
}
