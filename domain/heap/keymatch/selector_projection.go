package keymatch

import (
	"sync"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// SelectorProjection is the sealed, owner-fenced projection from one Value
// relation to its distinct Heap key-selection operands.  It is the one place
// that quotients multiple Value alternatives which denote the same Heap
// selector (for example, many Summary table references all denote
// Kinds(Table)).  Consumers receive canonical Heap selectors and do not
// retain a second selector identity or their own deduplication rule.
//
// The same authority also owns the wider quotient every Heap construction
// observes: an alternative reaches Heap only through its table-key validity,
// its nil membership, its key selector, and its child-edge containment.
// VisitClasses and VisitPayloadClasses issue that quotient so no consumer
// invents a second classification of the same atoms.
//
// The atom-to-selector index is immutable declaration support.  scratch is
// short-lived implementation storage used only to mark already-emitted rows
// during one observation; it is not semantic State and never survives a
// projection call.
type SelectorProjection struct {
	heap   heapdomain.Schema
	values *valuedomain.Schema

	byAtom       map[valuedomain.Atom]atomRows
	selectors    []heapdomain.KeySelector
	classes      []atomClass
	payloadCount int
	scratch      sync.Pool
}

// atomRows is one atom's two sealed row identities: its distinct Heap
// selector and its distinct heap-observable class.  Row zero means the atom
// denotes no valid Heap selector; a class row is always issued.
type atomRows struct {
	selector uint32
	class    uint32
}

// atomClass is one complete heap-observable class.  payload is the coarser
// row shared by every class that a stored-value role cannot tell apart, since
// a payload observes no key selector.
type atomClass struct {
	containment heapdomain.Containment
	selector    uint32
	payload     uint32
	mayBeValid  bool
	mayBeNil    bool
}

type classIdentity struct {
	containment heapdomain.Containment
	selector    uint32
	mayBeValid  bool
	mayBeNil    bool
}

type payloadIdentity struct {
	containment heapdomain.Containment
	mayBeNil    bool
}

// NewSelectorProjection seals the complete finite selector image of one
// already-sealed Value and Heap authority.  Both authorities must retain the
// exact same immutable Heap Schema handle; independently sealed same-Link
// schemas deliberately do not mix.  It introduces no Factor, State
// coordinate, or equality plane.
func NewSelectorProjection(heap heapdomain.Schema, values *valuedomain.Schema) (*SelectorProjection, bool) {
	if values == nil || !values.OwnsHeapSchema(heap) || !heap.LinkOwner().Matches(values.LinkOwner()) || !values.LinkOwner().Available() || !heap.ContentID().Available() || values.AtomCount() == 0 {
		return nil, false
	}
	projection := &SelectorProjection{
		heap:   heap,
		values: values,
		byAtom: make(map[valuedomain.Atom]atomRows, values.AtomCount()),
	}
	index := &projectionIndex{
		bySelector: make(map[selectorIdentity]uint32, values.AtomCount()),
		byClass:    make(map[classIdentity]uint32, values.AtomCount()),
		byPayload:  make(map[payloadIdentity]uint32, values.AtomCount()),
	}
	complete := values.VisitSupport(values.Top(), func(atom valuedomain.Atom) {
		if !completeSelectorProjectionAtom(projection, index, atom) {
			projection = nil
		}
	})
	if !complete || projection == nil || len(projection.selectors) == 0 || len(projection.classes) == 0 {
		return nil, false
	}
	projection.scratch.New = func() any {
		return &selectorProjectionScratch{
			marks:        make([]uint32, len(projection.selectors)),
			classMarks:   make([]uint32, len(projection.classes)),
			payloadMarks: make([]uint32, projection.payloadCount),
		}
	}
	projection.scratch.Put(projection.scratch.New())
	return projection, true
}

// Visit emits every distinct valid Heap selector denoted by value exactly
// once.  Emission order is the canonical Value support order; Value normalizes
// every relation into that order before this projection sees it.  Nil/NaN
// alternatives have no valid Heap selector and are intentionally omitted.
//
// A false result is an owner, value, or visitor failure.  A value with no
// valid key alternative succeeds without invoking visit.
func (projection *SelectorProjection) Visit(value valuedomain.Value, visit func(heapdomain.KeySelector) bool) bool {
	if !projection.usable() || visit == nil {
		return false
	}
	scratch, ok := projection.scratch.Get().(*selectorProjectionScratch)
	if !ok || scratch == nil || len(scratch.marks) != len(projection.selectors) {
		return false
	}
	defer projection.scratch.Put(scratch)
	epoch := scratch.nextEpoch()

	valid := true
	if !projection.values.VisitSupport(value, func(atom valuedomain.Atom) {
		if !valid {
			return
		}
		row := projection.byAtom[atom].selector
		if row == 0 {
			return
		}
		index := int(row - 1)
		if index < 0 || index >= len(projection.selectors) {
			valid = false
			return
		}
		if scratch.marks[index] == epoch {
			return
		}
		scratch.marks[index] = epoch
		valid = visit(projection.selectors[index])
	}) {
		return false
	}
	return valid
}

// VisitClasses emits one representative alternative per distinct
// heap-observable class denoted by value.  A Heap construction reads an
// alternative only through its table-key validity, its nil membership, its
// key selector, and its child-edge containment, so two alternatives that
// agree on all four produce the identical cell and the identical selection at
// every use.  The quotient is therefore exact, not a precision policy: it
// removes duplicates a consumer would otherwise construct and immediately
// discard.  Emission order is the canonical Value support order.
func (projection *SelectorProjection) VisitClasses(value valuedomain.Value, visit func(valuedomain.Atom) bool) bool {
	return projection.visitQuotient(value, false, visit)
}

// VisitPayloadClasses is the same quotient restricted to what a stored-value
// role can observe: nil membership and the child-edge containment.  A payload
// selects no key, so alternatives that differ only in their key selector are
// one class here.
func (projection *SelectorProjection) VisitPayloadClasses(value valuedomain.Value, visit func(valuedomain.Atom) bool) bool {
	return projection.visitQuotient(value, true, visit)
}

func (projection *SelectorProjection) visitQuotient(value valuedomain.Value, payloadRole bool, visit func(valuedomain.Atom) bool) bool {
	if !projection.usable() || visit == nil {
		return false
	}
	scratch, ok := projection.scratch.Get().(*selectorProjectionScratch)
	if !ok || scratch == nil || len(scratch.classMarks) != len(projection.classes) || len(scratch.payloadMarks) != projection.payloadCount {
		return false
	}
	defer projection.scratch.Put(scratch)
	epoch := scratch.nextEpoch()
	marks := scratch.classMarks
	if payloadRole {
		marks = scratch.payloadMarks
	}

	valid := true
	if !projection.values.VisitSupport(value, func(atom valuedomain.Atom) {
		if !valid {
			return
		}
		rows, known := projection.byAtom[atom]
		if !known || rows.class == 0 || int(rows.class) > len(projection.classes) {
			// An alternative outside the sealed image keeps its own identity.
			// The quotient never merges what it could not classify.
			valid = visit(atom)
			return
		}
		index := int(rows.class) - 1
		if payloadRole {
			index = int(projection.classes[index].payload) - 1
		}
		if index < 0 || index >= len(marks) {
			valid = false
			return
		}
		if marks[index] == epoch {
			return
		}
		marks[index] = epoch
		valid = visit(atom)
	}) {
		return false
	}
	return valid
}

// FencedTo is the sealed projection's owner fence. A consumer that receives
// this projection from the seal proves it belongs to the exact Heap and Value
// authorities it is about to read, instead of building a second projection to
// be sure.
func (projection *SelectorProjection) FencedTo(heap heapdomain.Schema, values *valuedomain.Schema) bool {
	return projection.usable() && projection.heap == heap && projection.values == values
}

func (projection *SelectorProjection) usable() bool {
	return projection != nil && projection.values != nil && projection.values.OwnsHeapSchema(projection.heap) &&
		projection.heap.LinkOwner().Matches(projection.values.LinkOwner()) && projection.values.LinkOwner().Available()
}

type selectorProjectionScratch struct {
	marks        []uint32
	classMarks   []uint32
	payloadMarks []uint32
	// epoch is a reuse generation, not a solver iteration bound.  A rare
	// representation wrap clears only ephemeral marks and continues the same
	// complete projection law.
	epoch uint32
}

func (scratch *selectorProjectionScratch) nextEpoch() uint32 {
	if scratch == nil {
		return 0
	}
	scratch.epoch++
	if scratch.epoch == 0 {
		clear(scratch.marks)
		clear(scratch.classMarks)
		clear(scratch.payloadMarks)
		scratch.epoch = 1
	}
	return scratch.epoch
}

// projectionIndex is private seal bookkeeping for the three row tables. It
// exists only while NewSelectorProjection walks the sealed atom universe.
type projectionIndex struct {
	bySelector map[selectorIdentity]uint32
	byClass    map[classIdentity]uint32
	byPayload  map[payloadIdentity]uint32
}

func completeSelectorProjectionAtom(projection *SelectorProjection, index *projectionIndex, atom valuedomain.Atom) bool {
	if projection == nil || index == nil || !projection.values.OwnsAtom(atom) {
		return false
	}
	containment, containmentOK := Containment(projection.heap, projection.values, atom)
	if !containmentOK {
		return false
	}
	mayBeValid := atom.TableKeyValidity().MayBeValid()
	selectorRow := uint32(0)
	alternative, projected := Project(projection.heap, projection.values, atom)
	if projected {
		identity, ok := selectorProjectionIdentity(alternative.Selector())
		if !ok {
			return false
		}
		selectorRow = index.bySelector[identity]
		if selectorRow == 0 {
			if uint64(len(projection.selectors)) >= uint64(^uint32(0)) {
				return false
			}
			projection.selectors = append(projection.selectors, alternative.Selector())
			selectorRow = uint32(len(projection.selectors))
			index.bySelector[identity] = selectorRow
		}
	} else if mayBeValid {
		// Invalid table keys (nil and NaN) have no selector row by
		// construction.  Every other source is represented by the exact
		// owner-issued selector from Project, never by a fallback invented
		// here.
		return false
	}

	class := atomClass{
		containment: containment,
		selector:    selectorRow,
		mayBeValid:  mayBeValid,
		mayBeNil:    atom.RuntimeKinds().Contains(runtimekind.Nil),
	}
	payload := payloadIdentity{containment: containment, mayBeNil: class.mayBeNil}
	class.payload = index.byPayload[payload]
	if class.payload == 0 {
		if uint64(projection.payloadCount) >= uint64(^uint32(0)) {
			return false
		}
		projection.payloadCount++
		class.payload = uint32(projection.payloadCount)
		index.byPayload[payload] = class.payload
	}
	identity := classIdentity{containment: containment, selector: selectorRow, mayBeValid: class.mayBeValid, mayBeNil: class.mayBeNil}
	classRow := index.byClass[identity]
	if classRow == 0 {
		if uint64(len(projection.classes)) >= uint64(^uint32(0)) {
			return false
		}
		projection.classes = append(projection.classes, class)
		classRow = uint32(len(projection.classes))
		index.byClass[identity] = classRow
	}
	projection.byAtom[atom] = atomRows{selector: selectorRow, class: classRow}
	return true
}

// selectorIdentity is private projection bookkeeping.  Project maps one Atom
// only to an exact singleton or a kind selector; accepting a Finite selector
// here would silently broaden the atom-to-key contract and must instead be an
// explicit keymatch design change.
type selectorIdentity struct {
	kind      heapdomain.KeySelectorKind
	exact     heapdomain.ExactKey
	reference heapdomain.Reference
	kinds     runtimekind.Set
}

func selectorProjectionIdentity(selector heapdomain.KeySelector) (selectorIdentity, bool) {
	if !selector.Valid() {
		return selectorIdentity{}, false
	}
	switch selector.Kind() {
	case heapdomain.KeySelectorAtom:
		if selector.ExactCount() == 1 && selector.ReferenceCount() == 0 {
			exact, ok := selector.ExactAt(0)
			return selectorIdentity{kind: heapdomain.KeySelectorAtom, exact: exact}, ok
		}
		if selector.ExactCount() == 0 && selector.ReferenceCount() == 1 {
			reference, ok := selector.ReferenceAt(0)
			return selectorIdentity{kind: heapdomain.KeySelectorAtom, reference: reference}, ok
		}
		return selectorIdentity{}, false
	case heapdomain.KeySelectorKinds:
		kinds := selector.RuntimeKinds()
		if kinds == 0 || !kinds.Valid() {
			return selectorIdentity{}, false
		}
		return selectorIdentity{kind: heapdomain.KeySelectorKinds, kinds: kinds}, true
	default:
		return selectorIdentity{}, false
	}
}
