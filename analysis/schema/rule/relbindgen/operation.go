package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
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

// SolveLocal is an operation that carries per-invocation storage.
//
// Most operations hold only sealed owner state and are safe to share, so the
// substrate shares one. An operation that materializes a delivered span into
// the operand vocabulary a fold reads holds storage it refills, and sharing
// that across solve-local workers would be a race. Such an operation says so
// by answering with its own, and the substrate gives each worker one.
type SolveLocal[A, R any] interface {
	Operation[A, R]
	NewOperation() Operation[A, R]
}

type emission[R any] struct {
	key      identity.ContentID
	value    R
	presence model.Presence
}

// Emitter is the operation's bounded typed result sink. Its capacity is the
// sealed signature's declared output cardinality, so an expansion is finite
// under its declared denominator by construction: the emitter refuses the row
// that would exceed the bound and the invocation refuses as a whole.
type Emitter[R any] struct {
	rows     []emission[R]
	limit    int
	keyed    bool
	fallback identity.ContentID
	overflow bool
}

// Cap returns the declared output row bound.
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

// Put emits one present row at the destination addressed by the binding's
// declared address slot. A keyed expansion has no such address and refuses.
func (emitter *Emitter[R]) Put(value R) bool {
	return emitter.addressed(value, model.Present)
}

// PutOpaque emits one authenticated but intentionally opaque row.
func (emitter *Emitter[R]) PutOpaque(value R) bool {
	return emitter.addressed(value, model.AuthenticatedOpaque)
}

// PutAbsent emits proven absence at the addressed destination. Absence is a
// published fact, never a fabricated bottom value.
func (emitter *Emitter[R]) PutAbsent() bool {
	var zero R
	return emitter.addressed(zero, model.ProvenAbsent)
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

func (emitter *Emitter[R]) addressed(value R, kind model.PresenceKind) bool {
	if emitter == nil || emitter.keyed {
		return emitter.fail()
	}
	return emitter.append(emitter.fallback, value, kind)
}

func (emitter *Emitter[R]) named(key identity.ContentID, value R, kind model.PresenceKind) bool {
	if emitter == nil || !emitter.keyed || !key.Available() {
		return emitter.fail()
	}
	return emitter.append(key, value, kind)
}

func (emitter *Emitter[R]) append(key identity.ContentID, value R, kind model.PresenceKind) bool {
	presence, ok := model.NewPresence(kind)
	if !ok || emitter.overflow || len(emitter.rows) == emitter.limit {
		return emitter.fail()
	}
	for _, staged := range emitter.rows {
		if staged.key == key {
			return emitter.fail()
		}
	}
	emitter.rows = append(emitter.rows, emission[R]{key: key, value: value, presence: presence})
	return true
}

func (emitter *Emitter[R]) fail() bool {
	if emitter != nil {
		emitter.overflow = true
		emitter.rows = emitter.rows[:0]
	}
	return false
}

func (emitter *Emitter[R]) reset(fallback identity.ContentID, keyed bool) {
	emitter.rows = emitter.rows[:0]
	emitter.fallback = fallback
	emitter.keyed = keyed
	emitter.overflow = false
}
