package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// Decoder assembles one operation's typed argument from the frame's declared
// slots. With Encoder it is the whole of the thin typed semantic-operation
// binding a generator emits for a family.
type Decoder[A any] interface {
	Decode(Inputs) (A, bool)
}

// Encoder publishes one produced result across the signature's declared output
// columns.
type Encoder[R any] interface {
	Encode(Outputs, R) bool
}

// Operation is the owner's judgment. Its signature mentions only a decoded
// domain argument, a bounded Emitter, and the closed outcome vocabulary, so an
// operation cannot read a relation, inspect engine state, mint an engine
// identity, choose an undeclared destination, or publish. That is a property
// of the type, not of a runtime check.
//
// The four semantic classes are not four operation forms. Scalar judgment,
// finite expansion, grouped reduction, and cell update differ only in the
// declared input delivery, the declared output cardinality, and whether the
// destination row is addressed by an input slot or named by the owner.
type Operation[A, R any] interface {
	Evaluate(A, *Emitter[R]) outcome.Code
}

type emission[R any] struct {
	row      model.RowID
	value    R
	presence model.Presence
}

// Emitter is the operation's typed result sink. Numeric cardinalities give it
// their sealed static limit. CompleteDenominator deliberately has no numeric
// limit here: the mounted ProposalBuffer supplies the witness-backed capacity
// and proves exact row/column coverage when the operation settles.
type Emitter[R any] struct {
	rows        []emission[R]
	limit       int
	unbounded   bool
	destination binding.DestinationView
	overflow    bool
}

// Cap returns the declared numeric output row bound. A CompleteDenominator
// emitter has no static numeric bound and reports zero; its mounted proposal
// buffer supplies the witness-backed capacity.
func (emitter *Emitter[R]) Cap() int {
	if emitter == nil {
		return 0
	}
	return emitter.limit
}

// Len returns the number of rows emitted so far.
func (emitter *Emitter[R]) Len() int {
	if emitter == nil {
		return 0
	}
	return len(emitter.rows)
}

// Put emits one present row at the next row of the plan-resolved scalar or
// span destination. OwnerNamed operations must use PutAt instead.
func (emitter *Emitter[R]) Put(value R) bool {
	return emitter.sequential(value, model.Present)
}

// PutOpaque emits one authenticated but intentionally opaque row.
func (emitter *Emitter[R]) PutOpaque(value R) bool {
	return emitter.sequential(value, model.AuthenticatedOpaque)
}

// PutAbsent emits proven absence at the addressed destination. Absence is a
// published fact, never a fabricated bottom value.
func (emitter *Emitter[R]) PutAbsent() bool {
	var zero R
	return emitter.sequential(zero, model.ProvenAbsent)
}

// PutAt emits one present row at the destination row the owner names by its
// own content identity. A binding that declared an address slot refuses.
func (emitter *Emitter[R]) PutAt(key identity.ContentID, value R) bool {
	return emitter.named(key, value, model.Present)
}

// PutOpaqueAt emits one authenticated opaque row at an owner-named row.
func (emitter *Emitter[R]) PutOpaqueAt(key identity.ContentID, value R) bool {
	return emitter.named(key, value, model.AuthenticatedOpaque)
}

// PutAbsentAt emits proven absence at an owner-named destination row.
func (emitter *Emitter[R]) PutAbsentAt(key identity.ContentID) bool {
	var zero R
	return emitter.named(key, zero, model.ProvenAbsent)
}

func (emitter *Emitter[R]) sequential(value R, kind model.PresenceKind) bool {
	if emitter == nil || (!emitter.destination.IsScalar() && !emitter.destination.IsSpan()) {
		return emitter.fail()
	}
	cell, ok := emitter.destination.Next()
	if !ok || !cell.Row().Available() {
		return emitter.fail()
	}
	return emitter.append(cell.Row(), value, kind)
}

func (emitter *Emitter[R]) named(key identity.ContentID, value R, kind model.PresenceKind) bool {
	if emitter == nil || !emitter.destination.IsOwnerNamed() || !key.Available() {
		return emitter.fail()
	}
	relation, ok := emitter.destination.OwnerRelation()
	if !ok {
		return emitter.fail()
	}
	row, ok := model.IssueRowID(relation, key)
	if !ok {
		return emitter.fail()
	}
	return emitter.append(row, value, kind)
}

func (emitter *Emitter[R]) append(row model.RowID, value R, kind model.PresenceKind) bool {
	presence, ok := model.NewPresence(kind)
	if !ok || emitter.overflow || (!emitter.unbounded && len(emitter.rows) == emitter.limit) {
		return emitter.fail()
	}
	for _, staged := range emitter.rows {
		if staged.row == row {
			return emitter.fail()
		}
	}
	emitter.rows = append(emitter.rows, emission[R]{row: row, value: value, presence: presence})
	return true
}

func (emitter *Emitter[R]) fail() bool {
	if emitter != nil {
		emitter.overflow = true
		emitter.rows = emitter.rows[:0]
	}
	return false
}

func (emitter *Emitter[R]) reset(destination binding.DestinationView) {
	emitter.rows = emitter.rows[:0]
	emitter.destination = destination
	emitter.overflow = false
}
