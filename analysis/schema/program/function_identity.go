package programschema

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

const (
	callFormalIdentityVersion uint64 = 1
	callFormalIdentityTag            = "pcallbod"
)

// CallFormalIdentity returns the canonical call-interface identity of one
// sealed Body context. The caller owns the Body relation and proves that the
// context belongs to it; schema owns only this stable identity equation.
func CallFormalIdentity(context identity.ContentID) (identity.ContentID, bool) {
	if !context.Available() {
		return identity.ContentID{}, false
	}
	var payload [8 + 8 + sha256.Size]byte
	copy(payload[:8], callFormalIdentityTag)
	binary.BigEndian.PutUint64(payload[8:16], callFormalIdentityVersion)
	copy(payload[16:], context[:])
	id := identity.ContentID(sha256.Sum256(payload[:]))
	return id, id.Available()
}
