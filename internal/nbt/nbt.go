// Package nbt implements a minimal, dependency-free reader for the NBT
// (Named Binary Tag) format used by Minecraft save files. It does not
// attempt to be a general-purpose NBT library — it only supports walking
// a compound tag tree and extracting specific named fields, which is all
// callers (e.g. package worlds) currently need.
package nbt

import (
	"bufio"
	"encoding/binary"
	"io"
)

// Tag type constants, as defined by the NBT specification.
const (
	TagEnd       = 0
	TagByte      = 1
	TagShort     = 2
	TagInt       = 3
	TagLong      = 4
	TagFloat     = 5
	TagDouble    = 6
	TagByteArray = 7
	TagString    = 8
	TagList      = 9
	TagCompound  = 10
	TagIntArray  = 11
	TagLongArray = 12
)

// ReadRootCompound reads the type byte and name of the root tag of an NBT
// stream (uncompressed — callers are responsible for gzip decompression).
// Returns ok=false if the root tag is not a compound, which indicates the
// file isn't valid NBT.
func ReadRootCompound(r *bufio.Reader) bool {
	rootType, err := r.ReadByte()
	if err != nil || rootType != TagCompound {
		return false
	}
	if _, ok := ReadString(r); !ok {
		return false
	}
	return true
}

// Visitor is called for each scalar/string tag encountered while walking a
// compound, identified by its path (sequence of enclosing compound names)
// and its own name. Implementations should be cheap and side-effect based
// (e.g. populate a struct) — returning early is not supported; Walk always
// visits every tag in the tree it descends into.
//
// path does not include the current tag's own name.
type Visitor func(path []string, name string, tagType byte, r *bufio.Reader) bool

// Walk reads tags inside a compound until TagEnd, calling visit for each
// one. visit is responsible for reading (or skipping, via Skip) the
// payload of every tag type it doesn't care about — Walk itself only
// handles recursing into nested compounds when visit returns
// shouldDescend=true is not how this works; instead, Walk recurses into
// TagCompound automatically using descendPath to decide the child path,
// and visit is called for the compound tag itself first so it can record
// that it's "entering" a named section if needed via path tracking by the
// caller.
//
// To keep this generic, Walk delegates ALL non-compound payload handling
// to visit (which must call Skip if it doesn't consume the tag itself),
// and handles TagCompound by calling descendPath(path, name) to compute
// the child path, then recursing.
func Walk(r *bufio.Reader, path []string, descendPath func(path []string, name string) []string, visit Visitor) {
	for {
		tagType, err := r.ReadByte()
		if err != nil || tagType == TagEnd {
			return
		}
		name, ok := ReadString(r)
		if !ok {
			return
		}

		if tagType == TagCompound {
			childPath := descendPath(path, name)
			Walk(r, childPath, descendPath, visit)
			continue
		}

		if !visit(path, name, tagType, r) {
			return
		}
	}
}

// ReadString reads a length-prefixed (2-byte big-endian) UTF-8 string, as
// used for NBT tag names and TagString payloads.
func ReadString(r *bufio.Reader) (string, bool) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return "", false
	}
	n := int(lenBuf[0])<<8 | int(lenBuf[1])
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", false
	}
	return string(buf), true
}

// ReadLong reads a TagLong payload (8-byte big-endian signed integer).
func ReadLong(r *bufio.Reader) (int64, bool) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(buf[:])), true
}

// ReadInt reads a TagInt payload (4-byte big-endian signed integer). Also
// used as the length prefix for array and list tags.
func ReadInt(r *bufio.Reader) (int32, bool) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, false
	}
	return int32(binary.BigEndian.Uint32(buf[:])), true
}

// Skip reads and discards the payload for tagType. Callers (Visitor
// implementations) use this for tag types they don't care about.
func Skip(r *bufio.Reader, tagType byte) bool {
	switch tagType {
	case TagByte:
		_, err := r.ReadByte()
		return err == nil
	case TagShort:
		_, err := r.Discard(2)
		return err == nil
	case TagInt, TagFloat:
		_, err := r.Discard(4)
		return err == nil
	case TagLong, TagDouble:
		_, err := r.Discard(8)
		return err == nil
	case TagByteArray:
		n, ok := ReadInt(r)
		if !ok {
			return false
		}
		_, err := r.Discard(int(n))
		return err == nil
	case TagString:
		_, ok := ReadString(r)
		return ok
	case TagIntArray:
		n, ok := ReadInt(r)
		if !ok {
			return false
		}
		_, err := r.Discard(int(n) * 4)
		return err == nil
	case TagLongArray:
		n, ok := ReadInt(r)
		if !ok {
			return false
		}
		_, err := r.Discard(int(n) * 8)
		return err == nil
	case TagList:
		elemType, err := r.ReadByte()
		if err != nil {
			return false
		}
		n, ok := ReadInt(r)
		if !ok {
			return false
		}
		for i := int32(0); i < n; i++ {
			if elemType == TagCompound {
				for {
					t, err := r.ReadByte()
					if err != nil {
						return false
					}
					if t == TagEnd {
						break
					}
					if _, ok := ReadString(r); !ok {
						return false
					}
					if !Skip(r, t) {
						return false
					}
				}
			} else if !Skip(r, elemType) {
				return false
			}
		}
		return true
	}
	return false
}
