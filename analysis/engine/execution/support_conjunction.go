package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/operand"
)

// ConjoinSupport is the ONE statement of the support a conclusion holds over
// when it consumed more than one observation.
//
// A conclusion may only hold where every read it consumed holds, so the
// support is the conjunction of what those reads proved. The conjunction is
// taken by ENTAILMENT rather than by equality, because the reads of one
// invocation do not all prove their answers over the same window: a member
// read at a coordinate this very rule writes is absent before the fixpoint
// puts anything there, and absent EVERYWHERE, which is a wider support than
// the prerequisite that reached it.
//
// So a support that CONTAINS the running meet leaves it alone, and one that
// proved less than everything before it moves the meet down to itself. Two
// supports neither of which contains the other are refused: that meet is one
// this cannot compute, and publishing over whichever arrived first would claim
// a conclusion holds where one of its reads was never proved.
//
// Both halves of a fold ask this - the product of exact cells and the vector
// spanned beside it - so the two cannot disagree about what one invocation
// concluded over.
func ConjoinSupport(running, next support.Mask) (support.Mask, bool) {
	if !running.Valid() || !next.Valid() {
		return support.Mask{}, false
	}
	switch {
	case running.Entails(next):
		return running, true
	case next.Entails(running):
		return next, true
	default:
		return support.Mask{}, false
	}
}

// ConjoinCells folds ConjoinSupport over the cells of one delivered vector, in
// the order the span published them. The starting mask is the support the rest
// of the invocation proved, so a vector cell can only narrow the conclusion,
// never widen it past what its siblings held.
func ConjoinCells[V any](running support.Mask, cells []operand.MemberCell[V]) (support.Mask, bool) {
	region := running
	for _, cell := range cells {
		conjoined, ok := ConjoinSupport(region, cell.Region)
		if !ok {
			return support.Mask{}, false
		}
		region = conjoined
	}
	return region, true
}
