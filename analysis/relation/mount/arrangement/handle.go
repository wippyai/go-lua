package arrangement

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
)

// Handle is an opaque physical arrangement coordinate authenticated by one
// mounted fence.  The local coordinate is intentionally private: only the
// arrangement/runtime owner can carry a Handle, while callers can validate
// its fence or use it as a comparable registry key.
type Handle struct {
	fence address.Fence
	slot  uint64
}

// NewHandle adopts one non-zero private inventory coordinate for fence.
// Zero coordinates and incomplete fences are unavailable.
func NewHandle(fence address.Fence, slot uint64) (Handle, bool) {
	if !fence.Available() || slot == 0 {
		return Handle{}, false
	}
	return Handle{fence: fence, slot: slot}, true
}

// Available reports whether the handle carries a complete fence and private
// coordinate.
func (handle Handle) Available() bool { return handle.fence.Available() && handle.slot != 0 }

// ValidFor reports whether the coordinate belongs to exactly fence.
func (handle Handle) ValidFor(fence address.Fence) bool {
	return handle.Available() && handle.fence.Same(fence)
}

// Fence returns the immutable authentication fence without revealing the
// local arrangement coordinate.
func (handle Handle) Fence() address.Fence { return handle.fence }

func handleDigest(handle Handle) []byte {
	var store [8]byte
	var generation [8]byte
	var slot [8]byte
	binary.BigEndian.PutUint64(store[:], uint64(handle.fence.StoreID()))
	binary.BigEndian.PutUint64(generation[:], uint64(handle.fence.Generation()))
	binary.BigEndian.PutUint64(slot[:], handle.slot)
	mount := handle.fence.MountID()
	value, _ := identity.DeriveContentID(
		"analysis/relation/mount/arrangement/handle/v1",
		identityBytes(handle.fence.SchemaID().Owner().Content()),
		identityBytes(handle.fence.SchemaID().Content()),
		identityBytes(handle.fence.CertificateDigest()),
		store[:], mount[:], generation[:], slot[:],
	)
	return identityBytes(value)
}

func identityBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}
