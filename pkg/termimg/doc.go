// Package termimg encodes images for the kitty graphics protocol, drawn
// through Unicode placeholders: the image is sent to the terminal once,
// out of band, and shows in text cells that name it, so it scrolls, clips
// and hides under overlays like any other text.
//
// The sequences it returns are for the terminal only, written outside the
// view (such as with tea.Raw), and the placeholder rows are text for the
// view. Nothing here checks that the terminal knows placeholders.
//
// The rows name their image by a 256-color foreground, so a caller shows
// them only under a color profile of 256 colors or more: fewer colors, or
// none, lose the ID. The renderer must also keep each placeholder with
// its three diacritics as one cell of width 1. U+10EEEE is ambiguous in
// East Asian Width, so a renderer that counts ambiguous characters as wide
// would break the rows.
//
// See https://sw.kovidgoyal.net/kitty/graphics-protocol/.
package termimg
