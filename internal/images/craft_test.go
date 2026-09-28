package images

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"os"
	"testing"
)

// Images made in code at any size, as small as their format allows, to
// measure what each way of decoding allocates.

// pngChunk appends a PNG chunk to b.
func pngChunk(b *bytes.Buffer, typ string, data []byte) {
	_ = binary.Write(b, binary.BigEndian, uint32(len(data)))
	b.WriteString(typ)
	b.Write(data)
	_ = binary.Write(b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
}

// pngOpts says how craftPNG makes a PNG.
type pngOpts struct {
	depth, colorType byte
	interlaced       bool
	trns             []byte // a tRNS chunk, if not nil
}

// craftPNG returns a PNG of w by h pixels, all zero.
func craftPNG(tb testing.TB, w, h int, o pngOpts) []byte {
	tb.Helper()
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8], ihdr[9] = o.depth, o.colorType
	if o.interlaced {
		ihdr[12] = 1
	}
	pngChunk(&b, "IHDR", ihdr)
	if o.colorType == 3 {
		pngChunk(&b, "PLTE", make([]byte, 3*256))
	}
	if o.trns != nil {
		pngChunk(&b, "tRNS", o.trns)
	}
	bits := map[byte]int{0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[o.colorType] * int(o.depth)
	rows := func(pw, ph int) int {
		if pw == 0 || ph == 0 {
			return 0
		}
		return ph * (1 + (pw*bits+7)/8)
	}
	n := rows(w, h)
	if o.interlaced {
		n = 0
		for _, p := range [7][4]int{{0, 0, 8, 8}, {4, 0, 8, 8}, {0, 4, 4, 8}, {2, 0, 4, 4}, {0, 2, 2, 4}, {1, 0, 2, 2}, {0, 1, 1, 2}} {
			n += rows((w-p[0]+p[2]-1)/p[2], (h-p[1]+p[3]-1)/p[3])
		}
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write(make([]byte, n)); err != nil {
		tb.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}
	pngChunk(&b, "IDAT", z.Bytes())
	pngChunk(&b, "IEND", nil)
	return b.Bytes()
}

// jpegOpts says how flatJPEG makes a JPEG.
type jpegOpts struct {
	progressive bool
	// ids names the components, one byte each: 3 make YCbCr, unless
	// named RGB, and 4 CMYK.
	ids string
	// adobe is the transform of an Adobe APP14 segment, or -1 for none;
	// late puts it after the scans.
	adobe int
	late  bool
}

// flatJPEG returns a JPEG of w by h pixels of one color: every block has
// no coefficients, and its Huffman tables have one code of one bit, so
// each block takes a bit or two.
func flatJPEG(w, h int, o jpegOpts) []byte {
	var b bytes.Buffer
	seg := func(marker byte, data ...byte) {
		b.Write([]byte{0xff, marker})
		_ = binary.Write(&b, binary.BigEndian, uint16(2+len(data)))
		b.Write(data)
	}
	adobe := func() {
		seg(0xee, append([]byte("Adobe"), 0, 100, 0, 0, 0, 0, byte(o.adobe))...)
	}
	n := len(o.ids)
	b.Write([]byte{0xff, 0xd8})
	if o.adobe >= 0 && !o.late {
		adobe()
	}
	seg(0xdb, append([]byte{0}, bytes.Repeat([]byte{1}, 64)...)...)
	sof := make([]byte, 0, 6+3*n)
	sof = append(sof, 8, byte(h>>8), byte(h), byte(w>>8), byte(w), byte(n))
	for i := range n {
		sof = append(sof, o.ids[i], 0x11, 0)
	}
	marker := byte(0xc0)
	if o.progressive {
		marker = 0xc2
	}
	seg(marker, sof...)
	// A DC table and an AC table, each of symbol 0 coded as one bit.
	table := append([]byte{1}, make([]byte, 15)...)
	dht := append(append([]byte{0x00}, table...), 0)
	dht = append(append(append(dht, 0x10), table...), 0)
	seg(0xc4, dht...)
	blocks := ((w + 7) / 8) * ((h + 7) / 8)
	scan := func(ids string, ss, se byte, bits int) {
		s := make([]byte, 0, 1+2*len(ids)+3)
		s = append(s, byte(len(ids)))
		for i := range len(ids) {
			s = append(s, ids[i], 0x00)
		}
		seg(0xda, append(s, ss, se, 0)...)
		b.Write(make([]byte, (bits+7)/8))
	}
	if o.progressive {
		// The DC of all components, then the AC of each: a bit a block.
		scan(o.ids, 0, 0, blocks*n)
		for i := range n {
			scan(o.ids[i:i+1], 1, 63, blocks)
		}
	} else {
		// The DC and an end of block: two bits a block.
		scan(o.ids, 0, 63, 2*blocks*n)
	}
	if o.adobe >= 0 && o.late {
		adobe()
	}
	b.Write([]byte{0xff, 0xd9})
	return b.Bytes()
}

// repeatScans returns d, a JPEG, with its last scan repeated n more times.
func repeatScans(d []byte, n int) []byte {
	last := bytes.LastIndex(d, []byte{0xff, 0xda})
	scan := d[last : len(d)-2]
	out := append([]byte(nil), d[:len(d)-2]...)
	for range n {
		out = append(out, scan...)
	}
	return append(out, 0xff, 0xd9)
}

// hideFrame puts a restart marker after the SOI of a JPEG, which the
// decoder skips without a length, and then a long APP1 holding a fake
// SOF0 where a walk that read a length after the restart marker lands.
func hideFrame(d []byte) []byte {
	const appLen = 0xfff0
	app := make([]byte, 2+appLen)
	app[0], app[1] = 0xff, 0xe1
	binary.BigEndian.PutUint16(app[2:], appLen)
	// Read as a length, the marker ff e1 skips 0xffe1 bytes past it.
	fake := 2 + 0xffe1 - 2
	app[fake], app[fake+1] = 0xff, 0xc0
	out := append([]byte{0xff, 0xd8, 0xff, 0xd0}, app...)
	return append(out, d[2:]...)
}

// bitWriter writes bits least significant first, as VP8L reads them.
type bitWriter struct {
	b []byte
	n uint
}

func (w *bitWriter) put(v uint32, n uint) {
	for i := range n {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		w.b[len(w.b)-1] |= byte(v>>i&1) << (w.n % 8)
		w.n++
	}
}

// trees writes the 5 Huffman codes of a group, each of one symbol, which
// decodes taking no bits.
func (w *bitWriter) trees(green, red, blue, alpha, dist byte) {
	for _, s := range []byte{green, red, blue, alpha, dist} {
		w.put(1, 1) // simple
		w.put(0, 1) // one symbol
		w.put(1, 1) // of 8 bits
		w.put(uint32(s), 8)
	}
}

// vp8lOpts says how flatVP8L makes a lossless WebP.
type vp8lOpts struct {
	// groups claims 65536 groups of Huffman codes through an entropy
	// image, and has none of their codes.
	groups bool
}

// flatVP8L returns a lossless WebP of w by h pixels of one color, a few
// bytes at any size.
func flatVP8L(w, h int, o vp8lOpts) []byte {
	var bw bitWriter
	bw.put(0x2f, 8)
	bw.put(uint32(w-1), 14)
	bw.put(uint32(h-1), 14)
	bw.put(0, 1) // no alpha
	bw.put(0, 3) // version
	bw.put(0, 1) // no more transforms
	bw.put(0, 1) // no color cache
	if o.groups {
		bw.put(1, 1) // an entropy image
		bw.put(0, 3) // of tiles of 4 pixels
		bw.put(0, 1) // no color cache
		// Every tile is of group 0xffff, its red and green.
		bw.trees(0xff, 0xff, 0, 0, 0)
	} else {
		bw.put(0, 1)
		bw.trees(0, 0, 0, 0, 0)
	}
	return riffWebP("VP8L", bw.b)
}

// riffWebP returns a simple WebP of one chunk.
func riffWebP(id string, chunk []byte) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF\x00\x00\x00\x00WEBP")
	b.WriteString(id)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(chunk)))
	b.Write(chunk)
	if len(chunk)%2 == 1 {
		b.WriteByte(0)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}

