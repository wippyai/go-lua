package value

import (
	"math/bits"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// Value is one immutable normalized correlated relation over a Schema's
// sealed atoms and Capability(atom, capability) pairs.  Its image is one
// compact slice: each fixed-width row starts with a one-based atom ID followed
// by the owning schema's capability bit words.  State retains neither
// structural roots nor 32-byte content IDs nor static types.
type Value struct {
	schema *Schema
	image  []uint64
	top    bool
}

func (value Value) valid() bool {
	if value.schema == nil || value.schema.potential == 0 {
		return false
	}
	if value.top {
		return len(value.image) == 0
	}
	stride := value.schema.stride()
	if stride == 0 || len(value.image)%stride != 0 {
		return false
	}
	var previous uint64
	for offset := 0; offset < len(value.image); offset += stride {
		atom := value.image[offset]
		if atom == 0 || atom > uint64(len(value.schema.atoms)) || offset != 0 && previous >= atom {
			return false
		}
		previous = atom
	}
	return true
}

// IsBottom and IsTop expose only the two owner-fenced lattice extremes.
// They do not enumerate or project a correlated alternative.
func (value Value) IsBottom() bool { return value.valid() && !value.top && len(value.image) == 0 }
func (value Value) IsTop() bool    { return value.valid() && value.top }

func (schema *Schema) stride() int {
	if schema == nil {
		return 0
	}
	return 1 + schema.capWords
}

// Bottom is the sparse Factor default: it denotes no reachable concrete
// Value alternative, not Lua nil.
func (schema *Schema) Bottom() Value {
	if schema == nil {
		return Value{}
	}
	return schema.bottom
}

// Default is the sparse Value Factor default.
func (schema *Schema) Default() Value { return schema.Bottom() }

// Top is the constant all-alternatives image.  It is never expanded into an
// atom×capability matrix in hot State.
func (schema *Schema) Top() Value {
	if schema == nil {
		return Value{}
	}
	return schema.top
}

// Singleton constructs one exact alternative without any capability.  The
// atom must belong to this exact schema.
func (schema *Schema) Singleton(atom Atom) (Value, bool) {
	if schema == nil || !atom.valid() || atom.schema != schema {
		return Value{}, false
	}
	image := make([]uint64, schema.stride())
	image[0] = uint64(atom.id)
	return schema.canonical(image), true
}

// Alternatives constructs the normalized union of exact owner-bound atoms.
// Capability is deliberately absent until the explicit attached relation is
// requested; no source type or endpoint can silently invent it.
func (schema *Schema) Alternatives(atoms ...Atom) (Value, bool) {
	if schema == nil {
		return Value{}, false
	}
	if len(atoms) == 0 {
		return schema.Bottom(), true
	}
	ids := make([]uint32, len(atoms))
	for index, atom := range atoms {
		if !atom.valid() || atom.schema != schema {
			return Value{}, false
		}
		ids[index] = atom.id
	}
	for index := 1; index < len(ids); index++ {
		current := ids[index]
		cursor := index
		for cursor != 0 && ids[cursor-1] > current {
			ids[cursor] = ids[cursor-1]
			cursor--
		}
		ids[cursor] = current
	}
	unique := ids[:0]
	for _, id := range ids {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	image := make([]uint64, len(unique)*schema.stride())
	for index, id := range unique {
		image[index*schema.stride()] = uint64(id)
	}
	return schema.canonical(image), true
}

// SourceValue returns a source literal/runtime-TypeValue alternative at an
// existing Link Value coordinate. Allocation deliberately has no source fact:
// its sole executable authority is the allocation Rule's atomic Age+Fresh
// patch. Dynamic source families likewise come only from their exact Rule.
func (schema *Schema) SourceValueID(subject identity.ContentID) (Value, bool) {
	if schema == nil || !subject.Available() {
		return Value{}, false
	}
	row, ok := schema.coordinates[subject]
	if !ok || row.atom == 0 || !schema.owns(row.source) || schema.Equal(row.source, schema.Bottom()) {
		return Value{}, false
	}
	return row.source, true
}

// Equal is exact owner-local relation equality.  Equal and all other algebra
// entry points reject another Link/Schema rather than treating it as bottom.
func (schema *Schema) Equal(left, right Value) bool {
	if !schema.owns(left) || !schema.owns(right) || left.top != right.top || len(left.image) != len(right.image) {
		return false
	}
	for index := range left.image {
		if left.image[index] != right.image[index] {
			return false
		}
	}
	return true
}

// Same recognizes the immutable image fast path, falling back to semantic
// equality only when an independently built but equal image is compared.
func (schema *Schema) Same(left, right Value) bool {
	if !schema.owns(left) || !schema.owns(right) {
		return false
	}
	if left.top || right.top || len(left.image) == 0 || len(right.image) == 0 {
		return schema.Equal(left, right)
	}
	return &left.image[0] == &right.image[0] || schema.Equal(left, right)
}

func (schema *Schema) owns(value Value) bool {
	return schema != nil && value.schema == schema && value.valid()
}

// LessOrEq is relation inclusion: an atom must occur on the right and every
// capability attached to that atom on the left must occur on the right.  This
// is the capability non-smearing law in the partial order itself.
func (schema *Schema) LessOrEq(left, right Value) bool {
	if !schema.owns(left) || !schema.owns(right) {
		return false
	}
	// Top has an empty compact image, so it must be handled before sparse
	// Bottom.  Treating its nil slice as Bottom collapses the order and is
	// exactly the kind of representation leak this owner must forbid.
	if left.top {
		return right.top
	}
	if right.top {
		return true
	}
	if len(left.image) == 0 {
		return true
	}
	if len(right.image) == 0 {
		return false
	}
	stride := schema.stride()
	for leftAt, rightAt := 0, 0; leftAt < len(left.image); leftAt += stride {
		leftID := left.image[leftAt]
		for rightAt < len(right.image) && right.image[rightAt] < leftID {
			rightAt += stride
		}
		if rightAt == len(right.image) || right.image[rightAt] != leftID || !schema.capabilitySubset(left.image[leftAt+1:leftAt+stride], right.image[rightAt+1:rightAt+stride]) {
			return false
		}
	}
	return true
}

func (schema *Schema) capabilitySubset(left, right []uint64) bool {
	if len(left) != schema.capWords || len(right) != schema.capWords {
		return false
	}
	for index := range left {
		if left[index]&^right[index] != 0 {
			return false
		}
	}
	return true
}

// Join returns the finite least upper bound.  The exact image is retained
// when one input already covers the other; only a real relation expansion
// allocates one new compact immutable image.
func (schema *Schema) Join(left, right Value) (Value, bool) {
	if !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	if left.top || right.top {
		return schema.Top(), true
	}
	if len(left.image) == 0 {
		return right, true
	}
	if len(right.image) == 0 {
		return left, true
	}
	if schema.LessOrEq(left, right) {
		return right, true
	}
	if schema.LessOrEq(right, left) {
		return left, true
	}
	stride := schema.stride()
	result := make([]uint64, 0, len(left.image)+len(right.image))
	leftAt, rightAt := 0, 0
	for leftAt < len(left.image) && rightAt < len(right.image) {
		leftID, rightID := left.image[leftAt], right.image[rightAt]
		switch {
		case leftID < rightID:
			result = append(result, left.image[leftAt:leftAt+stride]...)
			leftAt += stride
		case leftID > rightID:
			result = append(result, right.image[rightAt:rightAt+stride]...)
			rightAt += stride
		default:
			result = append(result, leftID)
			for word := 0; word < schema.capWords; word++ {
				result = append(result, left.image[leftAt+1+word]|right.image[rightAt+1+word])
			}
			leftAt += stride
			rightAt += stride
		}
	}
	result = append(result, left.image[leftAt:]...)
	result = append(result, right.image[rightAt:]...)
	return schema.canonical(result), true
}

// Widen is exact finite relation union.  Its WidenRank supplies the strict
// remaining-alternative proof required when Value participates in a cyclic
// executor tuple.
func (schema *Schema) Widen(previous, next Value) (Value, bool) {
	return schema.Join(previous, next)
}

// Domain exposes the exact schema-local lattice.  The generic carrier cannot
// return an admission failure, so a foreign value is a composition defect and
// panics rather than becoming an unobservable false positive.
func (schema *Schema) Domain() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom:   schema.Bottom,
		Top:      schema.Top,
		Equal:    schema.Equal,
		Same:     schema.Same,
		LessOrEq: schema.LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := schema.Join(left, right)
			if !ok {
				panic("value: foreign correlated relation")
			}
			return value
		},
		Widen: func(previous, next Value) Value {
			value, ok := schema.Widen(previous, next)
			if !ok {
				panic("value: foreign correlated relation")
			}
			return value
		},
	}
}

