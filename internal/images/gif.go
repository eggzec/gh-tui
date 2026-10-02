package images

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"time"

	"golang.org/x/image/draw"
)

// Limits of the animations decoded. A GIF past any of them shows its
// first frame, as one is shown where the terminal plays no animation.
const (
	// maxFrames is the most frames an animation may have.
	maxFrames = 200
	// maxFramesDecoded is the most bytes an animation may take as it
	// decodes: a byte for each pixel of each of its frames, which the
	// decoder holds at once, and four for each pixel of the two canvases
	// the frames are drawn on. It is the bound of any decode.
	maxFramesDecoded = maxDecoded
	// maxFramesSent is the most bytes of PNG the frames of an animation
	// may take together, since the app keeps them to send again.
	maxFramesSent = 16 << 20
	// maxFramesShown is the most bytes the frames of an animation may
	// take in the terminal, which keeps each decoded, four bytes for each
	// pixel of the size they are sent at: a few such animations still
	// stay well within what kitty keeps of all images before it drops
	// some.
	maxFramesShown = 32 << 20
)

// GIF delays: one of a hundredth of a second or less is played as a
// tenth, as browsers play it, since many GIFs say 0 and mean "soon".
const (
	gifTick      = 10 * time.Millisecond
	minGIFDelay  = 2 * gifTick
	slowGIFDelay = 100 * time.Millisecond
)

// Frame is one frame of an animation: a PNG of the whole image, as it
// shows, and how long it shows before the next.
type Frame struct {
	PNG   []byte
	Delay time.Duration
}

// errNotAnimated is a GIF that animates nothing, or not within the
// limits, and so shows its first frame.
var errNotAnimated = errors.New("not animated within the limits")

// decodeAnimation makes data, a GIF inspect let through, an animation
// that fits box: each frame as it shows, drawn over the frames before as
// their disposal says, scaled down to the size its first frame would be
// sent at. It fails with errNotAnimated when data has one frame or is
// past the limits it can tell before decoding, and with another error
// when it doesn't decode; either way the caller shows the first frame
// instead. Frames past maxFramesSent once encoded leave the first frame
// alone, already decoded.
func decodeAnimation(data []byte, box Box) (Image, error) {
	walk, err := gifFrames(data)
	if err != nil {
		return Image{}, err
	}
	count := walk.frames
	img := fit(walk.width, walk.height, box)
	if shown := count * img.Width * img.Height * 4; shown > maxFramesShown {
		return Image{}, fmt.Errorf("%w: %d frames of %dx%d take %d bytes shown", errNotAnimated, count, img.Width, img.Height, shown)
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("%w: gif: %w", ErrFormat, err)
	}
	if len(g.Image) != count {
		// The walk and the decoder disagree: trust neither.
		return Image{}, fmt.Errorf("%w: gif of %d frames decoded %d", ErrFormat, count, len(g.Image))
	}
	screen := image.Rect(0, 0, walk.width, walk.height)
	img.Format = "gif"
	img.Loops = gifLoops(g.LoopCount)
	canvas := image.NewRGBA(screen)
	var prev *image.RGBA
	// Each frame is scaled without the sharper scaler's scratch, which
	// it would allocate anew for every frame.
	var s scaler
	enc := png.Encoder{BufferPool: &onePool{}}
	sent := 0
	for i, fr := range g.Image {
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			if prev == nil {
				prev = image.NewRGBA(screen)
			}
			copy(prev.Pix, canvas.Pix)
		}
		r := fr.Bounds().Intersect(screen)
		draw.Draw(canvas, r, fr, r.Min, draw.Over)
		var out bytes.Buffer
		if err := enc.Encode(&out, s.scale(canvas, img.Width, img.Height)); err != nil {
			return Image{}, fmt.Errorf("encode frame: %w", err)
		}
		if sent += out.Len(); sent > maxFramesSent {
			if len(img.Frames) == 0 {
				return Image{}, fmt.Errorf("%w: a frame takes over %d bytes", errNotAnimated, maxFramesSent)
			}
			// The first frame shows alone, as it was drawn.
			img.PNG, img.Frames, img.Loops = img.Frames[0].PNG, nil, 0
			return img, nil
		}
		delay := 0
		if i < len(g.Delay) {
			delay = g.Delay[i]
		}
		img.Frames = append(img.Frames, Frame{PNG: out.Bytes(), Delay: gifDelay(delay)})
		switch disposal {
		case gif.DisposalBackground:
			// Browsers clear to transparent, not to the background
			// color, and so does this.
			draw.Draw(canvas, r, image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			copy(canvas.Pix, prev.Pix)
		}
	}
	img.PNG = img.Frames[0].PNG
	return img, nil
}

