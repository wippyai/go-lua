package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// KeyedDestination declares that the operation names each destination row by
// the owner's own content identity rather than reading it from an input slot.
// It is the address form a finite expansion uses.
const KeyedDestination = -1

// Spec is the declaration a generated binding hands this substrate.
type Spec[A, R any] struct {
	// Signature is the sealed operation contract this binding answers for.
	Signature signature.Signature
	// Decoder, Encoder and Operation are the family's thin typed artifacts.
	Decoder   Decoder[A]
	Encoder   Encoder[R]
	Operation Operation[A, R]
	// Address names the scalar input slot whose row addresses every proposal,
	// or KeyedDestination for an owner-named expansion.
	Address int
	// Refusal is the owner-issued reason this binding refuses with.
	Refusal model.RefusalID
}

// Bind admits one generated family binding. The returned factory accepts only
// the exact sealed signature it was constructed with, so binding.Admit refuses
// a foreign or drifted contract without any runtime form inspection.
func Bind[A, R any](spec Spec[A, R]) (binding.Factory, bool) {
	if !spec.Signature.Available() || spec.Decoder == nil || spec.Encoder == nil || spec.Operation == nil || !spec.Refusal.Available() {
		return nil, false
	}
	authority := spec.Signature.Authority()
	if !authority.Available() {
		return nil, false
	}
	outputs := spec.Signature.Outputs()
	if len(outputs) == 0 {
		return nil, false
	}
	for _, declared := range outputs {
		if !declared.Available() || declared.Relation != authority.Denominator.Relation() {
			return nil, false
		}
	}
	limit, ok := rowLimit(spec.Signature.Cardinality())
	if !ok || limit == 0 {
		return nil, false
	}
	if spec.Address != KeyedDestination {
		input, inputOK := spec.Signature.InputAt(spec.Address)
		if !inputOK || !input.Delivery.IsScalar() {
			return nil, false
		}
		if limit != 1 {
			return nil, false
		}
	}
	return factory[A, R]{spec: spec, outputs: outputs, limit: int(limit)}, true
}

func rowLimit(cardinality model.Cardinality) (uint32, bool) {
	if !cardinality.Available() {
		return 0, false
	}
	switch cardinality.Kind() {
	case model.ExactlyOne, model.Optional:
		return 1, true
	case model.BoundedMany:
		return cardinality.Bound()
	default:
		return 0, false
	}
}

type factory[A, R any] struct {
	spec    Spec[A, R]
	outputs []signature.Output
	limit   int
}

func (value factory[A, R]) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != value.spec.Signature.Digest() {
		return nil, false
	}
	return bound[A, R]{factory: value}, true
}

type bound[A, R any] struct {
	factory factory[A, R]
}

func (value bound[A, R]) Signature() signature.Signature { return value.factory.spec.Signature }

func (value bound[A, R]) NewWorker(fence binding.Fence) (binding.Worker, bool) {
	if !fence.Available() || fence.Schema() != value.factory.spec.Signature.Fence().Schema {
		return nil, false
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		return nil, false
	}
	operation := value.factory.spec.Operation
	if local, ok := operation.(SolveLocal[A, R]); ok {
		operation = local.NewOperation()
		if operation == nil {
			return nil, false
		}
	}
	return &worker[A, R]{
		factory:   value.factory,
		operation: operation,
		fence:     fence,
		issuer:    issuer,
		emitter:   Emitter[R]{rows: make([]emission[R], 0, value.factory.limit), limit: value.factory.limit},
	}, true
}

// worker is the solve-local adapter. Its scratch is allocated once and reused
// across invocations; nothing it holds can name a relation outside the frame.
type worker[A, R any] struct {
	factory factory[A, R]
	// operation is this worker's own when the family said it carries
	// per-invocation storage, and the shared one otherwise.
	operation Operation[A, R]
	fence     binding.Fence
	issuer    binding.Issuer
	emitter   Emitter[R]
}

func (value *worker[A, R]) Evaluate(frame binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	operation := value.factory.spec.Signature
	if buffer == nil || !buffer.Available() || buffer.Signature().Digest() != operation.Digest() {
		return value.refuse(nil)
	}
	if !frame.Validate(operation, value.fence) {
		return value.refuse(nil)
	}
	inputs := Inputs{frame: frame}
	var fallback identity.ContentID
	keyed := value.factory.spec.Address == KeyedDestination
	if !keyed {
		key, ok := inputs.RowKeyAt(value.factory.spec.Address)
		if !ok {
			return value.refuse(nil)
		}
		fallback = key
	}
	value.emitter.reset(fallback, keyed)
	argument, ok := value.factory.spec.Decoder.Decode(inputs)
	if !ok {
		return value.refuse(nil)
	}
	code := value.operation.Evaluate(argument, &value.emitter)
	if value.emitter.overflow || !operation.Allows(code) {
		return value.refuse(nil)
	}
	if code != outcome.Produced {
		if value.emitter.Len() != 0 {
			return value.refuse(nil)
		}
		return value.settle(code)
	}
	for index := range value.emitter.rows {
		staged := value.emitter.rows[index]
		outputs := Outputs{
			declared: value.factory.outputs,
			buffer:   buffer,
			issuer:   value.issuer,
			rowKey:   staged.key,
			presence: staged.presence,
		}
		if !value.factory.spec.Encoder.Encode(outputs, staged.value) {
			return value.refuse(buffer)
		}
	}
	return value.settle(outcome.Produced)
}

// settle returns the operation's own closed disposition. The binding never
// seals the batch: publication settlement belongs to the engine.
func (value *worker[A, R]) settle(code outcome.Code) outcome.Result {
	if code == outcome.Refused {
		return value.refuse(nil)
	}
	result, ok := outcome.NewResult(code, model.RefusalID{})
	if !ok {
		return outcome.Result{}
	}
	return result
}

// refuse reports the owner's refusal reason. A buffer that already carries
// staged rows is abandoned so no partial publication can survive.
func (value *worker[A, R]) refuse(buffer *binding.ProposalBuffer) outcome.Result {
	if buffer != nil {
		buffer.Abandon()
	}
	result, ok := outcome.NewResult(outcome.Refused, value.factory.spec.Refusal)
	if !ok {
		return outcome.Result{}
	}
	return result
}
