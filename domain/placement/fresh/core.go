package fresh

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// operand is the owner-fenced coordinate carried by one canonical Target
// fresh root.  Heap's Key is the coordinate authority; Placement keeps the
// corresponding ContentID only as the Link occurrence identity.
//
// The type is intentionally private. A caller can receive an operand only
// after the engine resolves an occurrence through the exact Placement/Heap
// authority.
type operand struct {
	key heapdomain.Key
	id  identity.ContentID
}

// operandForSchema authenticates one canonical fresh root against Placement's
// projected Heap authority. It deliberately does not consult Heap's
// Program-allocation provenance projection, which would reject Target roots.
// The exact fresh identity is Heap's sealed
// (ApplicationID, OutcomeResultID, ordinal) triple.
func operandForSchema(schema placement.Schema, key heapdomain.Key) (operand, bool) {
	if !schema.Valid() {
		return operand{}, false
	}
	heap := schema.Heap()
	if !heap.Valid() || !heap.OwnsKey(key) || key.Kind() != heapdomain.RootAllocation {
		return operand{}, false
	}
	index, indexOK := heap.KeyIndex(key)
	canonical, canonicalOK := schema.KeyAt(index)
	if !indexOK || index < 0 || !canonicalOK || canonical != key {
		return operand{}, false
	}
	id, idOK := key.ContentID()
	applicationID, outcomeResultID, _, freshOK := key.FreshResultID()
	if !idOK || !id.Available() ||
		!freshOK || !applicationID.Available() || !outcomeResultID.Available() {
		return operand{}, false
	}
	return operand{key: key, id: id}, true
}

// operandContentForSchema reauthenticates both halves of a received operand.
// The digest is the exact Heap ContentID and is therefore also the engine's
// owner-fenced operand identity.
func operandContentForSchema(schema placement.Schema, candidate operand) (operand, [32]byte, bool) {
	canonical, ok := operandForSchema(schema, candidate.key)
	if !ok || candidate.id != canonical.id {
		return operand{}, [32]byte{}, false
	}
	return canonical, [32]byte(canonical.id), true
}

// operandCoordinateForSchema projects the Heap-issued dense coordinate. No
// fresh-root index is created by Placement.
func operandCoordinateForSchema(schema placement.Schema, candidate operand) (uint64, bool) {
	canonical, _, ok := operandContentForSchema(schema, candidate)
	if !ok {
		return 0, false
	}
	index, indexOK := schema.Heap().KeyIndex(canonical.key)
	return uint64(index), indexOK && index >= 0 && index < schema.KeyCount()
}
