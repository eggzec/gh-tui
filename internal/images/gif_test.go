package images

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"slices"
	"testing"
	"time"
)

var (
	red   = color.RGBA{R: 0xff, A: 0xff}
	blue  = color.RGBA{B: 0xff, A: 0xff}
	green = color.RGBA{G: 0xff, A: 0xff}
	white = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	none  = color.RGBA{}
)

// gifPalette has every color of the tests, and a transparent index.
var gifPalette = color.Palette{none, red, blue, green, white}

// frameOf returns a frame over r of c, with the pixels at holes
// transparent.
func frameOf(r image.Rectangle, c color.Color, holes ...image.Point) *image.Paletted {
	m := image.NewPaletted(r, gifPalette)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			m.Set(x, y, c)
		}
	}
	for _, p := range holes {
		m.SetColorIndex(p.X, p.Y, 0)
	}
	return m
}

func encodeGIF(tb testing.TB, g *gif.GIF) []byte {
	tb.Helper()
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		tb.Fatal(err)
	}
	return b.Bytes()
}

// pixels returns the colors of a PNG, row by row.
func pixels(tb testing.TB, data []byte) [][]color.RGBA {
	tb.Helper()
	m, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		tb.Fatal(err)
	}
	b := m.Bounds()
	rows := make([][]color.RGBA, b.Dy())
	for y := range rows {
		for x := range b.Dx() {
			r, g, bl, a := m.At(b.Min.X+x, b.Min.Y+y).RGBA()
			rows[y] = append(rows[y], color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)})
		}
	}
	return rows
}

// unscaled is a box that takes a small image as it is: a pixel a cell.
var unscaled = Box{Cols: 100, Rows: 100, CellWidth: 1, CellHeight: 1, Animate: true}

// Each frame shows as the frames before it left the screen, drawn over by
// its own pixels but those transparent, and then disposed of as it says:
// left, cleared to transparent, or put back as it was before it.
func TestDecodeAnimationDisposal(t *testing.T) {
	screen := image.Config{Width: 4, Height: 2}
	left, right := image.Rect(0, 0, 2, 2), image.Rect(2, 0, 4, 2)
	data := encodeGIF(t, &gif.GIF{
		Image: []*image.Paletted{
			frameOf(image.Rect(0, 0, 4, 2), red),
			frameOf(left, blue),
			frameOf(right, green),
			frameOf(image.Rect(2, 0, 4, 1), white, image.Pt(2, 0)),
		},
		Delay:    []int{10, 10, 10, 10},
		Disposal: []byte{gif.DisposalNone, gif.DisposalBackground, gif.DisposalPrevious, gif.DisposalNone},
		Config:   image.Config{ColorModel: gifPalette, Width: screen.Width, Height: screen.Height},
	})
	img, err := decode(data, unscaled)
	if err != nil {
		t.Fatal(err)
	}
	want := [][][]color.RGBA{
		{{red, red, red, red}, {red, red, red, red}},
		{{blue, blue, red, red}, {blue, blue, red, red}},
		// The blue was cleared after it showed.
		{{none, none, green, green}, {none, none, green, green}},
		// The green was taken back; the hole shows the red beneath.
		{{none, none, red, white}, {none, none, red, red}},
	}
	if len(img.Frames) != len(want) {
		t.Fatalf("%d frames, want %d", len(img.Frames), len(want))
	}
	for i, f := range img.Frames {
		if got := pixels(t, f.PNG); !slices.EqualFunc(got, want[i], slices.Equal) {
			t.Errorf("frame %d = %v, want %v", i, got, want[i])
		}
	}
	if !bytes.Equal(img.PNG, img.Frames[0].PNG) {
		t.Error("the image isn't its first frame")
	}
	if img.Width != 4 || img.Height != 2 || img.Cols != 4 || img.Rows != 2 || img.Format != "gif" {
		t.Errorf("image %dx%d in %dx%d cells as %s, want 4x2 in 4x2 as gif", img.Width, img.Height, img.Cols, img.Rows, img.Format)
	}
}

// The frames of an animation are scaled down as its first frame would be.
func TestDecodeAnimationScaled(t *testing.T) {
	r := image.Rect(0, 0, 40, 20)
	data := encodeGIF(t, &gif.GIF{
		Image: []*image.Paletted{frameOf(r, red), frameOf(r, blue)},
		Delay: []int{10, 10},
	})
	box := Box{Cols: 2, Rows: 1, CellWidth: 10, CellHeight: 10, Animate: true}
	img, err := decode(data, box)
	if err != nil {
		t.Fatal(err)
	}
	still, err := decode(data, Box{Cols: 2, Rows: 1, CellWidth: 10, CellHeight: 10})
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != still.Width || img.Height != still.Height || img.Cols != still.Cols || img.Rows != still.Rows {
		t.Errorf("animation %dx%d in %dx%d, want the still's %dx%d in %dx%d",
			img.Width, img.Height, img.Cols, img.Rows, still.Width, still.Height, still.Cols, still.Rows)
	}
	for i, f := range img.Frames {
		if p := pixels(t, f.PNG); len(p) != img.Height || len(p[0]) != img.Width {
			t.Errorf("frame %d is %dx%d, want %dx%d", i, len(p[0]), len(p), img.Width, img.Height)
		}
	}
}

