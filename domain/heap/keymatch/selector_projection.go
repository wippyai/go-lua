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
// The atom-to-selector index is immutable declaration support.  scratch is
// short-lived implementation storage used only to mark already-emitted rows
// during one observation; it is not semantic State and never survives a
// projection call.
type SelectorProjection struct {
	heap   heapdomain.Schema
	values *valuedomain.Schema

	byAtom    map[valuedomain.Atom]uint32
	selectors []heapdomain.KeySelector
	scratch   sync.Pool
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
		byAtom: make(map[valuedomain.Atom]uint32, values.AtomCount()),
	}
	bySelector := make(map[selectorIdentity]uint32, values.AtomCount())
	complete := values.VisitSupport(values.Top(), func(atom valuedomain.Atom) {
		if !completeSelectorProjectionAtom(projection, bySelector, atom) {
			projection = nil
		}
	})
	if !complete || projection == nil || len(projection.selectors) == 0 {
		return nil, false
	}
	projection.scratch.New = func() any {
		return &selectorProjectionScratch{marks: make([]uint32, len(projection.selectors))}
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
	if projection == nil || projection.values == nil || !projection.values.OwnsHeapSchema(projection.heap) || !projection.heap.LinkOwner().Matches(projection.values.LinkOwner()) || !projection.values.LinkOwner().Available() || visit == nil {
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
		row := projection.byAtom[atom]
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

type selectorProjectionScratch struct {
	marks []uint32
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
		scratch.epoch = 1
	}
	return scratch.epoch
}

func completeSelectorProjectionAtom(projection *SelectorProjection, bySelector map[selectorIdentity]uint32, atom valuedomain.Atom) bool {
	if projection == nil || bySelector == nil || !projection.values.OwnsAtom(atom) {
		return false
	}
	alternative, projected := Project(projection.heap, projection.values, atom)
	if !projected {
		// Invalid table keys (nil and NaN) have no row by construction.  Every
		// other source is represented by the exact owner-issued selector from
		// Project, never by a fallback invented here.
		return !atom.TableKeyValidity().MayBeValid()
	}
	identity, ok := selectorProjectionIdentity(alternative.Selector())
	if !ok {
		return false
	}
	row := bySelector[identity]
	if row == 0 {
		if uint64(len(projection.selectors)) >= uint64(^uint32(0)) {
			return false
		}
		projection.selectors = append(projection.selectors, alternative.Selector())
		row = uint32(len(projection.selectors))
		bySelector[identity] = row
	}
	projection.byAtom[atom] = row
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
