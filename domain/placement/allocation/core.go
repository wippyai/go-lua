package allocation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// operand is the owner-fenced allocation coordinate carried by one mounted
// allocation occurrence.  The Heap Key is the canonical identity; Placement
// deliberately does not retain a second occurrence or coordinate directory.
//
// The type stays private so a caller cannot manufacture a seed operand.  The
// only issuer is the hot rule's direct projection from Heap's canonical
// mount-scoped occurrence row.
type operand struct {
	key heapdomain.Key
	id  identity.ContentID
}

// allocationOperandForSchema authenticates one exact Heap allocation root
// against Placement's projected Heap authority.  In particular, a Boot root,
// a foreign Key, a missing KeyID, and a stale dense coordinate are all
// rejected before the rule can project a write.
func allocationOperandForSchema(schema placement.Schema, key heapdomain.Key) (operand, bool) {
	if !schema.Valid() {
		return operand{}, false
	}
	heap := schema.Heap()
	if !heap.Valid() || !heap.OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
		return operand{}, false
	}
	// Program allocation seeding is intentionally disjoint from the Link
	// fresh-root seed. Keep the source-family rejection explicit even if a
	// future Heap projection grows a broader allocation-origin view.
	if _, _, _, fresh := key.FreshResultID(); fresh {
		return operand{}, false
	}
	index, indexOK := heap.KeyIndex(key)
	canonical, canonicalOK := schema.KeyAt(index)
	id, idOK := key.ContentID()
	_, _, allocationID, kind, form, originOK := heap.AllocationOriginForKey(key)
	if !indexOK || index < 0 || !canonicalOK || canonical != key || !idOK || !id.Available() ||
		!originOK || !allocationID.Available() || !kind.Valid() || !form.Valid() {
		return operand{}, false
	}
	return operand{key: key, id: id}, true
}

// allocationOperandContentForSchema returns the canonical Heap Key identity
// used by engine derivation admission.  Requiring the candidate's key and
// digest to reproduce from the exact projected Heap schema prevents a stale
// or cross-binding operand from becoming Stack evidence.
func allocationOperandContentForSchema(schema placement.Schema, candidate operand) (operand, [32]byte, bool) {
	canonical, ok := allocationOperandForSchema(schema, candidate.key)
	if !ok || candidate.id != canonical.id {
		return operand{}, [32]byte{}, false
	}
	return canonical, [32]byte(canonical.id), true
}

// allocationCoordinateForSchema projects the already-issued Heap dense
// coordinate.  It never scans or creates an allocation index of its own.
func allocationCoordinateForSchema(schema placement.Schema, candidate operand) (uint64, bool) {
	canonical, _, ok := allocationOperandContentForSchema(schema, candidate)
	if !ok {
		return 0, false
	}
	index, indexOK := schema.Heap().KeyIndex(canonical.key)
	if !indexOK || index < 0 || uint64(index) >= uint64(schema.KeyCount()) {
		return 0, false
	}
	return uint64(index), true
}