// lossyWebP returns the lossy WebP of testdata with its frame header
// claiming w by h pixels. Its data then runs out early, after the frame
// is allocated.
func lossyWebP(tb testing.TB, w, h int) []byte {
	tb.Helper()
	b, err := os.ReadFile("testdata/blue-purple-pink.lossy.webp")
	if err != nil {
		tb.Fatal(err)
	}
	// RIFF header, chunk header, frame tag and start code.
	binary.LittleEndian.PutUint16(b[12+8+6:], uint16(w))
	binary.LittleEndian.PutUint16(b[12+8+8:], uint16(h))
	return b
}

// vp8xWrap puts a VP8X chunk claiming a canvas of w by h before the image
// chunk of a simple WebP.
func vp8xWrap(simple []byte, w, h int) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF\x00\x00\x00\x00WEBPVP8X")
	_ = binary.Write(&b, binary.LittleEndian, uint32(10))
	b.Write([]byte{0, 0, 0, 0, byte(w - 1), byte((w - 1) >> 8), byte((w - 1) >> 16), byte(h - 1), byte((h - 1) >> 8), byte((h - 1) >> 16)})
	b.Write(simple[12:])
	out := b.Bytes()
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}

// alphaWebP returns the lossy WebP of testdata, 150 by 100 pixels, with
// an alpha chunk compressed as compression says: 0 plain, 1 lossless.
func alphaWebP(tb testing.TB, compression byte) []byte {
	tb.Helper()
	simple, err := os.ReadFile("testdata/blue-purple-pink.lossy.webp")
	if err != nil {
		tb.Fatal(err)
	}
	const w, h = 150, 100
	alpha := append([]byte{compression}, bytes.Repeat([]byte{0xff}, w*h)...)
	var b bytes.Buffer
	b.WriteString("RIFF\x00\x00\x00\x00WEBPVP8X")
	_ = binary.Write(&b, binary.LittleEndian, uint32(10))
	b.Write([]byte{1 << 4, 0, 0, 0, w - 1, 0, 0, h - 1, 0, 0})
	b.WriteString("ALPH")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(alpha)))
	b.Write(alpha)
	if len(alpha)%2 == 1 {
		b.WriteByte(0)
	}
	b.Write(simple[12:])
	out := b.Bytes()
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}
