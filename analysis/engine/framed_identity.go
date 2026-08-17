package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// Engine identities are minted from canonical framed preimages. The domain and
// version open the preimage and every part carries its own length, so no
// concatenation of parts can be reparsed as a different part vector and no two
// domains can mint the same identity from the same content.

func framedDigest(domain string, version uint64, encode func(*canonical.DigestWriter) bool) ([32]byte, bool) {
	var writer canonical.DigestWriter
	if writer.Reset(domain, version) != nil || !encode(&writer) || writer.Finish() != nil {
		return [32]byte{}, false
	}
	digest := writer.Sum()
	return digest, digest != [32]byte{}
}

// framedContentID mints one engine content identity. An unencodable preimage
// yields the unavailable identity, which every admission fence refuses.
func framedContentID(domain string, version uint64, encode func(*canonical.DigestWriter) bool) identity.ContentID {
	digest, ok := framedDigest(domain, version, encode)
	if !ok {
		return identity.ContentID{}
	}
	return identity.ContentID(digest)
}

// framedCompositionKey mints one cold composition key under the same framing.
func framedCompositionKey(domain string, version uint64, encode func(*canonical.DigestWriter) bool) (composition.Key, bool) {
	digest, ok := framedDigest(domain, version, encode)
	if !ok {
		return composition.Key{}, false
	}
	key := composition.Key{ID: composition.ID(digest), Version: version}
	return key, key.Available()
}

func writeContentIDs(writer *canonical.DigestWriter, values ...identity.ContentID) bool {
	for _, value := range values {
		if writer.Bytes(value[:]) != nil {
			return false
		}
	}
	return true
}

// writeRuleSlotCapability frames one slot capability as three scalar parts.
// Framing each field separately keeps two capabilities that differ only in
// where one field ends from sharing a preimage.
func writeRuleSlotCapability(writer *canonical.DigestWriter, capability RuleSlotCapability) bool {
	activation := uint64(0)
	if capability.activation {
		activation = 1
	}
	return writer.Uint(uint64(capability.kind)) == nil && writer.Uint(activation) == nil && writer.Uint(capability.ordinal) == nil
}
