package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) evidenceCallSiteResultTargets(targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, len(targets))
	for i := range targets {
		out[i] = lowerCallResultTarget(targets[i])
	}
	return out
}

func (l *lowerer) callSiteResultTargetsFromWIR(point cfg.Point, targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if l != nil && l.wir != nil {
		return lowerWIRCallResultTargets(l.wir.CallResultTargets(point))
	}
	return l.evidenceCallSiteResultTargets(targets)
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

func lowerCallResultTarget(target semantics.CallResultTarget) factflow.CallResultTarget {
	targetKind := factflow.CallResultTargetUnknown
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		targetKind = factflow.CallResultTargetLocalAssignment
	case semantics.CallResultTargetOrdinaryAssignment:
		targetKind = factflow.CallResultTargetOrdinaryAssignment
	case semantics.CallResultTargetReturn:
		return factflow.NewCallResultTarget(factflow.CallResultTargetReturn, target.Index, target.ResultIndex, 0, path.Path{})
	case semantics.CallResultTargetExpression:
		return factflow.NewCallResultTarget(factflow.CallResultTargetExpression, target.Index, target.ResultIndex, 0, path.Path{})
	}
	targetSymbol := symbol.ID(0)
	if target.HasSymbol {
		targetSymbol = target.Symbol
	}
	targetPath := path.Path{}
	if target.HasPath {
		targetPath = target.Path
	} else if target.Kind == semantics.CallResultTargetLocalAssignment && target.HasSymbol {
		targetPath = path.NewPath(target.Symbol, target.Name)
	} else if target.Kind == semantics.CallResultTargetOrdinaryAssignment && target.HasSymbol {
		targetPath = path.NewPath(target.Symbol, "")
	}
	return factflow.NewCallResultTarget(targetKind, target.Index, target.ResultIndex, targetSymbol, targetPath)
}
