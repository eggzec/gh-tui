package images

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Limits of what is decoded, so a hostile image can't take the memory or
// the time of the app.
const (
	// maxBytes is the most bytes an image may take, as fetched.
	maxBytes = 10 << 20
	// maxSide is the most pixels either side of an image may have.
	maxSide = 8192
	// maxPixels is the most pixels an image may have. The header says,
	// so a small file that would decode to a huge image, a decompression
	// bomb, is refused unread.
	maxPixels = 16 << 20
	// maxDecoded is the most bytes an image may take as it decodes, by
	// what its headers say its decoder allocates. With what scaling and
	// encoding the PNG add, one decode allocates at most about 70 MB
	// (TestDecodeMemory), so the maxDecodes images decoded at once, with
	// their bytes fetched, take about 160 MB at most.
	maxDecoded = 48 << 20
	// maxSent is the most pixels either side of an image sent to the
	// terminal may have.
	maxSent = 2048
	// maxScans is the most scans a JPEG may have, each of which costs a
	// pass over the whole image as it decodes.
	maxScans = 64
	// maxSentPixels is the most pixels an image sent may have. The
	// terminal scales it to its cells, so this costs little sharpness,
	// and it bounds what scaling allocates.
	maxSentPixels = 1 << 20
	// maxScratch is the most bytes the final, sharper scaling may take
	// for its scratch: 32 for each destination column and source row.
	// Beyond it, the image is scaled without scratch.
	maxScratch   = 16 << 20
	scratchBytes = 32
)

// Cell size in pixels when the terminal doesn't say.
const (
	defaultCellWidth  = 8
	defaultCellHeight = 16
)

// Box is the room an image may take, in cells, and the size of a cell in
// pixels. An image is scaled down to fit, keeping its aspect, never up.
// A side of 0 cells doesn't limit the image; a cell size of 0 is 8 by 16.
type Box struct {
	Cols, Rows            int
	CellWidth, CellHeight int
}

func (b Box) cell() (w, h int) {
	w, h = b.CellWidth, b.CellHeight
	if w <= 0 || h <= 0 {
		return defaultCellWidth, defaultCellHeight
	}
	return w, h
}

// Image is an image ready to send to the terminal: a PNG of Width by
// Height pixels that fits Cols by Rows cells of its box.
type Image struct {
	PNG           []byte
	Width, Height int
	Cols, Rows    int
	// Format is what the image was fetched as: png, jpeg, gif or webp,
	// lossy. Of a gif, only the first frame is kept.
	Format string
}

// codec decodes a format of image.
type codec struct {
	name   string
	config func(io.Reader) (image.Config, error)
	decode func(io.Reader) (image.Image, error)
}

// codecs are the formats shown, by the content type the data sniffs as.
// SVG isn't one: it would need a renderer, and is mostly badges.
var codecs = map[string]codec{
	"image/png":  {"png", png.DecodeConfig, png.Decode},
	"image/jpeg": {"jpeg", jpeg.DecodeConfig, jpeg.Decode},
	// Decode reads only the first frame.
	"image/gif": {"gif", gif.DecodeConfig, gif.Decode},
	// Lossy only: lossless WebP is refused by its header (webpBytes).
	"image/webp": {"webp", webp.DecodeConfig, webp.Decode},
}

// inspect returns the codec of data, after checking from its header that
// it would decode within the limits. The format comes from the data,
// never from the name or a header.
func inspect(data []byte) (codec, error) {
	if len(data) > maxBytes {
		return codec{}, fmt.Errorf("%w: %d bytes", ErrTooLarge, len(data))
	}
	c, ok := codecs[http.DetectContentType(data)]
	if !ok {
		return codec{}, ErrFormat
	}
	cfg, err := c.config(bytes.NewReader(data))
	if err != nil {
		return codec{}, fmt.Errorf("%w: %s: %w", ErrFormat, c.name, err)
	}
	if err := checkSize(cfg.Width, cfg.Height); err != nil {
		return codec{}, err
	}
	n, err := decodedBytes(c.name, cfg, data)
	if err != nil {
		return codec{}, err
	}
	if n > maxDecoded {
		return codec{}, fmt.Errorf("%w: %dx%d pixels take %d bytes decoded", ErrTooLarge, cfg.Width, cfg.Height, n)
	}
	return c, nil
}

// decode makes data an image that fits box, after inspecting it.
func decode(data []byte, box Box) (Image, error) {
	c, err := inspect(data)
	if err != nil {
		return Image{}, err
	}
	src, err := c.decode(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("%w: %s: %w", ErrFormat, c.name, err)
	}
	// A frame of a gif may claim more than its header did.
	b := src.Bounds()
	if err := checkSize(b.Dx(), b.Dy()); err != nil {
		return Image{}, err
	}
	img := fit(b.Dx(), b.Dy(), box)
	img.Format = c.name
	// Only the smaller copy is kept, so the decoded image can go before
	// the final scaling allocates.
	src = shrink(src, img.Width, img.Height)
	dst := image.NewRGBA(image.Rect(0, 0, img.Width, img.Height))
	sb := src.Bounds()
	var k draw.Scaler = draw.CatmullRom
	if img.Width*sb.Dy()*scratchBytes > maxScratch {
		k = draw.ApproxBiLinear
	}
	k.Scale(dst, dst.Bounds(), src, sb, draw.Src, nil)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return Image{}, fmt.Errorf("encode image: %w", err)
	}
	img.PNG = out.Bytes()
	return img, nil
}

// shrink returns src scaled down to at most twice w by h, without the
// scratch that a sharper scaler takes for each row of the source, or src
// if it is no larger.
func shrink(src image.Image, w, h int) image.Image {
	b := src.Bounds()
	mw, mh := min(b.Dx(), 2*w), min(b.Dy(), 2*h)
	if mw == b.Dx() && mh == b.Dy() {
		return src
	}
	mid := image.NewRGBA(image.Rect(0, 0, mw, mh))
	draw.ApproxBiLinear.Scale(mid, mid.Bounds(), src, b, draw.Src, nil)
	return mid
}

func checkSize(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("%w: empty", ErrFormat)
	}
	if w > maxSide || h > maxSide || w*h > maxPixels {
		return fmt.Errorf("%w: %dx%d pixels", ErrTooLarge, w, h)
	}
	return nil
}

// fit returns the size an image of w by h pixels is sent at, and the cells
// it takes, to fit box: scaled down only, its aspect kept, at least one
// pixel and one cell.
func fit(w, h int, box Box) Image {
	cw, ch := box.cell()
	maxW, maxH := maxSent, maxSent
	if box.Cols > 0 {
		maxW = min(maxW, box.Cols*cw)
	}
	if box.Rows > 0 {
		maxH = min(maxH, box.Rows*ch)
	}
	scale := min(1, float64(maxW)/float64(w), float64(maxH)/float64(h))
	tw := max(1, min(maxW, int(float64(w)*scale+0.5)))
	th := max(1, min(maxH, int(float64(h)*scale+0.5)))
	img := Image{Cols: max(1, ceilDiv(tw, cw)), Rows: max(1, ceilDiv(th, ch))}
	// The cells stay those of the size fitted; fewer pixels fill them.
	if tw*th > maxSentPixels {
		s := math.Sqrt(float64(maxSentPixels) / float64(tw*th))
		tw, th = max(1, int(float64(tw)*s)), max(1, int(float64(th)*s))
	}
	img.Width, img.Height = tw, th
	return img
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }
