package address

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Fence identifies exactly one mounted revision of one checked certificate.
//
// A fence is deliberately a value object.  SchemaID and CertificateDigest
// identify the logical artifact, while StoreID, MountID, and Generation make
// the runtime address space explicit.  The fields remain private so callers
// cannot manufacture a partially available fence or replace one coordinate
// without going through the constructor.
type Fence struct {
	schemaID          model.SchemaID
	certificateDigest identity.ContentID
	storeID           identity.StoreID
	mountID           identity.MountID
	generation        identity.Generation
}

// NewFence adopts one exact logical certificate and one mounted store
// revision.  Every coordinate is required; the zero Fence is never valid.
func NewFence(schemaID model.SchemaID, certificateDigest identity.ContentID, storeID identity.StoreID, mountID identity.MountID, generation identity.Generation) (Fence, bool) {
	fence := Fence{
		schemaID:          schemaID,
		certificateDigest: certificateDigest,
		storeID:           storeID,
		mountID:           mountID,
		generation:        generation,
	}
	if !fence.Available() {
		return Fence{}, false
	}
	return fence, true
}

// Available reports whether every logical and runtime coordinate is present.
func (fence Fence) Available() bool {
	return fence.schemaID.Available() &&
		fence.certificateDigest.Available() &&
		fence.storeID.Available() &&
		fence.mountID.Available() &&
		fence.generation.Available()
}

// SchemaID returns the owner-issued schema identity carried by the fence.
func (fence Fence) SchemaID() model.SchemaID { return fence.schemaID }

// CertificateDigest returns the checked certificate identity carried by the
// fence.  It is not a physical-book digest.
func (fence Fence) CertificateDigest() identity.ContentID { return fence.certificateDigest }

// StoreID returns the process-local store identity carried by the fence.
func (fence Fence) StoreID() identity.StoreID { return fence.storeID }

// MountID returns the mounted-instance identity carried by the fence.
func (fence Fence) MountID() identity.MountID { return fence.mountID }

// Generation returns the exact store revision carried by the fence.
func (fence Fence) Generation() identity.Generation { return fence.generation }

// Same reports whether both fences are complete and identify the same mounted
// certificate revision.
func (fence Fence) Same(other Fence) bool {
	return fence.Available() && other.Available() && fence == other
}

// ValidFor is the exact-fence relation used by addresses and books.
func (fence Fence) ValidFor(other Fence) bool { return fence.Same(other) }