// gifDelay returns how long a frame of delay hundredths of a second
// shows.
func gifDelay(delay int) time.Duration {
	d := time.Duration(delay) * gifTick
	if d < minGIFDelay {
		return slowGIFDelay
	}
	return d
}

// gifLoops returns how many times an animation whose GIF says count
// plays, or 0 for forever: a GIF says 0 for forever, -1 for once, and
// otherwise how many times it plays again after the first.
func gifLoops(count int) int {
	switch {
	case count == 0:
		return 0
	case count < 0:
		return 1
	}
	return count + 1
}

// gifWalk is what the blocks of a GIF say: the size of its screen and
// how many frames it has.
type gifWalk struct {
	width, height, frames int
}

// gifFrames walks the blocks of data, a GIF, without decoding any, and
// returns what they say, if it animates within the limits: more than
// one frame, at most maxFrames, whose pixels and canvases decode to at
// most maxFramesDecoded bytes. It fails with errNotAnimated otherwise,
// and with ErrFormat for blocks it can't walk.
func gifFrames(data []byte) (gifWalk, error) {
	bad := func(what string) (gifWalk, error) { return gifWalk{}, fmt.Errorf("%w: gif: %s", ErrFormat, what) }
	if len(data) < 13 || !bytes.HasPrefix(data, []byte("GIF8")) {
		return bad("no header")
	}
	w, h := int(binary.LittleEndian.Uint16(data[6:])), int(binary.LittleEndian.Uint16(data[8:]))
	decoded := 8 * w * h
	i := 13 + colorTable(data[10])
	frames := 0
	for {
		if i >= len(data) {
			return bad("no trailer")
		}
		switch data[i] {
		case 0x21: // An extension: its label, then its sub-blocks.
			n, ok := subBlocks(data, i+2)
			if !ok {
				return bad("cut extension")
			}
			i = n
		case 0x2c: // A frame: its descriptor, its colors, then its data.
			if i+10 > len(data) {
				return bad("cut frame")
			}
			fw, fh := int(binary.LittleEndian.Uint16(data[i+5:])), int(binary.LittleEndian.Uint16(data[i+7:]))
			frames++
			decoded += fw * fh
			if frames > maxFrames || decoded > maxFramesDecoded {
				return gifWalk{}, fmt.Errorf("%w: over %d frames or %d bytes", errNotAnimated, maxFrames, maxFramesDecoded)
			}
			// The descriptor, the colors, and the LZW code size.
			n, ok := subBlocks(data, i+10+colorTable(data[i+9])+1)
			if !ok {
				return bad("cut frame")
			}
			i = n
		case 0x3b: // The trailer.
			if frames < 2 {
				return gifWalk{}, errNotAnimated
			}
			return gifWalk{width: w, height: h, frames: frames}, nil
		default:
			return bad("unknown block")
		}
	}
}

// colorTable returns the bytes of the color table that flags, the packed
// byte of a screen or a frame, says follows it.
func colorTable(flags byte) int {
	if flags&0x80 == 0 {
		return 0
	}
	return 3 << (flags&7 + 1)
}

// subBlocks returns where the sub-blocks of data from i end, after the
// empty one that ends them, and false if data ends first.
func subBlocks(data []byte, i int) (int, bool) {
	for i < len(data) {
		n := int(data[i])
		i++
		if n == 0 {
			return i, true
		}
		i += n
	}
	return 0, false
}

// onePool keeps the one buffer of a PNG encoder, which encodes the frames
// of an animation one after the other.
type onePool struct{ b *png.EncoderBuffer }

func (p *onePool) Get() *png.EncoderBuffer  { b := p.b; p.b = nil; return b }
func (p *onePool) Put(b *png.EncoderBuffer) { p.b = b }
