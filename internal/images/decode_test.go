package images

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"runtime"
	"testing"
)

func pngOf(tb testing.TB, w, h int) []byte {
	tb.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range m.Pix {
		m.Pix[i] = byte(i)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		tb.Fatal(err)
	}
	return b.Bytes()
}

// bombPNG returns a small PNG whose header claims w by h pixels.
func bombPNG(tb testing.TB, w, h int) []byte {
	tb.Helper()
	return pngHeader(tb, w, h, 8)
}

// pngHeader returns a small PNG whose header claims w by h pixels of RGBA
// at depth bits a sample.
func pngHeader(tb testing.TB, w, h int, depth byte) []byte {
	tb.Helper()
	b := pngOf(tb, 1, 1)
	// The IHDR chunk follows the 8-byte signature: length, type, data,
	// CRC of type and data. Its data is width, height, depth, color type.
	binary.BigEndian.PutUint32(b[16:], uint32(w))
	binary.BigEndian.PutUint32(b[20:], uint32(h))
	b[24] = depth
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

// jpegHeader returns a small JPEG whose frame header claims w by h pixels,
// as a progressive JPEG if progressive is set.
func jpegHeader(tb testing.TB, w, h int, progressive bool) []byte {
	tb.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		tb.Fatal(err)
	}
	b := buf.Bytes()
	i := bytes.Index(b, []byte{0xff, 0xc0})
	if i < 0 {
		tb.Fatal("no SOF0")
	}
	if progressive {
		b[i+1] = 0xc2
	}
	// Marker, length, precision, then height and width.
	binary.BigEndian.PutUint16(b[i+5:], uint16(h))
	binary.BigEndian.PutUint16(b[i+7:], uint16(w))
	return b
}

