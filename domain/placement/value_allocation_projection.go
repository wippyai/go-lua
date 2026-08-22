package placement

import (
	"github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// ValueAllocationProjection is Placement's owner-fenced projection of one
// complete Value relation onto Heap allocation roots.  Value remains the
// authority for atom identity; Placement only retains the exact allocation
// keys needed by its own dense factor.
//
// Exact roots are kept in Placement/Heap dense order and aliases are
// deduplicated.  HasOpaque records an authenticated opaque reference
// alternative; IsTop records an authenticated Value Top.  Either condition
// asks a caller to widen its own route or demand policy to the complete
// allocation denominator.  The projection deliberately does not choose that
// policy for the caller.
type ValueAllocationProjection struct {
	schema Schema

	// valid distinguishes a refused projection from a valid scalar relation.
	valid  bool
	bottom bool
	top    bool
	opaque bool

	inline [valueAllocationInlineWidth]heap.Key
	extra  []heap.Key
	count  int
}

const valueAllocationInlineWidth = 8

// ProjectValueAllocations authenticates and projects one complete Value
// relation against the exact Placement/Heap owner.  Bottom, scalar-only, and
// exact rooted relations are valid projections.  Top and opaque reference
// alternatives remain explicit widening facts.  A foreign, malformed, or
// otherwise unavailable Value relation is refused; it is never converted to
// an opaque or Unknown result.
func ProjectValueAllocations(schema Schema, values *valuedomain.Schema, fact valuedomain.Value) (ValueAllocationProjection, bool) {
	projection := ValueAllocationProjection{schema: schema}
	if !schema.Valid() || values == nil || !values.Valid() || !values.OwnsHeapSchema(schema.Heap()) {
		return ValueAllocationProjection{}, false
	}
	// Authenticate the relation before inspecting Bottom or Top.  Those
	// extrema are owner-local and a foreign Value must not widen this Heap.
	if !values.Equal(fact, fact) {
		return ValueAllocationProjection{}, false
	}
	projection.valid = true
	if fact.IsBottom() {
		projection.bottom = true
		return projection, true
	}
	if fact.IsTop() {
		projection.top = true
		return projection, true
	}

	heapSchema := schema.Heap()
	for atomIndex, atomCount := 0, values.ValueAtomCount(fact); atomIndex < atomCount; atomIndex++ {
		atom, atomOK := values.ValueAtomAt(fact, atomIndex)
		if !atomOK {
			return ValueAllocationProjection{}, false
		}
		classification, classificationOK := ClassifyAtom(values, atom)
		if !classificationOK || !classification.Valid() {
			return ValueAllocationProjection{}, false
		}
		switch classification.Class {
		case AtomClassAllocation:
			key := classification.Key
			if !heapSchema.OwnsKey(key) || key.Kind() != heap.RootAllocation {
				return ValueAllocationProjection{}, false
			}
			dense, denseOK := heapSchema.AllocationKeyIndex(key)
			canonical, canonicalOK := schema.KeyAt(dense)
			if !denseOK || dense < 0 || !canonicalOK || canonical != key {
				return ValueAllocationProjection{}, false
			}
			if !projection.addExact(dense, key) {
				return ValueAllocationProjection{}, false
			}
		case AtomClassOpaque:
			// This bit is set only after values.Equal and ClassifyAtom have
			// authenticated the exact owner-issued opaque atom.  It is not a
			// fallback for a missing or malformed relation.
			projection.opaque = true
		}
	}
	return projection, true
}

// Valid reports whether the projection was produced by
// ProjectValueAllocations.  A valid scalar or Bottom projection has no exact
// roots and is still distinguishable through IsBottom and IsTop.
func (projection ValueAllocationProjection) Valid() bool {
	return projection.valid && projection.schema.Valid() && projection.count >= 0
}

// IsBottom reports whether the authenticated Value relation was Bottom.
func (projection ValueAllocationProjection) IsBottom() bool {
	return projection.Valid() && projection.bottom
}

// IsTop reports whether the authenticated Value relation was Top.
func (projection ValueAllocationProjection) IsTop() bool {
	return projection.Valid() && projection.top
}

// HasOpaque reports whether the authenticated Value relation contained an
// opaque reference alternative.  Opaque is an explicit Value fact, not a
// substitute for missing evidence.
func (projection ValueAllocationProjection) HasOpaque() bool {
	return projection.Valid() && projection.opaque
}

// Widened reports whether the caller must account for the complete
// allocation-root denominator because the relation was Top or contained an
// authenticated opaque reference alternative.
func (projection ValueAllocationProjection) Widened() bool {
	return projection.IsTop() || projection.HasOpaque()
}

// ExactCount reports the number of distinct exact allocation roots retained
// by the projection.  Top has no finite exact image and therefore reports
// zero; mixed opaque relations retain their exact roots for callers that need
// them even though Widened is true.
func (projection ValueAllocationProjection) ExactCount() int {
	if !projection.Valid() || projection.top || projection.count < 0 {
		return 0
	}
	return projection.count
}

// ExactAt returns one exact allocation root in canonical dense order.
func (projection ValueAllocationProjection) ExactAt(index int) (heap.Key, bool) {
	if index < 0 || index >= projection.ExactCount() {
		return heap.Key{}, false
	}
	if index < len(projection.inline) {
		return projection.inline[index], true
	}
	overflow := index - len(projection.inline)
	if overflow < 0 || overflow >= len(projection.extra) {
		return heap.Key{}, false
	}
	return projection.extra[overflow], true
}

func (projection *ValueAllocationProjection) addExact(dense int, key heap.Key) bool {
	if projection == nil || !projection.Valid() || projection.top || dense < 0 || !key.Valid() || key.Kind() != heap.RootAllocation {
		return false
	}
	position := 0
	heapSchema := projection.schema.Heap()
	for position < projection.count {
		prior, priorOK := projection.ExactAt(position)
		if !priorOK {
			return false
		}
		priorDense, priorOK := heapSchema.AllocationKeyIndex(prior)
		if !priorOK {
			return false
		}
		switch {
		case priorDense == dense:
			return prior == key
		case priorDense > dense:
			goto insert
		default:
			position++
		}
	}

insert:
	if projection.count < len(projection.inline) {
		for index := projection.count; index > position; index-- {
			projection.inline[index] = projection.inline[index-1]
		}
		projection.inline[position] = key
		projection.count++
		return true
	}
	if position < len(projection.inline) {
		carried := projection.inline[len(projection.inline)-1]
		for index := len(projection.inline) - 1; index > position; index-- {
			projection.inline[index] = projection.inline[index-1]
		}
		projection.inline[position] = key
		projection.extra = append(projection.extra, heap.Key{})
		copy(projection.extra[1:], projection.extra[:len(projection.extra)-1])
		projection.extra[0] = carried
	} else {
		overflow := position - len(projection.inline)
		if overflow < 0 || overflow > len(projection.extra) {
			return false
		}
		projection.extra = append(projection.extra, heap.Key{})
		copy(projection.extra[overflow+1:], projection.extra[overflow:len(projection.extra)-1])
		projection.extra[overflow] = key
	}
	projection.count++
	return true
}
