package identity

import (
	"crypto/sha256"
	"encoding/binary"
)

// deriveFrameHeader is the fixed length prefix every framed field carries.
const deriveFrameHeader = 8

// deriveInlineFrame bounds the preimage assembled without touching the heap.
// It covers a domain tag plus the identity vector of every ordinary
// derivation; a wider preimage is streamed instead, so no derivation allocates
// a buffer proportional to its payload.
const deriveInlineFrame = 512

// DeriveContentID derives one content identity from a versioned domain tag
// and its ordered payload. The caller owns the meaning of the tag and parts;
// identity owns the single framed digest construction shared by those owners.
//
// Assembling the framed preimage in one bounded buffer is the same content as
// streaming each frame into a running digest, and it is what keeps the hottest
// identity operation in an analysis off the heap: a digest object, a tag copy,
// and a frame vector are otherwise paid once per derived identity.
func DeriveContentID(tag string, parts ...[]byte) (ContentID, bool) {
	if tag == "" {
		return ContentID{}, false
	}
	width := deriveFrameHeader + len(tag)
	for _, part := range parts {
		width += deriveFrameHeader + len(part)
	}
	if width > deriveInlineFrame {
		return streamContentID(tag, parts)
	}
	var inline [deriveInlineFrame]byte
	preimage := appendFrame(appendFrameString(inline[:0], tag), parts...)
	id := ContentID(sha256.Sum256(preimage))
	return id, id.Available()
}

// streamContentID commits the same framing one field at a time. It is the
// bound on the construction: a derivation over a whole plane vector costs one
// running digest rather than a preimage the width of the plane.
func streamContentID(tag string, parts [][]byte) (ContentID, bool) {
	hash := sha256.New()
	var header [deriveFrameHeader]byte
	binary.BigEndian.PutUint64(header[:], uint64(len(tag)))
	hash.Write(header[:])
	hash.Write([]byte(tag))
	for _, part := range parts {
		binary.BigEndian.PutUint64(header[:], uint64(len(part)))
		hash.Write(header[:])
		hash.Write(part)
	}
	var id ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

// appendFrame commits one canonical field per part: its big-endian length
// followed by its bytes.
func appendFrame(preimage []byte, parts ...[]byte) []byte {
	var header [deriveFrameHeader]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(header[:], uint64(len(part)))
		preimage = append(append(preimage, header[:]...), part...)
	}
	return preimage
}

// appendFrameString commits one canonical text field under the same framing.
// It is separate only so a domain tag never needs a byte-slice copy.
func appendFrameString(preimage []byte, value string) []byte {
	var header [deriveFrameHeader]byte
	binary.BigEndian.PutUint64(header[:], uint64(len(value)))
	return append(append(preimage, header[:]...), value...)
}
