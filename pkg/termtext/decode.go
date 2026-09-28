package termtext

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Byte order marks.
const (
	bomUTF8    = "\xef\xbb\xbf"
	bomUTF16LE = "\xff\xfe"
	bomUTF16BE = "\xfe\xff"
)

// Decode returns src as UTF-8, whatever encoding it was read in, as far as
// it can tell: UTF-8 as it is; UTF-16 by its byte order mark; text that is
// mostly not UTF-8, such as Latin-1, as Windows-1252, its superset, the way
// browsers read both; and any byte left that is none of these as <XX>, its
// value in hex, the way less shows it. A byte order mark is dropped. The
// result is valid UTF-8, so its characters, wide ones too, show in their
// own width; control characters are left to [Clean].
func Decode(src string) string {
	switch {
	case strings.HasPrefix(src, bomUTF16LE):
		return fromUTF16(src[len(bomUTF16LE):], false)
	case strings.HasPrefix(src, bomUTF16BE):
		return fromUTF16(src[len(bomUTF16BE):], true)
	}
	src = strings.TrimPrefix(src, bomUTF8)
	if utf8.ValidString(src) {
		return src
	}
	if legacy(src) {
		return fromWindows1252(src)
	}
	var b strings.Builder
	b.Grow(len(src) + len(src)/8)
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		if r == utf8.RuneError && size == 1 {
			writeHex(&b, src[i])
		} else {
			b.WriteString(src[i : i+size])
		}
		i += size
	}
	return b.String()
}

// UTF16 reports whether src starts with the byte order mark of UTF-16,
// which [Decode] reads it by. Such text holds NUL bytes, so a check for
// binary content should decode it first.
func UTF16(src string) bool {
	return strings.HasPrefix(src, bomUTF16LE) || strings.HasPrefix(src, bomUTF16BE)
}

// legacy reports whether src, which isn't valid UTF-8, is more likely in
// a legacy 8-bit encoding: it has more bytes that aren't UTF-8 than
// characters that are, beyond ASCII.
func legacy(src string) bool {
	invalid, valid := 0, 0
	for i := 0; i < len(src); {
		if src[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(src[i:])
		if r == utf8.RuneError && size == 1 {
			invalid++
		} else {
			valid++
		}
		i += size
	}
	return invalid > valid
}

// windows1252 holds the characters of bytes 0x80 to 0x9f in Windows-1252,
// where Latin-1 has control characters. Five bytes are undefined, 0.
var windows1252 = [32]rune{
	'€', 0, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0, 'Ž', 0,
	0, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0, 'ž', 'Ÿ',
}

// fromWindows1252 decodes src from Windows-1252, and shows the bytes it
// leaves undefined as <XX>.
func fromWindows1252(src string) string {
	var b strings.Builder
	b.Grow(len(src) + len(src)/4)
	for i := range len(src) {
		c := src[i]
		switch {
		case c < utf8.RuneSelf:
			b.WriteByte(c)
		case c >= 0xa0:
			// Latin-1 is the first 256 code points.
			b.WriteRune(rune(c))
		case windows1252[c-0x80] != 0:
			b.WriteRune(windows1252[c-0x80])
		default:
			writeHex(&b, c)
		}
	}
	return b.String()
}

// fromUTF16 decodes src from UTF-16, big-endian if big is set. A lone
// surrogate turns into U+FFFD, and a last odd byte shows as <XX>.
func fromUTF16(src string, big bool) string {
	units := make([]uint16, len(src)/2)
	for i := range units {
		a, z := uint16(src[2*i]), uint16(src[2*i+1])
		if big {
			units[i] = a<<8 | z
		} else {
			units[i] = z<<8 | a
		}
	}
	var b strings.Builder
	b.Grow(len(src))
	for _, r := range utf16.Decode(units) {
		b.WriteRune(r)
	}
	if len(src)%2 == 1 {
		writeHex(&b, src[len(src)-1])
	}
	return b.String()
}

// writeHex writes byte c as <XX>.
func writeHex(b *strings.Builder, c byte) {
	const digits = "0123456789ABCDEF"
	b.WriteByte('<')
	b.WriteByte(digits[c>>4])
	b.WriteByte(digits[c&0xf])
	b.WriteByte('>')
}
