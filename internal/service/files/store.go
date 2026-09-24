package files

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"path"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Store keeps objects across sessions, such as the disk layer of the
// cache. Keys are lowercase hex. Get reports false for an object it can't
// read, and Put may fail; either way the service reads from GitHub instead.
type Store interface {
	Get(kind, key string) ([]byte, bool)
	Put(kind, key string, data []byte) error
	Delete(kind, key string)
}

// noStore keeps nothing.
type noStore struct{}

func (noStore) Get(string, string) ([]byte, bool) { return nil, false }
func (noStore) Put(string, string, []byte) error  { return nil }
func (noStore) Delete(string, string)             {}

// The kinds of objects the service stores. Blobs, trees and listings are
// named by the SHA they were read by, which fixes their content, so they
// never need revalidating. A ref names where a branch, tag or HEAD pointed
// when it was last read, with the validators to ask GitHub whether it moved.
const (
	kindBlob    = "blob"
	kindTree    = "tree"
	kindListing = "listing"
	kindRef     = "ref"
)

// codecVersion starts every tree and ref the service stores. Change it when
// their encoding changes, and older objects read as misses.
const codecVersion = 1

var errCodec = errors.New("unreadable object")

// encodeTree encodes t compactly: a listing of a large repository has tens
// of thousands of entries. The Name of an entry is the last element of its
// Path, so only the Path is kept.
func encodeTree(t core.Tree) []byte {
	n := 2 + 2*binary.MaxVarintLen64 + len(t.SHA)
	for _, e := range t.Entries {
		// A varint below 2 MiB takes three bytes at most. A larger size
		// only makes append grow b.
		n += len(e.Path) + len(e.Mode) + len(e.Type) + len(e.SHA) + 5*3
	}
	b := make([]byte, 0, n)
	b = append(b, codecVersion)
	var flags byte
	if t.Truncated {
		flags |= 1
	}
	b = append(b, flags)
	b = appendString(b, t.SHA)
	b = binary.AppendUvarint(b, uint64(len(t.Entries)))
	for _, e := range t.Entries {
		b = appendString(b, e.Path)
		b = appendString(b, e.Mode)
		b = appendString(b, string(e.Type))
		b = appendString(b, e.SHA)
		b = binary.AppendUvarint(b, uint64(max(e.Size, 0)))
	}
	return b
}

// decodeTree decodes what encodeTree encoded. Every string of the tree
// shares one copy of data, so decoding allocates little however large the
// listing is.
func decodeTree(data []byte) (core.Tree, error) {
	d := decoder{s: string(data)}
	if d.byte() != codecVersion {
		return core.Tree{}, errCodec
	}
	flags := d.byte()
	t := core.Tree{Truncated: flags&1 != 0, SHA: d.string()}
	n := d.uvarint()
	// Each entry takes at least five bytes, which bounds a corrupt count.
	if d.err != nil || n > uint64(len(d.s))/5 {
		return core.Tree{}, errCodec
	}
	t.Entries = make([]core.TreeEntry, n)
	for i := range t.Entries {
		e := &t.Entries[i]
		e.Path = d.string()
		e.Name = path.Base(e.Path)
		e.Mode = d.string()
		e.Type = core.EntryType(d.string())
		e.SHA = d.string()
		e.Size = int64(d.uvarint())
	}
	if d.err != nil || d.s != "" {
		return core.Tree{}, errCodec
	}
	return t, nil
}

// refRecord is where a ref pointed when it was last read, and the
// validators of that response.
type refRecord struct {
	SHA          string
	ETag         string
	LastModified string
}

func encodeRef(r refRecord) []byte {
	b := []byte{codecVersion}
	b = appendString(b, r.SHA)
	b = appendString(b, r.ETag)
	return appendString(b, r.LastModified)
}

func decodeRef(data []byte) (refRecord, error) {
	d := decoder{s: string(data)}
	if d.byte() != codecVersion {
		return refRecord{}, errCodec
	}
	r := refRecord{SHA: d.string(), ETag: d.string(), LastModified: d.string()}
	if d.err != nil || d.s != "" || !isSHA(r.SHA) {
		return refRecord{}, errCodec
	}
	return r, nil
}

func appendString(b []byte, s string) []byte {
	b = binary.AppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

// decoder reads what the append functions wrote from s. After an error,
// every read returns the zero value.
type decoder struct {
	s   string
	err error
}

func (d *decoder) byte() byte {
	if d.err != nil || d.s == "" {
		d.err = errCodec
		return 0
	}
	c := d.s[0]
	d.s = d.s[1:]
	return c
}

func (d *decoder) uvarint() uint64 {
	if d.err != nil {
		return 0
	}
	// Uvarint takes a []byte; ten bytes is the longest varint.
	var buf [binary.MaxVarintLen64]byte
	n := copy(buf[:], d.s)
	v, k := binary.Uvarint(buf[:n])
	if k <= 0 {
		d.err = errCodec
		return 0
	}
	d.s = d.s[k:]
	return v
}

func (d *decoder) string() string {
	n := d.uvarint()
	if d.err != nil || n > uint64(len(d.s)) {
		d.err = errCodec
		return ""
	}
	s := d.s[:n]
	d.s = d.s[n:]
	return s
}

// refKey names the record of ref in repo, read as kind. Repository names
// ignore case, refs don't. Hashing makes any ref a valid key.
func refKey(kind string, repo core.RepoRef, ref string) string {
	h := sha256.Sum256([]byte(kind + "\x00" + strings.ToLower(repo.String()) + "\x00" + ref))
	return hex.EncodeToString(h[:])
}

// isBlobID reports whether sha is the git object name of a blob with
// content: the SHA-1, or in a SHA-256 repository the SHA-256, of a header
// and the content.
func isBlobID(sha string, content []byte) bool {
	var h hash.Hash
	switch len(sha) {
	case sha1.Size * 2:
		// Git names objects by SHA-1. This checks a name against its
		// content; it doesn't secure anything.
		h = sha1.New()
	case sha256.Size * 2:
		h = sha256.New()
	default:
		return false
	}
	h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00"))
	h.Write(content)
	want, err := hex.DecodeString(sha)
	return err == nil && bytes.Equal(h.Sum(nil), want)
}
