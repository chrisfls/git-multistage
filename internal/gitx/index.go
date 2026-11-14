package gitx

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
)

// IndexEntry is one entry of an index file, as `ls-files -s` prints it.
type IndexEntry struct {
	Mode  string // octal, six digits
	OID   string
	Stage int
	Path  string
}

// Index entry flags, as Git's read-cache.c stores them on disk.
const (
	indexNameMask    = 0x0fff
	indexExtended    = 0x4000
	indexStageShift  = 12
	indexExtFlags    = 0x6000 // intent-to-add and skip-worktree, the only extended flags
	indexModeTypeDir = 0o040000
	indexModeType    = 0o170000
	indexEntryFixed  = 40 // ctime, mtime, dev, ino, mode, uid, gid, and size
	maxVarintBytes   = 10
)

// ReadIndex reads the index file at path and returns the entries whose path
// match accepts, in index order. A missing file has no entries, as Git reads
// it. It reads versions 2, 3, and 4 of the format; ok is false, so that only
// Git reads the file, for a split index, a sparse index, any other required
// extension, another version, and a file that it cannot parse. It does not
// check the file's checksum, which Git does not check on a normal read either.
func (r *Repo) ReadIndex(path string, match func(name []byte) bool) (entries []IndexEntry, ok bool) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, true
	}
	hashLen := len(r.EmptyTree) / 2
	if err != nil || hashLen == 0 || len(data) < indexHeaderSize+hashLen || string(data[:4]) != "DIRC" {
		return nil, false
	}
	version := binary.BigEndian.Uint32(data[4:8])
	if version < 2 || version > 4 {
		return nil, false
	}
	p := indexParser{data: data[:len(data)-hashLen], pos: indexHeaderSize, hashLen: hashLen, version: version}
	for range binary.BigEndian.Uint32(data[8:12]) {
		if !p.next() {
			return nil, false
		}
		if match(p.name) {
			entries = append(entries, IndexEntry{
				Mode:  fmt.Sprintf("%06o", p.mode),
				OID:   hex.EncodeToString(p.oid),
				Stage: p.stage,
				Path:  string(p.name),
			})
		}
	}
	if !p.extensionsOptional() {
		return nil, false
	}
	return entries, true
}

// indexParser reads the entries of an index file without its checksum.
type indexParser struct {
	data    []byte
	pos     int
	hashLen int
	version uint32
	// The fields of the last entry. Version 4 builds each path on the
	// path before it.
	name  []byte
	mode  uint32
	oid   []byte
	stage int
}

// next reads the entry at pos into the fields of the last entry. It reports
// false for a malformed entry and a sparse directory entry.
func (p *indexParser) next() bool {
	start := p.pos
	nameAt := start + indexEntryFixed + p.hashLen + 2
	if nameAt > len(p.data) {
		return false
	}
	e := p.data[start:]
	p.mode = binary.BigEndian.Uint32(e[24:28])
	p.oid = e[indexEntryFixed : indexEntryFixed+p.hashLen]
	flags := binary.BigEndian.Uint16(e[indexEntryFixed+p.hashLen:])
	p.stage = int(flags>>indexStageShift) & 3
	if flags&indexExtended != 0 {
		if p.version < 3 || nameAt+2 > len(p.data) ||
			binary.BigEndian.Uint16(p.data[nameAt:])&^indexExtFlags != 0 {
			return false
		}
		nameAt += 2
	}
	return p.mode&indexModeType != indexModeTypeDir && p.readName(start, nameAt) &&
		int(flags&indexNameMask) == min(len(p.name), indexNameMask)
}

// readName reads the path of the entry at start, whose path field starts at
// nameAt, into name, and moves pos to the next entry.
func (p *indexParser) readName(start, nameAt int) bool {
	strip := 0
	if p.version == 4 {
		var n int
		if strip, n = varint(p.data[nameAt:]); n == 0 || strip > len(p.name) {
			return false
		}
		nameAt += n
	}
	nul := bytes.IndexByte(p.data[nameAt:], 0)
	if nul < 0 {
		return false
	}
	suffix := p.data[nameAt : nameAt+nul]
	if p.version == 4 {
		p.name = append(p.name[:len(p.name)-strip], suffix...)
		p.pos = nameAt + nul + 1
		return true
	}
	p.name = suffix
	// Versions 2 and 3 pad each entry with 1 to 8 NULs to a multiple of 8
	// bytes.
	p.pos = start + (nameAt-start+nul+8)&^7
	return p.pos <= len(p.data)
}

// extensionsOptional reads the extensions after the entries: each is a
// 4-byte signature, a 4-byte size, and its data. It reports whether every
// extension is optional, which Git marks with an upper-case first letter; a
// split index (`link`) and a sparse index (`sdir`) have required ones.
func (p *indexParser) extensionsOptional() bool {
	for p.pos < len(p.data) {
		if p.pos+8 > len(p.data) {
			return false
		}
		if sig := p.data[p.pos]; sig < 'A' || sig > 'Z' {
			return false
		}
		size := int64(binary.BigEndian.Uint32(p.data[p.pos+4:]))
		if int64(p.pos)+8+size > int64(len(p.data)) {
			return false
		}
		p.pos += 8 + int(size)
	}
	return true
}

// varint decodes the offset varint of Git's varint.c, which version 4 of the
// index uses for the bytes to remove from the previous path. n is 0 for a
// malformed number and for one that does not fit in an int.
func varint(b []byte) (v, n int) {
	for n < len(b) && n < maxVarintBytes {
		c := b[n]
		n++
		v += int(c & 127)
		if c&128 == 0 {
			return v, n
		}
		if v >= math.MaxInt>>7 {
			return 0, 0
		}
		v = (v + 1) << 7
	}
	return 0, 0
}