// WidenRank is the exact remaining finite relation capacity plus the final
// transition to constant Top.  The rank is undefined at Top because no strict
// successor exists there.
func (schema *Schema) WidenRank(value Value) (uint64, bool) {
	if !schema.owns(value) || value.top {
		return 0, false
	}
	occupied := schema.occupancy(value)
	if occupied >= schema.potential {
		return 0, false
	}
	return schema.potential - occupied + 1, true
}

// WidenMeasure is the total Factor-facing rank projection. Zero denotes Top
// or an invalid foreign fact, neither of which has a strict successor.
func (schema *Schema) WidenMeasure(value Value) uint64 {
	rank, ok := schema.WidenRank(value)
	if !ok {
		return 0
	}
	return rank
}

func (schema *Schema) occupancy(value Value) uint64 {
	if !schema.owns(value) {
		return 0
	}
	if value.top {
		return schema.potential
	}
	stride := schema.stride()
	occupied := uint64(0)
	for offset := 0; offset < len(value.image); offset += stride {
		occupied++
		for word := 1; word < stride; word++ {
			occupied += uint64(bits.OnesCount64(value.image[offset+word]))
		}
	}
	return occupied
}

func (schema *Schema) canonical(image []uint64) Value {
	value := Value{schema: schema, image: image}
	if schema.occupancy(value) == schema.potential {
		return schema.Top()
	}
	return value
}

