package binding

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Factory admits generated typed adapters for one sealed operation.
type Factory interface {
	Bind(signature.Signature) (Binding, bool)
}

// Binding carries the exact signature and creates solve-local workers. It
// exposes no runtime state or generic function field.
type Binding interface {
	Signature() signature.Signature
	NewWorker(Fence) (Worker, bool)
}

// Worker is the generated adapter execution contract. It receives only the
// immutable frame and its signature-bound proposal buffer.
type Worker interface {
	Evaluate(Frame, *ProposalBuffer) outcome.Result
}

// ValueAlgebra is the generated typed authority used by state validation to
// prove joins, widenings, and ascent over authenticated values.
type ValueAlgebra interface {
	Type() model.TypeID
	Join(ValueToken, ValueToken) (ValueToken, bool)
	Widen(ValueToken, ValueToken) (ValueToken, bool)
	LessOrEqual(ValueToken, ValueToken) bool
}

// ValueEquality is the owner-supplied semantic equality authority for one
// Equatable TypeID. It is deliberately narrower than ValueAlgebra: key
// comparison may need to identify two differently encoded values without
// granting Join, Widen, or monotone ascent.
type ValueEquality interface {
	Type() model.TypeID
	Equal(ValueToken, ValueToken) bool
}

// AlgebraFactory and AlgebraRegistry resolve domain ascent authority by the
// canonical TypeID, independently of any operation signature.
type AlgebraFactory interface {
	Bind(model.TypeID) (ValueAlgebra, bool)
}

type AlgebraRegistry interface {
	Resolve(model.TypeID) (ValueAlgebra, bool)
}

// EqualityRegistry optionally accompanies an AlgebraRegistry at mount. It
// supplies the owner's narrow equality witness when one is declared; an
// Ascending owner may instead project equality from its ValueAlgebra without
// putting that algebra into mounted ascent state.
type EqualityRegistry interface {
	ResolveEquality(model.TypeID) (ValueEquality, bool)
}

func AdmitAlgebra(factory AlgebraFactory, typeID model.TypeID) (ValueAlgebra, bool) {
	if factory == nil || !typeID.Available() {
		return nil, false
	}
	algebra, ok := factory.Bind(typeID)
	if !ok || algebra == nil || algebra.Type() != typeID {
		return nil, false
	}
	return algebra, true
}

func ResolveAlgebra(registry AlgebraRegistry, typeID model.TypeID) (ValueAlgebra, bool) {
	if registry == nil || !typeID.Available() {
		return nil, false
	}
	algebra, ok := registry.Resolve(typeID)
	if !ok || algebra == nil || algebra.Type() != typeID {
		return nil, false
	}
	return algebra, true
}

// ResolveEquality admits one exact owner equality authority. A raw token
// identity is never treated as semantic equality at this boundary.
func ResolveEquality(registry EqualityRegistry, typeID model.TypeID) (ValueEquality, bool) {
	if registry == nil || !typeID.Available() {
		return nil, false
	}
	equality, ok := registry.ResolveEquality(typeID)
	if !ok || equality == nil || equality.Type() != typeID {
		return nil, false
	}
	return equality, true
}

// EqualityFromAlgebra projects the equality relation inherent in an owner's
// ValueAlgebra. The mount may resolve that owner algebra solely for an
// equality-only key requirement, but it stores only the narrower projection;
// this function never mints a second algebra or grants Join/Widen authority.
func EqualityFromAlgebra(algebra ValueAlgebra) (ValueEquality, bool) {
	if algebra == nil || !algebra.Type().Available() {
		return nil, false
	}
	return algebraEquality{algebra: algebra}, true
}

type algebraEquality struct{ algebra ValueAlgebra }

func (value algebraEquality) Type() model.TypeID {
	if value.algebra == nil {
		return model.TypeID{}
	}
	return value.algebra.Type()
}

func (value algebraEquality) Equal(left, right ValueToken) bool {
	return value.algebra != nil && left.Available() && right.Available() && left.Type() == value.algebra.Type() && right.Type() == value.algebra.Type() && value.algebra.LessOrEqual(left, right) && value.algebra.LessOrEqual(right, left)
}

// Admit rejects a factory result that widens or changes the requested ABI.
func Admit(factory Factory, operation signature.Signature) (Binding, bool) {
	if factory == nil || !operation.Available() {
		return nil, false
	}
	binding, ok := factory.Bind(operation)
	if !ok || binding == nil {
		return nil, false
	}
	bound := binding.Signature()
	if !bound.Available() || bound.Digest() != operation.Digest() {
		return nil, false
	}
	return binding, true
}
