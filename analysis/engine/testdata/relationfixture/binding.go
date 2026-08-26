package testfixture

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// worker is a deliberately small mounted binding used only to feed the
// public apply boundary. It has no physical reader or state access.
type worker struct {
	operation signature.Signature
	proposals map[binding.ScopeToken][]binding.Proposal
}

func (worker *worker) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if worker == nil || buffer == nil || !frame.Available() || worker.proposals == nil {
		return outcome.Result{}
	}
	proposals, ok := worker.proposals[frame.Scope()]
	if !ok {
		return outcome.Result{Code: outcome.NoSelection}
	}
	for _, proposal := range proposals {
		if !buffer.Append(proposal) {
			return outcome.Result{}
		}
	}
	return outcome.Result{Code: outcome.Produced}
}

type bindingFactory struct {
	bindings map[signature.Identity]binding.Binding
}

func (factory bindingFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() {
		return nil, false
	}
	value, ok := factory.bindings[operation.Identity()]
	return value, ok && value != nil
}

type operationBinding struct {
	operation signature.Signature
	worker    binding.Worker
}

func (value operationBinding) Signature() signature.Signature { return value.operation }

func (value operationBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return value.worker, value.worker != nil
}

type algebra struct{ typeID model.TypeID }

func (value algebra) Type() model.TypeID { return value.typeID }

func (value algebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return binding.ValueToken{}, false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	if bytes.Compare(leftOpaque[:], rightOpaque[:]) >= 0 {
		return left, true
	}
	return right, true
}

func (value algebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return binding.ValueToken{}, false
	}
	return right, true
}

func (value algebra) LessOrEqual(left, right binding.ValueToken) bool {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	return bytes.Compare(leftOpaque[:], rightOpaque[:]) <= 0
}

// tokenEquality is this fixture owner's semantic equality for its one test
// codec. The fixture encodes each logical value as its content identity, so
// token identity is the explicitly declared equality relation here. It is
// supplied separately from algebra because a read-only Join needs equality
// without thereby authorizing an ascent.
type tokenEquality struct{ typeID model.TypeID }

func (value tokenEquality) Type() model.TypeID { return value.typeID }

func (value tokenEquality) Equal(left, right binding.ValueToken) bool {
	return left.Available() && right.Available() && left.Type() == value.typeID && right.Type() == value.typeID && left.Same(right)
}

type algebraRegistry struct {
	value    algebra
	equality tokenEquality
}

func (registry algebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.value, registry.value.Type() == typeID
}

func (registry algebraRegistry) ResolveEquality(typeID model.TypeID) (binding.ValueEquality, bool) {
	return registry.equality, registry.equality.Type() == typeID
}
