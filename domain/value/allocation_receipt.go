package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/heap"
)

// AllocationResult is the sealed Value-owned result of one Heap allocation
// reference. It keeps the exact Heap key identity, its stable key ID, the
// Value coordinate, and the canonical Recent fact in the same reference row.
// Hot allocation rules consume this receipt without re-reading Link, Heap,
// Boundary, or coordinate maps.
type AllocationResult struct {
	schema     *Schema
	key        heap.Key
	keyID      identity.ContentID
	coordinate Coordinate
	routes     []Coordinate
	fresh      Value
	recent     uint32
	summary    uint32
}

func (result *AllocationResult) validFor(schema *Schema) bool {
	return result != nil && schema != nil && result.schema == schema &&
		result.keyID.Available() && result.coordinate.schema == schema && result.coordinate.Valid() && len(result.routes) != 0 && result.routes[0] == result.coordinate &&
		result.fresh.schema == schema && result.fresh.valid() && result.recent != 0 && result.summary == result.recent+1
}

// Owns reports whether this sealed receipt belongs to the exact Value schema.
func (result *AllocationResult) Owns(schema *Schema) bool { return result.validFor(schema) }

// Key returns the exact Heap key paired with this receipt.
func (result *AllocationResult) Key() (heap.Key, bool) {
	if result == nil || !result.keyID.Available() {
		return heap.Key{}, false
	}
	return result.key, true
}

// KeyID returns the stable identity issued with the exact Heap key.
func (result *AllocationResult) KeyID() (identity.ContentID, bool) {
	if result == nil || !result.keyID.Available() {
		return identity.ContentID{}, false
	}
	return result.keyID, true
}

// Coordinate returns the exact Value target paired with this allocation.
func (result *AllocationResult) Coordinate() (Coordinate, bool) {
	if result == nil || !result.coordinate.Valid() {
		return Coordinate{}, false
	}
	return result.coordinate, true
}

// RouteCount reports the complete canonical Value-coordinate image written
// atomically by this allocation. Route zero is always the allocation root;
// callable allocations additionally carry their exact Function value.
func (result *AllocationResult) RouteCount() int {
	if result == nil || !result.validFor(result.schema) {
		return 0
	}
	return len(result.routes)
}

// RouteAt returns one owner-issued allocation result route.
func (result *AllocationResult) RouteAt(index int) (Coordinate, bool) {
	if result == nil || !result.validFor(result.schema) || index < 0 || index >= len(result.routes) {
		return Coordinate{}, false
	}
	return result.routes[index], true
}

// Fresh returns the canonical Recent fact issued for this allocation.
func (result *AllocationResult) Fresh() (Value, bool) {
	if result == nil || !result.fresh.valid() {
		return Value{}, false
	}
	return result.fresh, true
}

// Age applies the receipt's presealed recency transition without reopening
// the Heap or Link topology.
func (result *AllocationResult) Age(state Value) (Value, bool) {
	if result == nil {
		return Value{}, false
	}
	return result.schema.AgeWithAllocation(state, result)
}

// AllocationResultFor returns the already-issued allocation receipt. It is an
// issuance/instance lookup; hot callbacks retain the receipt in their typed
// operand and do not call this method.
func (schema *Schema) AllocationResultFor(key heap.Key) (*AllocationResult, bool) {
	if schema == nil || !schema.heap.OwnsKey(key) || key.Kind() != heap.RootAllocation {
		return nil, false
	}
	reference := schema.allocRefs[key]
	if reference == 0 || int(reference) > len(schema.references) {
		return nil, false
	}
	result := schema.references[reference-1].allocationResult
	return result, result.validFor(schema)
}

// AgeWithAllocation applies the presealed Recent-to-Summary transition using
// only the exact allocation receipt. The ordinary Age API remains available
// for cold callers and resolves its receipt once before entering this path.
func (schema *Schema) AgeWithAllocation(state Value, result *AllocationResult) (Value, bool) {
	if schema == nil || !schema.owns(state) || !result.validFor(schema) {
		return Value{}, false
	}
	recent, summary := result.recent, result.summary
	if state.top || len(state.image) == 0 {
		return state, true
	}

	stride := schema.stride()
	var changed bool
	for offset := 0; offset < len(state.image); offset += stride {
		if state.image[offset] == uint64(recent) {
			changed = true
			break
		}
	}
	if !changed {
		return state, true
	}

	image := make([]uint64, 0, len(state.image))
	for offset := 0; offset < len(state.image); offset += stride {
		atom := uint32(state.image[offset])
		if atom == recent {
			atom = summary
		}
		if len(image) == 0 || image[len(image)-stride] != uint64(atom) {
			row := append([]uint64(nil), state.image[offset:offset+stride]...)
			row[0] = uint64(atom)
			image = append(image, row...)
			continue
		}
		for word := 1; word < stride; word++ {
			image[len(image)-stride+word] |= state.image[offset+word]
		}
	}
	return schema.canonical(image), true
}
