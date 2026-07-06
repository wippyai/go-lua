package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) callSiteResultTargetsFromWIR(point cfg.Point) []factflow.CallResultTarget {
	if l != nil && l.wir != nil {
		return lowerWIRCallResultTargets(l.wir.CallResultTargets(point))
	}
	return nil
}

func lowerWIRCallResultTargets(targets []wir.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, len(targets))
	for i, target := range targets {
		out[i] = lowerWIRCallResultTarget(target)
	}
	return out
}

func lowerWIRCallResultTarget(target wir.CallResultTarget) factflow.CallResultTarget {
	targetKind := factflow.CallResultTargetUnknown
	switch target.Kind {
	case wir.CallResultTargetLocalAssignment:
		targetKind = factflow.CallResultTargetLocalAssignment
	case wir.CallResultTargetOrdinaryAssignment:
		targetKind = factflow.CallResultTargetOrdinaryAssignment
	case wir.CallResultTargetReturn:
		targetKind = factflow.CallResultTargetReturn
	case wir.CallResultTargetExpression:
		targetKind = factflow.CallResultTargetExpression
	}
	targetIndex := target.Index
	resultIndex := target.ResultIndex
	targetSymbol := symbol.ID(0)
	if !target.Path.IsEmpty() {
		targetSymbol = target.Path.Symbol
	}
	return factflow.NewCallResultTarget(targetKind, targetIndex, resultIndex, targetSymbol, target.Path)
}
