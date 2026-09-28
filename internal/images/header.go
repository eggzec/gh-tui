package images

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
)

// What the decoders allocate, read from the headers they read. Each reader
// walks the data as its decoder does, so a header hidden from a simpler
// walk can't make an image look cheaper than it decodes. Counts are upper
// bounds, measured against the decoders: Go's image/png, image/jpeg and
// image/gif, and golang.org/x/image/webp.

// decodedBytes returns the most bytes the decoder of format allocates for
// data, an image whose header, by its codec's DecodeConfig, is cfg.
func decodedBytes(format string, cfg image.Config, data []byte) (int, error) {
	pixels := cfg.Width * cfg.Height
	switch format {
	case "png":
		return pixels * pngBytes(data), nil
	case "jpeg":
		// The decoder walks every block of the image for each scan and
		// can't be stopped, so a file of thousands of scans would hold a
		// decode for minutes. Real files have about ten. A marker can't
		// hide in entropy data, which writes an ff as ff 00, so the count
		// is at least the scans the decoder reads.
		if n := bytes.Count(data, []byte{0xff, jpegSOS}); n > maxScans {
			return 0, fmt.Errorf("%w: jpeg of %d scans", ErrTooLarge, n)
		}
		return jpegBytes(data, cfg.Width, cfg.Height), nil
	case "webp":
		return webpBytes(data, cfg.Width, cfg.Height)
	}
	// A gif's first frame is paletted, a byte a pixel, and lies within
	// its screen.
	return pixels, nil
}

// pngBytes returns how many bytes a pixel of data, a PNG, takes as it
// decodes. Its IHDR gives the depth and color type, and a tRNS chunk
// before the first IDAT makes gray decode as NRGBA, 4 or 8 bytes a pixel.
// An interlaced image decodes each pass into an image of its own before
// merging them into the whole, which costs as much again.
func pngBytes(data []byte) int {
	const sig = 8
	// IHDR: length, type, then width, height, depth, color type,
	// compression, filter and interlace.
	if len(data) < sig+8+13 {
		return 16
	}
	ihdr := data[sig+8:]
	depth, colorType, interlaced := ihdr[8], ihdr[9], ihdr[12] != 0
	trns := false
	for i := sig; i+8 <= len(data); i += 12 + int(binary.BigEndian.Uint32(data[i:])) {
		typ := string(data[i+4 : i+8])
		if typ == "IDAT" {
			break
		}
		if typ == "tRNS" {
			trns = true
			break
		}
	}
	// Paletted, and gray without tRNS, take a sample a pixel; the rest
	// decode as RGBA or NRGBA, 4 samples. A 16-bit sample takes 2 bytes.
	n := 4
	if colorType == 3 || colorType == 0 && !trns {
		n = 1
	}
	if depth == 16 {
		n *= 2
	}
	if interlaced {
		n *= 2
	}
	return n
}

// jpegBytes returns the most bytes data, a JPEG of w by h pixels, takes
// as it decodes. Each component decodes to a byte a sample, counted as if
// none were subsampled, in blocks that may pad each side by up to 31
// pixels. A progressive JPEG also keeps 64 int32 coefficients for each 64
// samples of each component. A CMYK JPEG, or one whose components are
// RGB, is then converted to an image of 4 bytes a pixel.
func jpegBytes(data []byte, w, h int) int {
	fr, ok := jpegFrameOf(data)
	if !ok {
		// The costliest: progressive CMYK.
		fr = jpegFrame{progressive: true, comps: 4}
	}
	n := fr.comps
	if fr.progressive {
		n += 4 * fr.comps
	}
	if fr.comps == 4 || fr.comps == 3 && (fr.rgb || adobeUnknown(data)) {
		n += 4
	}
	return (w + 31) * (h + 31) * n
}

// jpegFrame is what the frame header of a JPEG says of how it decodes.
type jpegFrame struct {
	progressive bool
	comps       int
	// rgb says the components are named R, G and B, which decode as
	// RGB.
	rgb bool
}

// JPEG markers.
const (
	jpegSOF0 = 0xc0
	jpegSOF1 = 0xc1
	jpegSOF2 = 0xc2
	jpegRST0 = 0xd0
	jpegRST7 = 0xd7
	jpegEOI  = 0xd9
	jpegSOS  = 0xda
)

// jpegFrameOf reads the frame header of data, a JPEG, the one image/jpeg
// decodes by: the first SOF0, SOF1 or SOF2, which must come before the
// first scan. It walks the markers before it as image/jpeg does, which
// skips bytes that aren't a marker, takes ff 00 for data, and reads no
// length after ff fill bytes or a restart marker. It reports false if it
// finds none.
func jpegFrameOf(data []byte) (jpegFrame, bool) {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return jpegFrame{}, false
	}
	for i := 2; ; {
		if i+2 > len(data) {
			return jpegFrame{}, false
		}
		t0, t1 := data[i], data[i+1]
		i += 2
		for t0 != 0xff {
			if i >= len(data) {
				return jpegFrame{}, false
			}
			t0, t1 = t1, data[i]
			i++
		}
		marker := t1
		if marker == 0 {
			continue
		}
		for marker == 0xff {
			if i >= len(data) {
				return jpegFrame{}, false
			}
			marker = data[i]
			i++
		}
		if marker == jpegEOI || marker == jpegSOS {
			// No frame, which the decoder refuses.
			return jpegFrame{}, false
		}
		if jpegRST0 <= marker && marker <= jpegRST7 {
			continue
		}
		if i+2 > len(data) {
			return jpegFrame{}, false
		}
		n := int(data[i])<<8 + int(data[i+1]) - 2
		i += 2
		if n < 0 || i+n > len(data) {
			return jpegFrame{}, false
		}
		switch marker {
		case jpegSOF0, jpegSOF1, jpegSOF2:
			// Precision, height, width, the number of components, then
			// 3 bytes for each: its ID, sampling and table.
			seg := data[i : i+n]
			if len(seg) < 6 {
				return jpegFrame{}, false
			}
			fr := jpegFrame{progressive: marker == jpegSOF2, comps: int(seg[5])}
			if fr.comps == 3 && len(seg) >= 6+3*3 {
				fr.rgb = seg[6] == 'R' && seg[9] == 'G' && seg[12] == 'B'
			}
			return fr, true
		}
		i += n
	}
}

