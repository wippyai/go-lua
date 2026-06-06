package flow

import "github.com/wippyai/go-lua/types/domain/value/product"

// SetReturnRelations installs the finite relation facts visible at a normal
// return point.
func SetReturnRelations(out *PointState, rels ReturnRelations) bool {
	if out == nil || ReturnRelationsDomain.Equal(out.ReturnRel, rels) {
		return false
	}
	out.ReturnRel = rels
	return true
}

// ClearReturnSlotValue removes the scratch value for one non-symbol return
// expression. Transfer decides which expressions need slots; flow owns the
// canonical storage key and mutation.
func ClearReturnSlotValue(out *PointState, index int) bool {
	if out == nil || index < 0 {
		return false
	}
	return NewPointWriter(out).DeleteValueKey(ReturnSlotValueKey(index))
}

// WriteReturnSlotValue records the scratch value for one non-symbol return
// expression so summary projection can read it through PointFacts.
func WriteReturnSlotValue(out *PointState, index int, value product.AbstractValue) bool {
	if out == nil || index < 0 || value.IsZero() {
		return false
	}
	return NewPointWriter(out).WriteValueKey(ReturnSlotValueKey(index), value, false)
}
