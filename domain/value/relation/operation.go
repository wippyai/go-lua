package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/allocation"
	"github.com/wippyai/go-lua/domain/value/freshresult"
	"github.com/wippyai/go-lua/domain/value/moduleload"
	"github.com/wippyai/go-lua/domain/value/runtimekind"
)

// ValueArithmeticOperation is domain/value's own binary arithmetic reduction.
type ValueArithmeticOperation struct{}

// Available reports whether the operation carries its owner mathematics. The
// reduction is a package function over values that carry their own owner, so
// there is no derived state to hold and nothing to be unavailable.
func (ValueArithmeticOperation) Available() bool { return true }

// Evaluate answers one binary arithmetic occurrence.
func (ValueArithmeticOperation) Evaluate(argument ValueArithmeticArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := valuedomain.ArithmeticValue(argument.Candidate, argument.Left, argument.Right)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueEqualityOperation is domain/value's own binary equality reduction.
type ValueEqualityOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (ValueEqualityOperation) Available() bool { return true }

// Evaluate answers one binary equality occurrence.
func (ValueEqualityOperation) Evaluate(argument ValueEqualityArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := valuedomain.EqualityValue(argument.Candidate, argument.Left, argument.Right)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueOrderOperation is domain/value's own binary order reduction.
type ValueOrderOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (ValueOrderOperation) Available() bool { return true }

// Evaluate answers one binary order occurrence.
func (ValueOrderOperation) Evaluate(argument ValueOrderArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := valuedomain.OrderValue(argument.Candidate, argument.Left, argument.Right)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueRefinementOperation is domain/value's own presence refinement.
type ValueRefinementOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (ValueRefinementOperation) Available() bool { return true }

// Evaluate answers one presence refinement occurrence.
func (ValueRefinementOperation) Evaluate(argument ValueRefinementArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := valuedomain.PresenceRefinementValue(argument.Candidate, argument.Fact)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueTransferOperation is domain/value's own storage transfer, which carries
// a fact from the coordinate it was observed at to the one it is stored at.
type ValueTransferOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (ValueTransferOperation) Available() bool { return true }

// Evaluate answers one storage transfer.
func (ValueTransferOperation) Evaluate(argument ValueTransferArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := valuedomain.IdentityValue(argument.Source)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueSourceOperation is domain/value's own source seed.
type ValueSourceOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (ValueSourceOperation) Available() bool { return true }

// Evaluate answers one seeded source coordinate.
func (ValueSourceOperation) Evaluate(argument ValueSourceArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := valuedomain.SourceFact(argument.Seed)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueBootstrapOperation is domain/value's own global bootstrap seed.
type ValueBootstrapOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (ValueBootstrapOperation) Available() bool { return true }

// Evaluate answers one sealed global binding receipt.
func (ValueBootstrapOperation) Evaluate(argument ValueBootstrapArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := valuedomain.GlobalBootstrapFact(argument.Result)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueAllocationOperation is domain/value/allocation's own judgment. It holds
// the sealed judgment and nothing else.
type ValueAllocationOperation struct {
	judgment allocation.Judgment
}

// NewValueAllocationOperation derives the allocation judgment from one sealed
// correlated-value schema.
func NewValueAllocationOperation(values *valuedomain.Schema) (ValueAllocationOperation, bool) {
	judgment, ok := allocation.Derive(values)
	if !ok {
		return ValueAllocationOperation{}, false
	}
	return ValueAllocationOperation{judgment: judgment}, true
}

// Available reports whether the operation carries a derived judgment.
func (operation ValueAllocationOperation) Available() bool { return operation.judgment.Valid() }

// Evaluate answers one allocation result receipt.
func (operation ValueAllocationOperation) Evaluate(argument ValueAllocationArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := operation.judgment.Result(argument.Candidate)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueAllocationAgeOperation is domain/value's own allocation carry: the
// transform the allocation rule ages every carried coordinate through.
type ValueAllocationAgeOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (ValueAllocationAgeOperation) Available() bool { return true }

// Evaluate ages one carried coordinate through its allocation receipt.
func (ValueAllocationAgeOperation) Evaluate(argument ValueAllocationAgeArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	if argument.Candidate == nil {
		return outcome.Refused
	}
	fact, held := argument.Candidate.Age(argument.Prior)
	return relbindgen.Carried(emitter, fact, held)
}

// ValueRuntimeKindOperation is domain/value/runtimekind's own judgment.
type ValueRuntimeKindOperation struct {
	judgment runtimekind.Judgment
}

// NewValueRuntimeKindOperation derives the runtime-kind judgment from one
// sealed correlated-value schema.
func NewValueRuntimeKindOperation(values *valuedomain.Schema) (ValueRuntimeKindOperation, bool) {
	judgment, ok := runtimekind.Derive(values)
	if !ok {
		return ValueRuntimeKindOperation{}, false
	}
	return ValueRuntimeKindOperation{judgment: judgment}, true
}

// Available reports whether the operation carries a derived judgment.
func (operation ValueRuntimeKindOperation) Available() bool { return operation.judgment.Valid() }

// Evaluate answers one runtime-kind comparison call.
func (operation ValueRuntimeKindOperation) Evaluate(argument ValueRuntimeKindArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := operation.judgment.Result(argument.Candidate, argument.Dispatched, argument.Subject, argument.Comparison)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueModuleLoadOperation is domain/value/moduleload's own judgment.
type ValueModuleLoadOperation struct {
	judgment moduleload.Judgment
}

// NewValueModuleLoadOperation derives the module-load judgment from one sealed
// correlated-value schema.
func NewValueModuleLoadOperation(values *valuedomain.Schema) (ValueModuleLoadOperation, bool) {
	judgment, ok := moduleload.Derive(values)
	if !ok {
		return ValueModuleLoadOperation{}, false
	}
	return ValueModuleLoadOperation{judgment: judgment}, true
}

// Available reports whether the operation carries a derived judgment.
func (operation ValueModuleLoadOperation) Available() bool { return operation.judgment.Valid() }

// Evaluate answers one candidate module load.
func (operation ValueModuleLoadOperation) Evaluate(argument ValueModuleLoadArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := operation.judgment.Result(argument.Candidate, argument.Argument, argument.Dispatched)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// ValueSummaryOperation is domain/value's own coordinatewise fold. The engine
// chose the group and delivered its complete span; the operation only folds,
// and an absent coordinate stays absent rather than becoming a stored default.
type ValueSummaryOperation struct {
	schema *valuedomain.Schema
}

// NewValueSummaryOperation adopts one sealed correlated-value schema.
func NewValueSummaryOperation(values *valuedomain.Schema) (ValueSummaryOperation, bool) {
	if values == nil || !values.Valid() {
		return ValueSummaryOperation{}, false
	}
	return ValueSummaryOperation{schema: values}, true
}

// Available reports whether the operation carries a sealed schema.
func (operation ValueSummaryOperation) Available() bool {
	return operation.schema != nil && operation.schema.Valid()
}

// Evaluate folds the delivered group into one summary observation.
func (operation ValueSummaryOperation) Evaluate(argument ValueSummaryArgument, emitter *relbindgen.Emitter[valuedomain.ValueSummaryObservation]) outcome.Code {
	seed := valuedomain.BeginValueSummary(operation.schema)
	folded, ok := valuedomain.AccumulateValueSummaryRows(operation.schema, seed, argument.Cells.Len(), argument.Cells.At)
	if !ok {
		return outcome.Refused
	}
	if folded.Rows == 0 {
		return outcome.NoSelection
	}
	if !emitter.Put(folded) {
		return outcome.Refused
	}
	return outcome.Produced
}

// ValueFreshResultOperation is domain/value/freshresult's own judgment: the
// fresh value a call result route publishes at the coordinate it names.
type ValueFreshResultOperation struct {
	judgment freshresult.Judgment
	derived  bool
}

// NewValueFreshResultOperation derives the fresh-result judgment from the two
// sealed algebras it reads through.
func NewValueFreshResultOperation(values *valuedomain.Schema, calls *calldomain.Algebra) (ValueFreshResultOperation, bool) {
	judgment, ok := freshresult.NewJudgment(values, calls)
	if !ok {
		return ValueFreshResultOperation{}, false
	}
	return ValueFreshResultOperation{judgment: judgment, derived: true}, true
}

// Available reports whether the operation carries a derived judgment. The
// owner seals its judgment at derivation and publishes no second reading of
// that seal, so the operation records the one its constructor obtained.
func (operation ValueFreshResultOperation) Available() bool { return operation.derived }

// Evaluate answers one fresh call result at the route it was selected on.
func (operation ValueFreshResultOperation) Evaluate(argument ValueFreshResultArgument, emitter *relbindgen.Emitter[valuedomain.Value]) outcome.Code {
	fact, reduction := operation.judgment.FreshResultFact(argument.Candidate, argument.CallFact, argument.Destination, argument.Tag, argument.Prior)
	return relbindgen.Reduce(emitter, fact, reduction)
}
