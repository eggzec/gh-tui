package notifications

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The selected row is bold in every cell, an unread thread looks different
// selected or not, and the title takes the accent in a focused pane only.
func TestSelectedLooks(t *testing.T) {
	t.Parallel()
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	th := ui.NewTheme(p, true)
	st := newStyles(th, ui.NewIcons(config.IconsASCII))
	bold := func(p paint) bool { return strings.HasPrefix(p.render("x"), "\x1b[1") }

	for unread := range 2 {
		plain, blurred, focused := st.look[lookPlain][unread], st.look[lookBlurred][unread], st.look[lookFocused][unread]
		if plain.title.render("x") == blurred.title.render("x") && unread == 0 {
			t.Errorf("read title is the same selected and not: %q", plain.title.render("x"))
		}
		if blurred.title.render("x") == focused.title.render("x") {
			t.Errorf("unread %d: title is the same in a focused and a blurred pane", unread)
		}
		for _, c := range []paint{blurred.tag, blurred.repo, blurred.title, blurred.reason, blurred.age, focused.tag, focused.title, focused.age} {
			if !bold(c) {
				t.Errorf("unread %d: selected cell %q is not bold", unread, c.render("x"))
			}
		}
		if bold(plain.tag) || bold(plain.age) {
			t.Errorf("unread %d: a row that isn't selected has bold cells", unread)
		}
	}
	unread := func(look int) string { return st.look[look][1].dot + st.look[look][1].title.render("x") }
	if unread(lookPlain) == unread(lookBlurred) || unread(lookPlain) == unread(lookFocused) {
		t.Errorf("an unread thread looks the same selected and not")
	}
}
