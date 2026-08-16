package identity

import (
	"crypto/sha256"
	"encoding/binary"
)

// DeriveContentID derives one content identity from a versioned domain tag
// and its ordered payload. The caller owns the meaning of the tag and parts;
// identity owns the single framed digest construction shared by those owners.
func DeriveContentID(tag string, parts ...[]byte) (ContentID, bool) {
	if tag == "" {
		return ContentID{}, false
	}
	hash := sha256.New()
	if !writeFrame(hash, []byte(tag)) {
		return ContentID{}, false
	}
	for _, part := range parts {
		if !writeFrame(hash, part) {
			return ContentID{}, false
		}
	}
	var id ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func writeFrame(hash interface{ Write([]byte) (int, error) }, value []byte) bool {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	first, firstErr := hash.Write(size[:])
	second, secondErr := hash.Write(value)
	return firstErr == nil && secondErr == nil && first == len(size) && second == len(value)
}