// Fingerprint is a stable local image hash.  Schema fencing happens before
// this method, so it never stores or hashes a Link ContentID in Value state.
func (schema *Schema) Fingerprint(value Value) uint64 {
	if !schema.owns(value) {
		return 0
	}
	hash := uint64(0xcbf29ce484222325)
	if value.top {
		return mix(hash, 0xffffffffffffffff)
	}
	for _, word := range value.image {
		hash = mix(hash, word)
	}
	return hash
}

func mix(hash, value uint64) uint64 {
	hash ^= value
	return hash * 0x100000001b3
}

// WithCapability attaches one exact sealed provider capability to one atom
// already present in the relation.  It cannot add a capability to another
// atom, so an alias/capability Rule must carry the same receiver alternative.
func (schema *Schema) WithCapability(value Value, atom Atom, capability identity.ContentID) (Value, bool) {
	if !schema.owns(value) || !atom.valid() || atom.schema != schema {
		return Value{}, false
	}
	capabilityID := schema.capabilityID[capability]
	if capabilityID == 0 {
		return Value{}, false
	}
	if value.top {
		return schema.Top(), true
	}
	stride := schema.stride()
	for offset := 0; offset < len(value.image); offset += stride {
		if value.image[offset] != uint64(atom.id) {
			continue
		}
		word, bit := (capabilityID-1)/64, uint((capabilityID-1)%64)
		if value.image[offset+1+int(word)]&(uint64(1)<<bit) != 0 {
			return value, true
		}
		image := append([]uint64(nil), value.image...)
		image[offset+1+int(word)] |= uint64(1) << bit
		return schema.canonical(image), true
	}
	return Value{}, false
}

// Age is the total Value-local recency transition for one exact Heap
// allocation key. It maps Recent(key) to Summary(key), retaining every
// attached capability. All other alternatives are unchanged. Age never
// manufactures a fresh alternative: the allocation Rule must make its result
// a separate strong write in the same contribution that applies this carry
// transform.
//
// Sealing orders Recent(root) immediately before Summary(root). The rewrite
// therefore preserves canonical order and can merge a pre-existing Summary
// row in one linear pass; it neither sorts nor allocates per-row storage.
func (schema *Schema) Age(value Value, key heap.Key) (Value, bool) {
	if !schema.owns(value) || !schema.heap.OwnsKey(key) || key.Kind() != heap.RootAllocation {
		return Value{}, false
	}
	reference := schema.allocRefs[key]
	if reference == 0 {
		return Value{}, false
	}
	recent, recentOK := schema.referenceAtom(reference, materialization.Recent)
	summary, summaryOK := schema.referenceAtom(reference, materialization.Summary)
	if !recentOK || !summaryOK || summary != recent+1 {
		return Value{}, false
	}
	if value.top || len(value.image) == 0 {
		return value, true
	}

	stride := schema.stride()
	var changed bool
	for offset := 0; offset < len(value.image); offset += stride {
		if value.image[offset] == uint64(recent) {
			changed = true
			break
		}
	}
	if !changed {
		return value, true
	}

	image := make([]uint64, 0, len(value.image))
	for offset := 0; offset < len(value.image); offset += stride {
		id := value.image[offset]
		if id == uint64(recent) {
			id = uint64(summary)
		}
		if len(image) != 0 && image[len(image)-stride] == id {
			for word := 1; word < stride; word++ {
				image[len(image)-stride+word] |= value.image[offset+word]
			}
			continue
		}
		image = append(image, id)
		image = append(image, value.image[offset+1:offset+stride]...)
	}
	return schema.canonical(image), true
}

