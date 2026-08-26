package relation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	calldomain "github.com/wippyai/go-lua/domain/call"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	typestatedomain "github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/domain/typestate/obligation"
	"github.com/wippyai/go-lua/domain/typestate/statecell"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// SealStateCellSpace seals the state-cell space one link's typestate columns
// are addressed over.
//
// The space is the axis's own denominator: it is issued against the link, it
// owns every cell it hands out, and it normalizes a cell to the dense position
// its own seal assigned. The mount reaches it here so the columns of one link
// are addressed over exactly one space and never over a second construction of
// the same one.
func SealStateCellSpace(link identity.ContentID, allocations, protocols int) (statecell.Space, bool) {
	return statecell.Seal(link, allocations, protocols)
}

// TypestateObligationOperation is domain/typestate/obligation's own judgment:
// the successor state of the resource cell one call actual names.
//
// A callee the analysis cannot follow is judged rather than dropped. The call
// fact reaches the judgment as authenticated opaque evidence, the declared
// escape discharges every proof about the resource, and the answer is
// published opaque. That is why this binding publishes a row for such a call
// rather than refusing one: a refusal removes the row from the population, and
// a missing row reads as a call with nothing to say about it, which is the one
// answer a soundness judgment may not give.
type TypestateObligationOperation struct {
	judgment obligation.Judgment
}

// NewTypestateObligationOperation derives the obligation judgment from the
// three sealed algebras it reads through.
func NewTypestateObligationOperation(values *valuedomain.Schema, calls *calldomain.Algebra, packs *packdomain.Schema) (TypestateObligationOperation, bool) {
	judgment, ok := obligation.Derive(values, calls, packs)
	if !ok {
		return TypestateObligationOperation{}, false
	}
	return TypestateObligationOperation{judgment: judgment}, true
}

// Available reports whether the operation carries a derived judgment.
func (operation TypestateObligationOperation) Available() bool { return operation.judgment.Valid() }

// Evaluate answers one call actual's obligation.
func (operation TypestateObligationOperation) Evaluate(argument TypestateObligationArgument, emitter *relbindgen.Emitter[typestatedomain.Abstract]) outcome.Code {
	fact, reduction := operation.judgment.Judge(argument.Candidate, argument.Argument, argument.Dispatched, argument.Tag, argument.Current)
	return relbindgen.Reduce(emitter, fact, reduction)
}
