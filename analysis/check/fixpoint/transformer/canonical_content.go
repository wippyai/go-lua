package transformer

import (
	"crypto/sha256"
	"encoding/hex"
)

// ContentID is an immutable identity derived solely from canonical semantic
// content. It deliberately has no generation, version counter, address, or
// process-local component. Hashes are indexes; callers that retain a cached
// artifact must also retain its canonical bytes for collision confirmation.
type ContentID [sha256.Size]byte

func contentID(encoded []byte) ContentID { return sha256.Sum256(encoded) }

func (id ContentID) Valid() bool {
	var zero ContentID
	return id != zero
}

func (id ContentID) String() string { return hex.EncodeToString(id[:]) }

// CanonicalContent is the representation-independent identity boundary used
// by the stage-1 registry, contracts, and dependency records.
type CanonicalContent interface {
	CanonicalBytes() []byte
	ContentID() ContentID
}