// A frame shows for its delay, but one of a hundredth of a second or less,
// as many say to mean soon, shows for a tenth, as in browsers; the loop
// count says how many times the frames play.
func TestDecodeAnimationTiming(t *testing.T) {
	r := image.Rect(0, 0, 2, 2)
	frames := []*image.Paletted{frameOf(r, red), frameOf(r, blue), frameOf(r, green), frameOf(r, white)}
	for _, tt := range []struct {
		loop, want int
	}{{0, 0}, {-1, 1}, {1, 2}, {4, 5}} {
		data := encodeGIF(t, &gif.GIF{Image: frames, Delay: []int{0, 1, 5, 20}, LoopCount: tt.loop})
		img, err := decode(data, unscaled)
		if err != nil {
			t.Fatal(err)
		}
		var delays []time.Duration
		for _, f := range img.Frames {
			delays = append(delays, f.Delay)
		}
		if want := []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 50 * time.Millisecond, 200 * time.Millisecond}; !slices.Equal(delays, want) {
			t.Errorf("delays %v, want %v", delays, want)
		}
		if img.Loops != tt.want {
			t.Errorf("a loop count of %d plays %d times, want %d", tt.loop, img.Loops, tt.want)
		}
	}
}

// A GIF shows its first frame alone when the box doesn't ask for the
// frames, when it has one, or past the limits on frames; so does one
// whose later frames don't decode, as it did before animations.
func TestDecodeAnimationFirstFrame(t *testing.T) {
	r := image.Rect(0, 0, 2, 2)
	two := encodeGIF(t, &gif.GIF{Image: []*image.Paletted{frameOf(r, red), frameOf(r, blue)}, Delay: []int{10, 10}})
	many := &gif.GIF{}
	for range maxFrames + 1 {
		many.Image, many.Delay = append(many.Image, frameOf(r, red)), append(many.Delay, 10)
	}
	broken := slices.Clone(two)
	// The second frame's data is cut short, before the trailer.
	broken = append(broken[:len(broken)-6], 0x3b)
	still := unscaled
	still.Animate = false
	for _, tt := range []struct {
		name string
		data []byte
		box  Box
	}{
		{"not asked", two, still},
		{"one frame", encodeGIF(t, &gif.GIF{Image: []*image.Paletted{frameOf(r, red)}, Delay: []int{10}}), unscaled},
		{"too many frames", encodeGIF(t, many), unscaled},
		{"later frame broken", broken, unscaled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			img, err := decode(tt.data, tt.box)
			if err != nil {
				t.Fatal(err)
			}
			if len(img.Frames) != 0 || img.Loops != 0 {
				t.Errorf("%d frames, %d loops, want the first frame alone", len(img.Frames), img.Loops)
			}
			if got := pixels(t, img.PNG); got[0][0] != red {
				t.Errorf("shows %v, want the red first frame", got[0][0])
			}
		})
	}
}

