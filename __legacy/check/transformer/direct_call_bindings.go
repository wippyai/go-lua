package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DirectCallBindings is one exact fixed-arity lexical application. Values and
// Paths use the callee Shape's packed namespace order. Parameter, capture, and
// global roots are namespace-distinct even when their dense indices coincide.
type DirectCallBindings struct {
	Values []ValueTerm
	Paths  []PathTerm
}

type directCallTarget struct {
	symbol symbol.ID
	slot   int
	kind   factflow.CallResultTargetKind
}

func exactDirectCallTargets(site factflow.CallSiteView) ([]directCallTarget, error) {
	out := make([]directCallTarget, 0, site.ResultTargetCount())
	seenSymbols := make(map[symbol.ID]struct{}, site.ResultTargetCount())
	for index := 0; index < site.ResultTargetCount(); index++ {
		target, _ := site.ResultTargetAt(index)
		if target.ResultIndex() < 0 {
			return nil, fmt.Errorf("transformer: direct-call result target %d is not an exact local root", index)
		}
		var targetSymbol symbol.ID
		switch target.Kind() {
		case factflow.CallResultTargetLocalAssignment:
			targetSymbol = target.TargetSymbol()
			if targetSymbol == 0 || target.TargetPathEmpty() || target.TargetPathSegmentCount() != 0 || target.TargetPathRef().Symbol != targetSymbol {
				return nil, fmt.Errorf("transformer: direct-call result target %d is not an exact local root", index)
			}
		case factflow.CallResultTargetOrdinaryAssignment:
			if target.TargetPathEmpty() {
				// A dynamic-index assignment has no single static destination path.
				// Its following N4 path-store transaction consumes the call's
				// canonical frame-result carrier, so no second result projection is
				// required here.
				if target.TargetSymbol() != 0 || target.Index() < 0 {
					return nil, fmt.Errorf("transformer: direct-call dynamic result target %d has malformed destination metadata", index)
				}
				break
			}
			if target.TargetPathRef().Symbol == 0 || target.TargetSymbol() != target.TargetPathRef().Symbol {
				return nil, fmt.Errorf("transformer: direct-call result target %d has no exact assignment path", index)
			}
		case factflow.CallResultTargetReturn:
			if site.Context() != factflow.CallSiteContextReturnSource || target.TargetSymbol() != 0 || !target.TargetPathEmpty() || target.Index() < 0 {
				return nil, fmt.Errorf("transformer: direct-call result target %d is not an exact return slot", index)
			}
		case factflow.CallResultTargetExpression:
			if target.TargetSymbol() != 0 || !target.TargetPathEmpty() || target.Index() < 0 {
				return nil, fmt.Errorf("transformer: direct-call result target %d is not an exact expression slot", index)
			}
		default:
			return nil, fmt.Errorf("transformer: direct-call result target %d has unsupported kind %d", index, target.Kind())
		}
		if targetSymbol != 0 {
			if _, duplicate := seenSymbols[targetSymbol]; duplicate {
				return nil, fmt.Errorf("transformer: direct-call duplicate result symbol %d", targetSymbol)
			}
			seenSymbols[targetSymbol] = struct{}{}
		}
		out = append(out, directCallTarget{symbol: targetSymbol, slot: target.ResultIndex(), kind: target.Kind()})
	}
	return out, nil
}