// HasCapability reads Capability(atom, capability) from one exact Value
// relation.  It is a projection of the correlated carrier, never a separate
// capability map or Factor.
func (schema *Schema) HasCapability(value Value, atom Atom, capability identity.ContentID) bool {
	if !schema.owns(value) || !atom.valid() || atom.schema != schema {
		return false
	}
	capabilityID := schema.capabilityID[capability]
	if capabilityID == 0 {
		return false
	}
	if value.top {
		return true
	}
	stride := schema.stride()
	for offset := 0; offset < len(value.image); offset += stride {
		if value.image[offset] == uint64(atom.id) {
			word, bit := (capabilityID-1)/64, uint((capabilityID-1)%64)
			return value.image[offset+1+int(word)]&(uint64(1)<<bit) != 0
		}
	}
	return false
}

// Presence projects nilability from the complete correlated relation.  A
// rooted or primitive atom is non-nil; only atomNil is absent.
func (schema *Schema) Presence(value Value) Presence {
	if !schema.owns(value) {
		return PresenceNone
	}
	if value.top {
		return PresenceAbsent | PresencePresent
	}
	stride := schema.stride()
	var result Presence
	for offset := 0; offset < len(value.image); offset += stride {
		if schema.atoms[value.image[offset]-1].kind == atomNil {
			result |= PresenceAbsent
		} else {
			result |= PresencePresent
		}
	}
	return result
}

// RuntimeKinds projects the exact may-kind set from the same atom image.
func (schema *Schema) RuntimeKinds(value Value) runtimekind.Set {
	if !schema.owns(value) {
		return 0
	}
	if value.top {
		return runtimekind.All
	}
	stride := schema.stride()
	var result runtimekind.Set
	for offset := 0; offset < len(value.image); offset += stride {
		result |= schema.atomKinds(uint32(value.image[offset]))
	}
	return result
}

// Truthiness projects exact Lua branch behavior from the same correlated
// alternatives.  It never rebuilds a Boolean/nilability product.
func (schema *Schema) Truthiness(value Value) Truth {
	if !schema.owns(value) {
		return TruthNone
	}
	if value.top {
		return TruthFalse | TruthTrue
	}
	stride := schema.stride()
	var result Truth
	for offset := 0; offset < len(value.image); offset += stride {
		result |= schema.atomTruth(uint32(value.image[offset]))
	}
	return result
}

// Atoms returns a cold, isolated owner-bound view of the relation's atom set.
// Capabilities remain attached in Value; use HasCapability for their exact
// correlated query rather than obtaining a parallel marginal collection.
func (schema *Schema) Atoms(value Value) ([]Atom, bool) {
	if !schema.owns(value) || value.top {
		return nil, false
	}
	stride := schema.stride()
	result := make([]Atom, 0, len(value.image)/stride)
	for offset := 0; offset < len(value.image); offset += stride {
		result = append(result, Atom{schema: schema, id: uint32(value.image[offset])})
	}
	return result, true
}

// VisitAtoms traverses the exact non-Top atom relation without materializing
// a second slice.  Cross-factor reductions use this hot path when they need
// rooted alternatives while preserving Value's correlated capability image.
// Top deliberately has no enumerable exact support and returns false: its
// consumer must apply its own lawful all-support reduction instead.
func (schema *Schema) VisitAtoms(value Value, visit func(Atom) bool) bool {
	if !schema.owns(value) || value.top || visit == nil {
		return false
	}
	stride := schema.stride()
	for offset := 0; offset < len(value.image); offset += stride {
		if !visit(Atom{schema: schema, id: uint32(value.image[offset])}) {
			return true
		}
	}
	return true
}

// VisitSupport visits the complete exact support of one owned Value relation
// in canonical schema order. Unlike VisitAtoms, Top visits every sealed Atom
// once; Bottom visits none. Cross-domain structural projections use this
// total support iterator instead of approximating Top from runtime kinds or
// from another domain's root table.
func (schema *Schema) VisitSupport(value Value, visit func(Atom)) bool {
	if !schema.owns(value) || visit == nil {
		return false
	}
	if value.top {
		for index := range schema.atoms {
			visit(Atom{schema: schema, id: uint32(index + 1)})
		}
		return true
	}
	stride := schema.stride()
	for offset := 0; offset < len(value.image); offset += stride {
		visit(Atom{schema: schema, id: uint32(value.image[offset])})
	}
	return true
}
