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
	out := l.evidenceCallSiteResultTargets(targets)
	if l == nil || l.wir == nil || len(out) == 0 {
		return out
	}
	for i, lowered := range out {
		target, ok := l.wir.CallResultTarget(point, lowered.ResultIndex())
		if !ok {
			if callResultTargetNeedsWIRPath(lowered) {
				out[i] = factflow.NewCallResultTarget(
					lowered.Kind(),
					lowered.Index(),
					lowered.ResultIndex(),
					0,
					path.Path{},
				)
			}
			continue
		}
		out[i] = lowerWIRCallResultTarget(target, lowered)
	}
	return out
}

func lowerWIRCallResultTarget(target wir.CallResultTarget, fallback factflow.CallResultTarget) factflow.CallResultTarget {
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
	default:
		targetKind = fallback.Kind()
	}
	targetIndex := target.Index
	if targetIndex < 0 {
		targetIndex = fallback.Index()
	}
	resultIndex := target.ResultIndex
	if resultIndex < 0 {
		resultIndex = fallback.ResultIndex()
	}
	targetSymbol := symbol.ID(0)
	if !target.Path.IsEmpty() {
		targetSymbol = target.Path.Symbol
	} else {
		targetSymbol = fallback.TargetSymbol()
		if callResultTargetNeedsWIRPath(fallback) {
			targetSymbol = 0
		}
	}
	return factflow.NewCallResultTarget(targetKind, targetIndex, resultIndex, targetSymbol, target.Path)
}

func callResultTargetNeedsWIRPath(target factflow.CallResultTarget) bool {
	switch target.Kind() {
	case factflow.CallResultTargetLocalAssignment, factflow.CallResultTargetOrdinaryAssignment:
		return true
	default:
		return false
	}
}

func (l *lowerer) callResultTargetPath(point cfg.Point, fact semantics.CallFact, resultIndex int) (path.Path, bool) {
	for _, target := range l.callSiteResultTargetsFromWIR(point, fact.ResultTargets) {
		if target.ResultIndex() != resultIndex {
			continue
		}
		targetPath := target.TargetPath()
		if targetPath.IsEmpty() {
			continue
		}
		return targetPath, true
	}
	return path.Path{}, false
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
