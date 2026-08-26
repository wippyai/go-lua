package arithmetic

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// worker is the only domain-shaped object in this fixture.  It receives the
// exact declared Frame slots and stages only mounted destination cells.  It
// never receives a relation reader, physical layout, or a fallback row.
type worker struct {
	operation   signature.Signature
	evaluate    func(binding.Frame, *binding.ProposalBuffer) outcome.Result
	proposals   map[binding.ScopeToken][]binding.Proposal
	evaluations int
}

func (value *worker) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if value == nil {
		return outcome.Result{}
	}
	value.evaluations++
	if value.proposals != nil {
		if proposals, ok := value.proposals[frame.Scope()]; ok {
			for _, proposal := range proposals {
				if buffer == nil || !buffer.Append(proposal) {
					return outcome.Result{}
				}
			}
			return outcome.Result{Code: outcome.Produced}
		}
	}
	if value.evaluate == nil {
		return outcome.Result{}
	}
	return value.evaluate(frame, buffer)
}

func (value *worker) Evaluations() int {
	if value == nil {
		return 0
	}
	return value.evaluations
}

type operationBinding struct {
	operation signature.Signature
	worker    *worker
}

func (value operationBinding) Signature() signature.Signature { return value.operation }

func (value operationBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return value.worker, value.worker != nil
}

type factory struct {
	bindings map[signature.Identity]binding.Binding
}

func (value factory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() {
		return nil, false
	}
	result, ok := value.bindings[operation.Identity()]
	return result, ok && result != nil
}

type opaqueAlgebra struct{ typeID model.TypeID }

func (value opaqueAlgebra) Type() model.TypeID { return value.typeID }

func (value opaqueAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return binding.ValueToken{}, false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	if bytes.Compare(leftOpaque[:], rightOpaque[:]) >= 0 {
		return left, true
	}
	return right, true
}

func (value opaqueAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return value.Join(left, right)
}

func (value opaqueAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	return bytes.Compare(leftOpaque[:], rightOpaque[:]) <= 0
}

type algebraRegistry struct{ value opaqueAlgebra }

func (value algebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return value.value, value.value.Type() == typeID
}
