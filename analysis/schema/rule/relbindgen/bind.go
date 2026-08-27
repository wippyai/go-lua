package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Spec is the declaration a generated binding hands this substrate.
type Spec[A, R any] struct {
	// Signature is the sealed operation contract this binding answers for.
	Signature signature.Signature
	// Decoder, Encoder and Operation are the family's thin typed artifacts.
	Decoder   Decoder[A]
	Encoder   Encoder[R]
	Operation Operation[A, R]
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
	outputs := spec.Signature.Outputs()
	if len(outputs) == 0 {
		return nil, false
	}
	for _, declared := range outputs {
		if !declared.Available() {
			return nil, false
		}
	}
	cardinality := spec.Signature.Cardinality()
	if !cardinality.Available() {
		return nil, false
	}
	if cardinality.Kind() == model.CompleteDenominator {
		if !completeOutputs(outputs) {
			return nil, false
		}
		// CompleteDenominator has no static numeric limit. The mounted
		// ProposalBuffer supplies the exact witness-backed capacity and rejects
		// any row outside that witness.
		return factory[A, R]{spec: spec, outputs: outputs, cardinality: cardinality.Kind()}, true
	}
	limit, ok := rowLimit(cardinality)
	if !ok || limit == 0 {
		return nil, false
	}
	return factory[A, R]{spec: spec, outputs: outputs, cardinality: cardinality.Kind(), limit: int(limit)}, true
}

func completeOutputs(outputs []signature.Output) bool {
	if len(outputs) == 0 || !outputs[0].Available() || !outputs[0].Denominator.Available() {
		return false
	}
	denominator := outputs[0].Denominator
	type outputKey struct {
		relation model.RelationID
		column   model.ColumnID
	}
	seen := make(map[outputKey]struct{}, len(outputs))
	for _, output := range outputs {
		if !output.Available() || output.Denominator != denominator || output.Relation != denominator.Relation() {
			return false
		}
		key := outputKey{relation: output.Relation, column: output.Column}
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
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
	spec        Spec[A, R]
	outputs     []signature.Output
	cardinality model.CardinalityKind
	limit       int
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
	return &worker[A, R]{
		factory: value.factory,
		fence:   fence,
		issuer:  issuer,
		emitter: Emitter[R]{rows: make([]emission[R], 0, value.factory.limit), limit: value.factory.limit, unbounded: value.factory.cardinality == model.CompleteDenominator},
	}, true
}

// worker is the solve-local adapter. Its scratch is allocated once and reused
// across invocations; nothing it holds can name a relation outside the frame.
type worker[A, R any] struct {
	factory factory[A, R]
	fence   binding.Fence
	issuer  binding.Issuer
	emitter Emitter[R]
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
	value.emitter.reset(buffer.Destination())
	argument, ok := value.factory.spec.Decoder.Decode(inputs)
	if !ok {
		return value.refuse(nil)
	}
	code := value.factory.spec.Operation.Evaluate(argument, &value.emitter)
	if value.emitter.overflow || !operation.Allows(code) {
		return value.refuse(buffer)
	}
	if !code.Publishes() {
		if value.emitter.Len() != 0 {
			return value.refuse(nil)
		}
		return value.settle(code)
	}
	if !value.factory.validEmissionCount(value.emitter.Len()) {
		return value.refuse(buffer)
	}
	for index := range value.emitter.rows {
		staged := value.emitter.rows[index]
		outputs := Outputs{
			declared: value.factory.outputs,
			buffer:   buffer,
			issuer:   value.issuer,
			row:      staged.row,
			presence: staged.presence,
		}
		if !value.factory.spec.Encoder.Encode(outputs, staged.value) {
			return value.refuse(buffer)
		}
	}
	return value.settle(code)
}

func (value factory[A, R]) validEmissionCount(count int) bool {
	switch value.cardinality {
	case model.ExactlyOne:
		return count == 1
	case model.Optional, model.BoundedMany:
		return count <= value.limit
	case model.CompleteDenominator:
		// The mounted destination witness, not the schema, determines the
		// complete row count. ProposalBuffer.Seal proves exact coverage.
		return true
	default:
		return false
	}
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
