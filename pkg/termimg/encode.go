package termimg

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// chunk is how many bytes of base64 each sequence of a transmission
// carries at most: the protocol's limit, a multiple of 4.
const chunk = kitty.MaxChunkSize

// placement is the ID of the one virtual placement each image has, so a
// new one with another size replaces it instead of adding one.
const placement = 1

// st ends each sequence; base64 never holds it.
const st = "\x1b\\"

// Transmit returns the sequences that send data, a PNG image width by
// height pixels, as image id (a=t, t=d, f=100), quietly (q=2), in chunks
// of base64 (m=1 up to the last, m=0). Only the first holds the keys but
// q. A PNG carries its own size, so a width or height below 1 is left out
// (s=, v=). Sending another image as the same id replaces it, and the
// cells that show it need no redraw. It returns "" for no data.
func Transmit(id ID, data []byte, width, height int) string {
	if len(data) == 0 {
		return ""
	}
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	b.Grow(len(enc) + (len(enc)/chunk+1)*48)
	for i := 0; i < len(enc); i += chunk {
		part := enc[i:min(i+chunk, len(enc))]
		last := i+chunk >= len(enc)
		var keys []string
		if i == 0 {
			keys = []string{"a=t", "f=100", "t=d", "i=" + id.key()}
			if width > 0 {
				keys = append(keys, "s="+strconv.Itoa(width))
			}
			if height > 0 {
				keys = append(keys, "v="+strconv.Itoa(height))
			}
		}
		keys = append(keys, "q=2")
		switch {
		case i == 0 && last:
			// One chunk needs no m.
		case last:
			keys = append(keys, "m=0")
		default:
			keys = append(keys, "m=1")
		}
		b.WriteString(ansi.KittyGraphics([]byte(part), keys...))
	}
	return b.String()
}

// Place returns the sequence that makes image id show in placeholder
// cells cols by rows (a=p, U=1), fitted to them with its aspect kept. A
// new size replaces the last. cols and rows are clamped as [Rows] clamps
// them, so the two agree.
func Place(id ID, cols, rows int) string {
	cols, rows = clamp(cols), clamp(rows)
	return ansi.KittyGraphics(nil, "a=p", "U=1", "i="+id.key(), "p="+strconv.Itoa(placement),
		"c="+strconv.Itoa(cols), "r="+strconv.Itoa(rows), "q=2")
}

// Delete returns the sequence that deletes image id and frees its data
// (a=d, d=I); its cells then show nothing.
func Delete(id ID) string {
	return ansi.KittyGraphics(nil, "a=d", "d=I", "i="+id.key(), "q=2")
}

// Query returns the sequence that asks whether the terminal knows the
// protocol, with a one-pixel image it doesn't keep (a=q). A terminal
// that does answers with id and OK or an error; the others say nothing.
func Query(id uint32) string {
	return ansi.KittyGraphics([]byte("AAAA"), "a=q", "i="+strconv.FormatUint(uint64(id), 10),
		"s=1", "v=1", "t=d", "f=24")
}

// Tmux wraps each kitty graphics sequence of seq in tmux's passthrough,
// one wrap each, so tmux sends them on to the terminal it runs in. tmux
// needs allow-passthrough on. Anything after the last sequence is wrapped
// too, so nothing of seq reaches tmux unwrapped. The placeholder cells
// need no wrap.
func Tmux(seq string) string {
	var b strings.Builder
	for seq != "" {
		end := strings.Index(seq, st)
		if end < 0 {
			end = len(seq)
		} else {
			end += len(st)
		}
		b.WriteString(ansi.TmuxPassthrough(seq[:end]))
		seq = seq[end:]
	}
	return b.String()
}

func (id ID) key() string { return strconv.FormatUint(uint64(id), 10) }
