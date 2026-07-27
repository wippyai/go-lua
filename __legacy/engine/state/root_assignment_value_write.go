package state

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// RootAssignmentValueWrite is one ProductDomain-sealed exact Values-slot
// replacement. It is the shared N4 value law for concrete State and guarded
// ValueLaneFactor adapters.
type RootAssignmentValueWrite struct {
	seal  *productDomainSeal
	slot  statekey.Value
	value product.Value
}

func (d ProductDomain) SealRootAssignmentValueWrite(slot statekey.Value, value product.Value) (RootAssignmentValueWrite, error) {
	if !d.Valid() || !d.ValuesEnabled() || slot == 0 || !product.BelongsToRegistry(d.reg, value) {
		return RootAssignmentValueWrite{}, fmt.Errorf("state: invalid root-assignment value write")
	}
	return RootAssignmentValueWrite{seal: d.seal, slot: slot, value: value}, nil
}

func (d ProductDomain) ownsRootAssignmentValueWrite(write RootAssignmentValueWrite) bool {
	return d.Valid() && write.seal == d.seal && write.slot != 0 && product.BelongsToRegistry(d.reg, write.value)
}

func (d ProductDomain) ApplyRootAssignmentValueWrite(write RootAssignmentValueWrite, current State) (State, error) {
	if !d.ownsRootAssignmentValueWrite(write) {
		return State{}, fmt.Errorf("state: foreign root-assignment value write")
	}
	return d.Normalize(current).WriteValue(d.reg, write.slot, write.value), nil
}

// ApplyRootAssignmentValueScalar replaces one exact transposed slot. Values
// Top absorbs every write; otherwise assignment is replacement, so current is
// intentionally not an input and no map is allocated.
func (d ProductDomain) ApplyRootAssignmentValueScalar(write RootAssignmentValueWrite, valuesTop bool) (product.Value, error) {
	if !d.ownsRootAssignmentValueWrite(write) {
		return product.Value{}, fmt.Errorf("state: foreign root-assignment value write")
	}
	if valuesTop {
		return product.Top(), nil
	}
	return write.value, nil
}
