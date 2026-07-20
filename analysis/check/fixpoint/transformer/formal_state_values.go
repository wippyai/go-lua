package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalStateValues projects the Values factor across the symbolic-to-State
// boundary. Input roots are quantified binders: the concrete EntryState
// already owns their boundary values, while the corresponding Middle roots
// own the current point values. Results are concrete State output slots. Heap
// templates are allocation existentials handled by the root quotient and may
// never escape as a live Values coordinate.
func formalStateValues(values state.ValueFactor[FormalSlot]) (state.ValueFactor[FormalSlot], error) {
	if values.Top {
		return values, nil
	}
	out := state.ValueFactor[FormalSlot]{Values: make(map[FormalSlot]product.Value, len(values.Values))}
	for slot, value := range values.Values {
		root, exact := slot.relationRoot()
		if !exact {
			return state.ValueFactor[FormalSlot]{}, fmt.Errorf("transformer: formal Values source has no structural root")
		}
		switch root.Kind {
		case RootParam, RootCapture, RootGlobal, RootAmbient:
			// Symbolic input binder: EntryState is its concrete publication.
		case RootMiddle, RootResult:
			out.Values[slot] = value
		case RootHeapTemplate:
			return state.ValueFactor[FormalSlot]{}, fmt.Errorf("transformer: live heap-template Values source cannot publish to State")
		default:
			return state.ValueFactor[FormalSlot]{}, fmt.Errorf("transformer: formal Values source has invalid root kind")
		}
	}
	return out, nil
}
