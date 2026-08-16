package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// ContentID is a full SHA-256 semantic identity. Its zero value is
// unavailable. ContentID is shared identity vocabulary; the owner that
// derives one decides the domain and preimage, while consumers only carry and
// compare the issued value.
type ContentID [sha256.Size]byte

// Available reports whether id was successfully derived.
func (id ContentID) Available() bool {
	// binary.Uint64's byte-wise implementation is portable and unaligned-safe;
	// fixed offsets let supported backends optimize these decodes without
	// changing the zero-value semantics.
	return binary.LittleEndian.Uint64(id[0:8])|
		binary.LittleEndian.Uint64(id[8:16])|
		binary.LittleEndian.Uint64(id[16:24])|
		binary.LittleEndian.Uint64(id[24:32]) != 0
}

// String returns the lower-case hexadecimal form of id.
func (id ContentID) String() string { return hex.EncodeToString(id[:]) }
