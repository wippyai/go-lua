package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) callProducerResultTargets(targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, 0, len(targets))
	for _, target := range targets {
		if lowered, ok := l.callProducerResultTarget(target); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) callProducerResultTarget(target semantics.CallResultTarget) (factflow.CallResultTarget, bool) {
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return factflow.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, target.Name)
		}
		return factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetOrdinaryAssignment:
		if !target.HasSymbol || target.Symbol == 0 {
			return factflow.CallResultTarget{}, false
		}
		if target.HasPath && len(target.Path.Segments) != 0 {
			return factflow.CallResultTarget{}, false
		}
		targetPath := target.Path
		if !target.HasPath {
			targetPath = path.NewPath(target.Symbol, "")
		}
		return factflow.NewCallResultTarget(factflow.CallResultTargetOrdinaryAssignment, target.Index, target.Symbol, targetPath), true
	case semantics.CallResultTargetReturn:
		return factflow.NewCallResultTarget(factflow.CallResultTargetReturn, target.Index, 0, path.Path{}), true
	default:
		return factflow.CallResultTarget{}, false
	}
}

func (l *lowerer) callSiteResultTargets(targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, len(targets))
	for i := range targets {
		out[i] = callSiteResultTarget(targets[i])
	}
	return out
}

func callSiteResultTarget(target semantics.CallResultTarget) factflow.CallResultTarget {
	targetKind := factflow.CallResultTargetUnknown
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		targetKind = factflow.CallResultTargetLocalAssignment
	case semantics.CallResultTargetOrdinaryAssignment:
		targetKind = factflow.CallResultTargetOrdinaryAssignment
	case semantics.CallResultTargetReturn:
		targetKind = factflow.CallResultTargetReturn
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
	return factflow.NewCallResultTarget(targetKind, target.Index, targetSymbol, targetPath)
}