// adobeUnknown reports whether data holds an Adobe APP14 segment whose
// transform is 0, unknown, which makes a 3-component JPEG decode as RGB.
// The decoder reads one anywhere before the end, after the scans too, so
// it is looked for in all the bytes: one found inside other data only
// overcounts.
func adobeUnknown(data []byte) bool {
	// Marker, length, "Adobe", version, two flags, then the transform.
	const app14, transform = "\xff\xeeXXAdobe", 4 + 11
	for i := 0; ; {
		j := bytes.Index(data[i:], []byte(app14[:2]))
		if j < 0 {
			return false
		}
		i += j
		if i+transform < len(data) && string(data[i+4:i+9]) == app14[4:] && data[i+transform] == 0 {
			return true
		}
		i += 2
	}
}

// webpBytes returns the most bytes data, a WebP whose canvas is w by h
// pixels, takes as it decodes, from the image the decoder decodes: the
// first VP8 chunk, whose own frame header gives its size. It refuses a
// frame of another size than the canvas, which the decoder would allocate
// unchecked.
//
// A lossy frame decodes to YCbCr 4:2:0 in macroblocks of 16 pixels, and
// copies its partitions, the chunk's bytes. An alpha chunk stored plain
// decodes to a byte a pixel. Lossless images aren't shown, nor alpha
// compressed losslessly: x/image/vp8l allocates the Huffman groups its
// entropy image names, about 170 MB from a file of a few bytes, before
// it reads any of them, which no header shows.
func webpBytes(data []byte, w, h int) (int, error) {
	const (
		riffHeader = 12
		alphaBit   = 1 << 4
	)
	var (
		vp8x, wantAlpha bool
		alpha           int // bytes of the alpha chunk's decoding
	)
	for i := riffHeader; ; {
		if i+8 > len(data) {
			return 0, fmt.Errorf("%w: webp: no image", ErrFormat)
		}
		id := string(data[i : i+4])
		n := int(binary.LittleEndian.Uint32(data[i+4:]))
		i += 8
		if n < 0 || n > len(data)-i {
			return 0, fmt.Errorf("%w: webp: chunk %q runs past the data", ErrFormat, id)
		}
		chunk := data[i : i+n]
		i += n + n&1
		switch id {
		case "VP8X":
			if vp8x || len(chunk) != 10 {
				return 0, fmt.Errorf("%w: webp: bad VP8X", ErrFormat)
			}
			vp8x, wantAlpha = true, chunk[0]&alphaBit != 0
		case "ALPH":
			if !wantAlpha || len(chunk) < 1 {
				return 0, fmt.Errorf("%w: webp: bad ALPH", ErrFormat)
			}
			wantAlpha = false
			switch chunk[0] & 3 {
			case 0:
				alpha = w * h
			case 1:
				return 0, fmt.Errorf("%w: webp: lossless alpha", ErrFormat)
			default:
				return 0, fmt.Errorf("%w: webp: bad ALPH", ErrFormat)
			}
		case "VP8 ":
			fw, fh, ok := vp8Size(chunk)
			if !ok {
				return 0, fmt.Errorf("%w: webp: bad VP8 frame header", ErrFormat)
			}
			if err := sameSize(vp8x, w, h, fw, fh); err != nil {
				return 0, err
			}
			return alpha + (fw+15)*(fh+15)*3/2 + len(chunk), nil
		case "VP8L":
			return 0, fmt.Errorf("%w: webp: lossless", ErrFormat)
		}
	}
}

// sameSize refuses a WebP frame of fw by fh pixels on a canvas of w by h
// that a VP8X chunk declared, unless they match.
func sameSize(vp8x bool, w, h, fw, fh int) error {
	if vp8x && (fw != w || fh != h) {
		return fmt.Errorf("%w: webp: %dx%d frame on a %dx%d canvas", ErrFormat, fw, fh, w, h)
	}
	return checkSize(fw, fh)
}

// vp8Size reads the size of a VP8 key frame: a 3-byte frame tag, a start
// code, then 14 bits each of width and height, little-endian, each
// followed by 2 bits of scaling, which the decoder ignores.
func vp8Size(chunk []byte) (w, h int, ok bool) {
	if len(chunk) < 10 || chunk[0]&1 != 0 || chunk[3] != 0x9d || chunk[4] != 0x01 || chunk[5] != 0x2a {
		return 0, 0, false
	}
	return int(binary.LittleEndian.Uint16(chunk[6:]) & 0x3fff), int(binary.LittleEndian.Uint16(chunk[8:]) & 0x3fff), true
}