// An animation whose frames, at the size they are sent at, would take
// more than its bound in the terminal, which keeps them decoded, shows its
// first frame; fitted to a smaller box, the same GIF animates.
func TestDecodeAnimationShownBytes(t *testing.T) {
	// Nine frames of 1000x1000 take 36 MB shown; of 100x100, 360 KB.
	g := &gif.GIF{Config: image.Config{ColorModel: gifPalette, Width: 1000, Height: 1000}}
	for range 9 {
		g.Image, g.Delay = append(g.Image, frameOf(image.Rect(0, 0, 1, 1), red)), append(g.Delay, 10)
	}
	data := encodeGIF(t, g)
	img, err := decode(data, Box{Cols: 1000, Rows: 1000, CellWidth: 1, CellHeight: 1, Animate: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 0 || len(img.PNG) == 0 {
		t.Errorf("at 1000x1000: %d frames, want the first frame alone", len(img.Frames))
	}
	img, err = decode(data, Box{Cols: 100, Rows: 100, CellWidth: 1, CellHeight: 1, Animate: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(img.Frames) != 9 || img.Width != 100 {
		t.Errorf("at 100x100: %d frames of %dx%d, want 9", len(img.Frames), img.Width, img.Height)
	}
	if shown := 9 * 1000 * 1000 * 4; shown <= maxFramesShown {
		t.Fatalf("the test's large animation takes %d bytes shown, within the bound", shown)
	}
}

// craftGIF returns the blocks of a GIF of a w by h screen with a frame of
// each size, whose data is a single empty sub-block: the walk reads only
// the blocks' sizes.
func craftGIF(w, h int, frames ...image.Point) []byte {
	le := func(n int) []byte { return []byte{byte(n), byte(n >> 8)} }
	b := append([]byte("GIF89a"), le(w)...)
	b = append(b, le(h)...)
	// A global table of 2 colors, and a comment, to skip.
	b = append(b, 0x80, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0x21, 0xfe, 3, 'h', 'i', '!', 0)
	for _, f := range frames {
		b = append(b, 0x21, 0xf9, 4, 0, 10, 0, 0, 0, 0x2c, 0, 0, 0, 0)
		b = append(b, le(f.X)...)
		b = append(b, le(f.Y)...)
		// A local table of 4 colors, the code size, one data block.
		b = append(b, 0x81, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 2, 1, 0x44, 0)
	}
	return append(b, 0x3b)
}

// The walk counts the frames from the blocks alone, and refuses one past
// the limits, by frames or by what the frames and the canvases decode to,
// or whose blocks it can't walk.
func TestGIFFrames(t *testing.T) {
	big := image.Pt(2048, 2048)
	for _, tt := range []struct {
		name string
		data []byte
		n    int
		err  error
	}{
		{"two", craftGIF(10, 10, image.Pt(10, 10), image.Pt(5, 5)), 2, nil},
		{"one", craftGIF(10, 10, image.Pt(10, 10)), 0, errNotAnimated},
		{"at the most frames", craftGIF(1, 1, slices.Repeat([]image.Point{{1, 1}}, maxFrames)...), maxFrames, nil},
		{"past the most frames", craftGIF(1, 1, slices.Repeat([]image.Point{{1, 1}}, maxFrames+1)...), 0, errNotAnimated},
		// 8 bytes a pixel of canvas and 1 a pixel of each frame: 4 frames
		// fill the bytes exactly, and a fifth is past them.
		{"within the bytes", craftGIF(big.X, big.Y, big, big, big, big), 4, nil},
		{"past the bytes", craftGIF(big.X, big.Y, big, big, big, big, big), 0, errNotAnimated},
		{"no header", []byte("PNG89a......."), 0, ErrFormat},
		{"cut", craftGIF(10, 10, image.Pt(10, 10), image.Pt(10, 10))[:40], 0, ErrFormat},
		{"no trailer", craftGIF(10, 10, image.Pt(10, 10), image.Pt(10, 10))[:57], 0, ErrFormat},
		{"unknown block", append(craftGIF(10, 10, image.Pt(10, 10))[:28], 0x99), 0, ErrFormat},
	} {
		t.Run(tt.name, func(t *testing.T) {
			walk, err := gifFrames(tt.data)
			n := walk.frames
			if n != tt.n || !errors.Is(err, tt.err) || (tt.err == nil) != (err == nil) {
				t.Errorf("gifFrames = %d, %v, want %d, %v", n, err, tt.n, tt.err)
			}
		})
	}
}

// The walk reads any bytes without panicking.
func FuzzGIFFrames(f *testing.F) {
	f.Add(craftGIF(10, 10, image.Pt(10, 10), image.Pt(5, 5)))
	f.Add([]byte("GIF89a\x01\x00\x01\x00\xff\x00\x00\x2c"))
	f.Fuzz(func(t *testing.T, data []byte) {
		walk, err := gifFrames(data)
		if n := walk.frames; err == nil && (n < 2 || n > maxFrames) {
			t.Errorf("%d frames without an error", n)
		}
	})
}

// The memory counts every frame of an animation, not only the first.
func TestMemoryCountsFrames(t *testing.T) {
	m := newMemory(10)
	anim := Image{PNG: []byte("1234"), Frames: []Frame{{PNG: []byte("1234")}, {PNG: []byte("5678")}, {PNG: []byte("9abc")}}}
	m.put("anim", anim)
	if _, ok := m.get("anim"); ok {
		t.Error("kept an animation of 12 bytes in a memory of 10")
	}
	m.put("still", Image{PNG: []byte("1234")})
	m.put("anim", Image{PNG: []byte("12"), Frames: []Frame{{PNG: []byte("12")}, {PNG: []byte("34")}}})
	if m.size != 8 {
		t.Errorf("memory holds %d bytes, want 8", m.size)
	}
}
