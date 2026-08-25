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

// AlgebraFactory and AlgebraRegistry resolve domain ascent authority by the
// canonical TypeID, independently of any operation signature.
type AlgebraFactory interface {
	Bind(model.TypeID) (ValueAlgebra, bool)
}

type AlgebraRegistry interface {
	Resolve(model.TypeID) (ValueAlgebra, bool)
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
