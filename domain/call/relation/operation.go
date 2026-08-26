package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	calldomain "github.com/wippyai/go-lua/domain/call"
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
