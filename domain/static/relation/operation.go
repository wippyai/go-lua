package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	staticdomain "github.com/wippyai/go-lua/domain/static"
)

// StaticTransferOperation is domain/static's own type-fact transfer, which
// carries a fact from the coordinate it was observed at to the one it is
// stored at.
type StaticTransferOperation struct{}

// Available reports whether the operation carries its owner mathematics.
func (StaticTransferOperation) Available() bool { return true }

// Evaluate answers one static type-fact transfer.
func (StaticTransferOperation) Evaluate(argument StaticTransferArgument, emitter *relbindgen.Emitter[staticdomain.TypeFact]) outcome.Code {
	fact, reduction := staticdomain.IdentityTypeFact(argument.Source)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// StaticTypeSummaryOperation is domain/static's own coordinatewise fold. The
// engine chose the group and delivered its complete span; the operation only
// folds, and an absent coordinate stays absent rather than becoming a stored
// default.
//
// The family is one the analyzer already publishes under its own result key
// and that stood in neither census plane, so nothing said which plan produced
// it. Declaring it here is what makes it answerable like any other.
type StaticTypeSummaryOperation struct {
	classes *staticdomain.ClassSet
}

// NewStaticTypeSummaryOperation adopts one sealed class set.
func NewStaticTypeSummaryOperation(classes *staticdomain.ClassSet) (StaticTypeSummaryOperation, bool) {
	if classes == nil {
		return StaticTypeSummaryOperation{}, false
	}
	return StaticTypeSummaryOperation{classes: classes}, true
}

// Available reports whether the operation carries a sealed class set.
func (operation StaticTypeSummaryOperation) Available() bool { return operation.classes != nil }

// Evaluate folds the delivered group into one type summary observation.
func (operation StaticTypeSummaryOperation) Evaluate(argument StaticTypeSummaryArgument, emitter *relbindgen.Emitter[staticdomain.TypeSummaryObservation]) outcome.Code {
	seed := staticdomain.BeginTypeSummary(operation.classes, argument.Cells.Len())
	folded, ok := staticdomain.AccumulateTypeSummaryRows(operation.classes, seed, argument.Cells.Len(), argument.Cells.At)
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