func TestDecodeFormats(t *testing.T) {
	src := image.NewPaletted(image.Rect(0, 0, 40, 20), color.Palette{color.Black, color.White})
	var j, g bytes.Buffer
	if err := jpeg.Encode(&j, image.NewGray(image.Rect(0, 0, 40, 20)), nil); err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(&g, &gif.GIF{Image: []*image.Paletted{src, src}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	// From golang.org/x/image's testdata.
	webp, err := os.ReadFile("testdata/blue-purple-pink.lossy.webp")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"png": pngOf(t, 40, 20), "jpeg": j.Bytes(), "gif": g.Bytes(), "webp": webp} {
		img, err := decode(data, Box{Cols: 80, Rows: 20})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if img.Format != name {
			t.Errorf("%s: format %q", name, img.Format)
		}
		out, err := png.Decode(bytes.NewReader(img.PNG))
		if err != nil || out.Bounds().Dx() != img.Width || out.Bounds().Dy() != img.Height {
			t.Errorf("%s: sent PNG %v (%v), want %dx%d", name, out.Bounds(), err, img.Width, img.Height)
		}
	}
}

func TestDecodeRefuses(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), ErrFormat},
		{"html", []byte("<html><img src=x></html>"), ErrFormat},
		{"truncated png", pngOf(t, 10, 10)[:40], ErrFormat},
		{"side too long", bombPNG(t, maxSide+1, 1), ErrTooLarge},
		// Within each side, but 25 MP: a bomb refused before decoding.
		{"too many pixels", bombPNG(t, 5000, 5000), ErrTooLarge},
		{"too many bytes", append(pngOf(t, 1, 1), make([]byte, maxBytes)...), ErrTooLarge},
		// 9 MP at 16 bits a sample takes 72 MB decoded.
		{"16-bit png", pngHeader(t, 3000, 3000, 16), ErrTooLarge},
		// 4 MP progressive keeps its coefficients too: about 60 MB.
		{"progressive jpeg", jpegHeader(t, 2000, 2000, true), ErrTooLarge},
	}
	for _, tt := range tests {
		if _, err := decode(tt.data, Box{}); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestFit(t *testing.T) {
	tests := []struct {
		name string
		w, h int
		box  Box
		want Image
	}{
		{"fits", 16, 32, Box{Cols: 10, Rows: 10}, Image{Width: 16, Height: 32, Cols: 2, Rows: 2}},
		{"never up", 4, 4, Box{Cols: 10, Rows: 10}, Image{Width: 4, Height: 4, Cols: 1, Rows: 1}},
		{"by width", 800, 160, Box{Cols: 50, Rows: 20}, Image{Width: 400, Height: 80, Cols: 50, Rows: 5}},
		{"by height", 100, 1000, Box{Cols: 80, Rows: 10}, Image{Width: 16, Height: 160, Cols: 2, Rows: 10}},
		{"cell size", 200, 100, Box{Cols: 10, Rows: 10, CellWidth: 10, CellHeight: 20}, Image{Width: 100, Height: 50, Cols: 10, Rows: 3}},
		{"unlimited to maxSent", 4096, 1024, Box{}, Image{Width: 2048, Height: 512, Cols: 256, Rows: 32}},
		// The cells of the fitted size, filled by fewer pixels.
		{"pixels", 4000, 4000, Box{}, Image{Width: 1024, Height: 1024, Cols: 256, Rows: 128}},
		{"thin", 8000, 1, Box{Cols: 10, Rows: 1}, Image{Width: 80, Height: 1, Cols: 10, Rows: 1}},
	}
	for _, tt := range tests {
		if got := fit(tt.w, tt.h, tt.box); got.Width != tt.want.Width || got.Height != tt.want.Height ||
			got.Cols != tt.want.Cols || got.Rows != tt.want.Rows {
			t.Errorf("%s: fit = %dx%d px %dx%d cells, want %dx%d px %dx%d cells", tt.name,
				got.Width, got.Height, got.Cols, got.Rows, tt.want.Width, tt.want.Height, tt.want.Cols, tt.want.Rows)
		}
	}
}

// What decoding takes is counted by what the header says the image
// decodes to, not only by its pixels.
func TestDecodedBytes(t *testing.T) {
	// Allowed by their size: the data after the header is what fails.
	for name, data := range map[string][]byte{
		"8-bit png of 9 MP":        pngHeader(t, 3000, 3000, 8),
		"16-bit png of 2 MP":       pngHeader(t, 2000, 1000, 16),
		"baseline jpeg of 12 MP":   jpegHeader(t, 4000, 3000, false),
		"progressive jpeg of 3 MP": jpegHeader(t, 2000, 1500, true),
	} {
		if _, err := decode(data, Box{}); errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, ok := jpegFrameOf([]byte{0xff, 0xd8, 0xff}); ok {
		t.Error("a JPEG cut short has a frame")
	}
}

// What decoding and scaling allocate stays within what two decodes at
// once may take, for images as large as allowed into boxes that made the
// sharp scaler's scratch huge, and for each way of decoding that takes
// more than a few bytes a pixel, at about the most pixels its count
// allows.
func TestDecodeMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("decodes large images")
	}
	encode := func(tb testing.TB, format string, w, h int) []byte {
		tb.Helper()
		var (
			b   bytes.Buffer
			err error
		)
		r := image.Rect(0, 0, w, h)
		switch format {
		case "png":
			err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, image.NewNRGBA(r))
		case "jpeg":
			err = jpeg.Encode(&b, image.NewYCbCr(r, image.YCbCrSubsampleRatio420), nil)
		case "gif":
			err = gif.Encode(&b, image.NewPaletted(r, color.Palette{color.Black, color.White}), nil)
		}
		if err != nil {
			tb.Fatal(err)
		}
		return b.Bytes()
	}
	wide := Box{Cols: 256, Rows: 128}
	tests := []struct {
		name string
		data func(tb testing.TB) []byte
		box  Box
	}{
		{"tall", func(tb testing.TB) []byte { tb.Helper(); return encode(tb, "png", 1536, 8192) }, Box{Cols: 80, Rows: 200}},
		{"square into a wide box", func(tb testing.TB) []byte { tb.Helper(); return encode(tb, "png", 3500, 3500) }, wide},
		{"square into a small box", func(tb testing.TB) []byte { tb.Helper(); return encode(tb, "png", 3500, 3500) }, Box{Cols: 40, Rows: 20}},
		{"photo", func(tb testing.TB) []byte { tb.Helper(); return encode(tb, "jpeg", 4000, 3000) }, Box{Cols: 200, Rows: 50}},
		{"gif", func(tb testing.TB) []byte { tb.Helper(); return encode(tb, "gif", 3500, 3500) }, wide},
		// Gray with tRNS decodes to NRGBA, 4 bytes a pixel, or 8 at 16 bits.
		{"gray png with tRNS", func(tb testing.TB) []byte {
			tb.Helper()
			return craftPNG(tb, 3500, 3500, pngOpts{depth: 8, trns: []byte{0, 0}})
		}, wide},
		{"16-bit gray png with tRNS", func(tb testing.TB) []byte {
			tb.Helper()
			return craftPNG(tb, 2500, 2500, pngOpts{depth: 16, trns: []byte{0, 0}})
		}, wide},
		// Interlaced, each pass is an image of its own.
		{"interlaced png", func(tb testing.TB) []byte {
			tb.Helper()
			return craftPNG(tb, 2500, 2500, pngOpts{depth: 8, colorType: 6, interlaced: true})
		}, wide},
		{"interlaced paletted png", func(tb testing.TB) []byte {
			tb.Helper()
			return craftPNG(tb, 4096, 4096, pngOpts{depth: 8, colorType: 3, interlaced: true})
		}, wide},
		// RGB decodes as YCbCr, then converts to RGBA: 7 bytes a pixel.
		{"rgb jpeg", func(testing.TB) []byte { return flatJPEG(2600, 2600, jpegOpts{ids: "RGB", adobe: -1}) }, wide},
		{"adobe rgb jpeg, marked after the scan", func(testing.TB) []byte {
			return flatJPEG(2600, 2600, jpegOpts{ids: "\x01\x02\x03", adobe: 0, late: true})
		}, wide},
		// CMYK decodes as YCbCr and black, then converts: 8.
		{"cmyk jpeg", func(testing.TB) []byte { return flatJPEG(2450, 2450, jpegOpts{ids: "CMYK", adobe: 0}) }, wide},
		{"ycck jpeg", func(testing.TB) []byte { return flatJPEG(2450, 2450, jpegOpts{ids: "\x01\x02\x03\x04", adobe: 2}) }, wide},
		// Progressive keeps 4 bytes of coefficients a sample besides.
		{"progressive jpeg", func(testing.TB) []byte {
			return flatJPEG(1790, 1790, jpegOpts{progressive: true, ids: "\x01\x02\x03", adobe: -1})
		}, wide},
		{"progressive rgb jpeg", func(testing.TB) []byte {
			return flatJPEG(1590, 1590, jpegOpts{progressive: true, ids: "RGB", adobe: -1})
		}, wide},
		{"progressive cmyk jpeg", func(testing.TB) []byte {
			return flatJPEG(1400, 1400, jpegOpts{progressive: true, ids: "CMYK", adobe: 0})
		}, wide},
		{"progressive jpeg behind a restart marker", func(testing.TB) []byte {
			return hideFrame(flatJPEG(1790, 1790, jpegOpts{progressive: true, ids: "\x01\x02\x03", adobe: -1}))
		}, wide},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.data(t)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			img, err := decode(data, tt.box)
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatal(err)
			}
			total := after.TotalAlloc - before.TotalAlloc
			t.Logf("%d KB into %dx%d cells: sent %dx%d, allocated %d MB", len(data)>>10, tt.box.Cols, tt.box.Rows,
				img.Width, img.Height, total>>20)
			if total > maxDecodeAlloc {
				t.Errorf("allocated %d MB, want at most %d", total>>20, maxDecodeAlloc>>20)
			}
		})
	}
}

