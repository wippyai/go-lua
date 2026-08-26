package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/activation/branch"
	"github.com/wippyai/go-lua/domain/call/dispatch"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// CallDispatchOperation is domain/call/dispatch's own fold. It holds the
// sealed judgment and nothing else.
type CallDispatchOperation struct {
	judgment dispatch.Judgment
}

// NewCallDispatchOperation derives the dispatch judgment from the three sealed
// algebras it reads through.
func NewCallDispatchOperation(calls *calldomain.Algebra, values *valuedomain.Schema, heaps heapdomain.Schema) (CallDispatchOperation, bool) {
	judgment, ok := dispatch.Derive(calls, values, heaps)
	if !ok {
		return CallDispatchOperation{}, false
	}
	return CallDispatchOperation{judgment: judgment}, true
}

// Available reports whether the operation carries a derived judgment.
func (operation CallDispatchOperation) Available() bool { return operation.judgment.Valid() }

// Evaluate answers one mounted call against the callee it observed.
func (operation CallDispatchOperation) Evaluate(argument CallDispatchArgument, emitter *relbindgen.Emitter[calldomain.Value]) outcome.Code {
	fact, reduction := operation.judgment.Dispatch(argument.Candidate, argument.Callee)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// CallActivationOperation is domain/call/activation's own branch selector: it
// settles whether one mounted call's activation branch holds and publishes no
// fact of its own.
//
// The branch ordinal the selector settles is the engine's enumeration, not a
// read this family declares, so the binding answers for the branch the frame
// was opened on and the enumeration stays where it is.
type CallActivationOperation struct {
	selector branch.Selector
	derived  bool
}

// NewCallActivationOperation derives the activation selector from the sealed
// call algebra.
func NewCallActivationOperation(calls *calldomain.Algebra) (CallActivationOperation, bool) {
	selector, ok := branch.Derive(calls)
	if !ok {
		return CallActivationOperation{}, false
	}
	return CallActivationOperation{selector: selector, derived: true}, true
}

// Available reports whether the operation carries a derived selector.
func (operation CallActivationOperation) Available() bool { return operation.derived }

// Evaluate settles one activation and publishes nothing. An emitter opened at
// a capacity of none is what makes that structural rather than intended.
func (operation CallActivationOperation) Evaluate(argument CallActivationArgument, emitter *relbindgen.Emitter[struct{}]) outcome.Code {
	if emitter.Cap() != 0 {
		return outcome.Refused
	}
	return relbindgen.Answer(operation.selector.Settle(argument.Candidate, 0, argument.Trigger))
}
