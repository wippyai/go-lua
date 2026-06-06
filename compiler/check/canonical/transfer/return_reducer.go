package transfer

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

type returnReducer struct{}

var returns = returnReducer{}

// Return facts are the projection boundary for a normal return point: finite
// return relations plus scratch values for non-identifier return expressions.
func (returnReducer) setRelations(out *flow.PointState, rels flow.ReturnRelations) bool {
	if out == nil || flow.ReturnRelationsDomain.Equal(out.ReturnRel, rels) {
		return false
	}
	out.ReturnRel = rels
	return true
}

func (returnReducer) clearSlot(out *flow.PointState, index int) bool {
	if out == nil || index < 0 {
		return false
	}
	return flow.NewPointWriter(out).DeleteValueKey(ReturnSlotKey(index))
}

func (returnReducer) writeSlot(out *flow.PointState, index int, value product.AbstractValue) bool {
	if out == nil || index < 0 || value.IsZero() {
		return false
	}
	return flow.NewPointWriter(out).WriteValueKey(ReturnSlotKey(index), value, false)
}