// maxDecodeAlloc is the most one decode may allocate, by what it counts:
// maxDecoded, and what scaling and encoding the PNG add.
const maxDecodeAlloc = 70 << 20

// Images whose headers hide what they decode to are refused unread.
func TestDecodeRefusesHidden(t *testing.T) {
	wide := Box{Cols: 256, Rows: 128}
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"gray png with tRNS", craftPNG(t, 4096, 4096, pngOpts{depth: 8, trns: []byte{0, 0}}), ErrTooLarge},
		{"16-bit gray png with tRNS", craftPNG(t, 4096, 4096, pngOpts{depth: 16, trns: []byte{0, 0}}), ErrTooLarge},
		{"interlaced png", craftPNG(t, 3400, 3400, pngOpts{depth: 8, colorType: 6, interlaced: true}), ErrTooLarge},
		{"rgb jpeg", flatJPEG(4096, 4096, jpegOpts{ids: "RGB", adobe: -1}), ErrTooLarge},
		{"adobe rgb jpeg, marked after the scan", flatJPEG(3000, 3000, jpegOpts{ids: "\x01\x02\x03", adobe: 0, late: true}), ErrTooLarge},
		{"cmyk jpeg", flatJPEG(3400, 3400, jpegOpts{ids: "CMYK", adobe: 0}), ErrTooLarge},
		{"progressive cmyk jpeg", flatJPEG(1500, 1500, jpegOpts{progressive: true, ids: "CMYK", adobe: 0}), ErrTooLarge},
		// Each scan is a pass over the whole image, which a decode can't
		// stop: thousands would hold it for minutes.
		{"jpeg of many scans", repeatScans(flatJPEG(1024, 1024, jpegOpts{progressive: true, ids: "\x01\x02\x03", adobe: -1}), 1000), ErrTooLarge},
		{"progressive jpeg behind a restart marker",
			hideFrame(flatJPEG(3000, 3000, jpegOpts{progressive: true, ids: "\x01\x02\x03", adobe: -1})), ErrTooLarge},
		// A VP8X canvas of 1x1 around a larger frame, which the decoder
		// would allocate.
		// Lossless WebP isn't shown: its Huffman groups, named by an
		// entropy image, are allocated before any is read, 65536 of
		// them from a file of a few bytes.
		{"lossless webp", flatVP8L(16, 16, vp8lOpts{}), ErrFormat},
		{"lossless webp claiming 65536 groups", flatVP8L(16, 16, vp8lOpts{groups: true}), ErrFormat},
		{"lossless webp in VP8X", vp8xWrap(flatVP8L(16, 16, vp8lOpts{groups: true}), 16, 16), ErrFormat},
		{"lossy webp with lossless alpha", alphaWebP(t, 1), ErrFormat},
		{"lossy webp on a smaller canvas", vp8xWrap(lossyWebP(t, 4096, 4096), 1, 1), ErrFormat},
		{"lossy webp on a larger canvas", vp8xWrap(lossyWebP(t, 100, 100), 4096, 4096), ErrFormat},
	}
	for _, tt := range tests {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := decode(tt.data, wide)
		runtime.ReadMemStats(&after)
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
		if total := after.TotalAlloc - before.TotalAlloc; total > 1<<20 {
			t.Errorf("%s: allocated %d KB before refusing", tt.name, total>>10)
		}
	}
	// Lossy with plain alpha still shows.
	if _, err := decode(alphaWebP(t, 0), wide); err != nil {
		t.Errorf("lossy webp with plain alpha: %v", err)
	}
	// The walk reads the frame the decoder reads.
	if fr, ok := jpegFrameOf(hideFrame(flatJPEG(8, 8, jpegOpts{progressive: true, ids: "\x01\x02\x03", adobe: -1}))); !ok || !fr.progressive {
		t.Errorf("frame behind a restart marker = %+v, %v; want progressive", fr, ok)
	}
}
