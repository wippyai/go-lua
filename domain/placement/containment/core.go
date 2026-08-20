package containment

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// operand is the one canonical parent allocation-root proof consumed by the
// Link-lane rule. Placement never mints a second root identity: key and id are
// both projected from Heap's sealed root row and are retained only together so
// the hot rule can authenticate the pair.
type operand struct {
	key heapdomain.Key
	id  identity.ContentID
}

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
	id, idOK := key.ContentID()
	if !indexOK || index < 0 || !canonicalOK || canonical != key || !idOK || !id.Available() {
		return operand{}, false
	}
	// Heap shares RootAllocation's carrier between mounted Program roots and
	// Target fresh roots. Fresh roots have no constructor fields and therefore
	// are intentionally omitted from containment's parent denominator; the
	// separate Placement/fresh seed owns their unconditional Stack write.
	// Program roots retain the existing origin validation below.
	_, _, _, freshOK := key.FreshResultID()
	if freshOK {
		return operand{}, false
	}
	_, _, allocationID, kind, form, originOK := heap.AllocationOriginForKey(key)
	if !originOK || !allocationID.Available() || !kind.Valid() || !form.Valid() {
		return operand{}, false
	}
	return operand{key: key, id: id}, true
}

func operandContentForSchema(schema placement.Schema, candidate operand) (operand, [32]byte, bool) {
	canonical, ok := operandForSchema(schema, candidate.key)
	if !ok || candidate.id != canonical.id {
		return operand{}, [32]byte{}, false
	}
	return canonical, [32]byte(canonical.id), true
}

func operandCoordinateForSchema(schema placement.Schema, candidate operand) (uint64, bool) {
	canonical, _, ok := operandContentForSchema(schema, candidate)
	if !ok {
		return 0, false
	}
	index, indexOK := schema.Heap().KeyIndex(canonical.key)
	return uint64(index), indexOK && index >= 0 && index < schema.KeyCount()
}
