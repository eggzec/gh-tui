package termimg

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestSequences(t *testing.T) {
	id := NewID(7, 42) // 7<<24 | 42
	tests := []struct{ name, got, want string }{
		{"transmit", Transmit(id, []byte("PNGDATA"), 16, 8),
			"\x1b_Ga=t,f=100,t=d,i=117440554,s=16,v=8,q=2;UE5HREFUQQ==\x1b\\"},
		{"transmit nothing", Transmit(id, nil, 1, 1), ""},
		{"transmit without sizes", Transmit(id, []byte("PNGDATA"), -1, 0),
			"\x1b_Ga=t,f=100,t=d,i=117440554,q=2;UE5HREFUQQ==\x1b\\"},
		{"place", Place(id, 4, 2), "\x1b_Ga=p,U=1,i=117440554,p=1,c=4,r=2,q=2\x1b\\"},
		{"place clamped", Place(id, 0, 1000), "\x1b_Ga=p,U=1,i=117440554,p=1,c=1,r=297,q=2\x1b\\"},
		{"delete", Delete(id), "\x1b_Ga=d,d=I,i=117440554,q=2\x1b\\"},
		{"query", Query(31), "\x1b_Ga=q,i=31,s=1,v=1,t=d,f=24;AAAA\x1b\\"},
		{"tmux", Tmux(Delete(id)), "\x1bPtmux;\x1b\x1b_Ga=d,d=I,i=117440554,q=2\x1b\x1b\\\x1b\\"},
		{"tmux trailing text", Tmux(Delete(id) + "x\x1b[2J"),
			"\x1bPtmux;\x1b\x1b_Ga=d,d=I,i=117440554,q=2\x1b\x1b\\\x1b\\\x1bPtmux;x\x1b\x1b[2J\x1b\\"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.name, tt.got, tt.want)
		}
	}
}

var apc = regexp.MustCompile("\x1b_G([^;\x1b]*)(?:;([^\x1b]*))?\x1b\\\\")

func TestTransmitChunks(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		chunks []int // the base64 bytes of each chunk
	}{
		{"one byte", 1, []int{4}},
		{"one full chunk", 3072, []int{4096}},
		{"just over a chunk", 3073, []int{4096, 4}},
		{"two full chunks", 6144, []int{4096, 4096}},
		{"three chunks", 7000, []int{4096, 4096, 1144}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := bytes.Repeat([]byte{0xa5, 0x5a, 0x00}, tt.size/3+1)[:tt.size]
			seq := Transmit(NewID(1, 2), data, 3, 4)
			ms := apc.FindAllStringSubmatch(seq, -1)
			if strings.Join(flatten(ms), "") != seq {
				t.Fatalf("not only kitty sequences: %q", seq)
			}
			if len(ms) != len(tt.chunks) {
				t.Fatalf("%d chunks, want %d", len(ms), len(tt.chunks))
			}
			var payload strings.Builder
			for i, m := range ms {
				keys, part := m[1], m[2]
				if len(part) != tt.chunks[i] {
					t.Errorf("chunk %d holds %d bytes, want %d", i, len(part), tt.chunks[i])
				}
				last := i == len(ms)-1
				var want string
				switch {
				case i == 0 && last:
					want = "a=t,f=100,t=d,i=16777218,s=3,v=4,q=2"
				case i == 0:
					want = "a=t,f=100,t=d,i=16777218,s=3,v=4,q=2,m=1"
				case last:
					want = "q=2,m=0"
				default:
					want = "q=2,m=1"
				}
				if keys != want {
					t.Errorf("chunk %d keys %q, want %q", i, keys, want)
				}
				payload.WriteString(part)
			}
			got, err := base64.StdEncoding.DecodeString(payload.String())
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("chunks decode to %d bytes (%v), want the %d sent", len(got), err, len(data))
			}
		})
	}
}

func TestTmuxWrapsEachChunk(t *testing.T) {
	seq := Transmit(NewID(1, 2), make([]byte, 3073), 1, 1)
	wrapped := Tmux(seq)
	if n := strings.Count(wrapped, "\x1bPtmux;"); n != 2 {
		t.Fatalf("%d wraps, want one for each of 2 chunks", n)
	}
	// tmux sends on what it wraps with each ESC halved again.
	var unwrapped strings.Builder
	for part := range strings.SplitSeq(strings.TrimSuffix(wrapped, "\x1b\\"), "\x1b\\\x1bPtmux;") {
		unwrapped.WriteString(strings.ReplaceAll(strings.TrimPrefix(part, "\x1bPtmux;"), "\x1b\x1b", "\x1b"))
	}
	if unwrapped.String() != seq {
		t.Errorf("unwrapped %q, want %q", unwrapped.String(), seq)
	}
	if Tmux("") != "" {
		t.Error("Tmux of nothing isn't empty")
	}
}

func flatten(ms [][]string) []string {
	s := make([]string, len(ms))
	for i, m := range ms {
		s[i] = m[0]
	}
	return s
}

func BenchmarkTransmit(b *testing.B) {
	data := make([]byte, 64<<10)
	b.ReportAllocs()
	for b.Loop() {
		Transmit(NewID(1, 2), data, 256, 256)
	}
}
