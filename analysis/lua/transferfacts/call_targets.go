package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type callResultTargetLoweringPolicy uint8

const (
	callResultTargetProducerStrict callResultTargetLoweringPolicy = iota
	callResultTargetCallSiteEvidence
)

func (l *lowerer) strictProducerResultTargets(targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, 0, len(targets))
	for _, target := range targets {
		if lowered, ok := lowerCallResultTarget(target, callResultTargetProducerStrict); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) evidenceCallSiteResultTargets(targets []semantics.CallResultTarget) []factflow.CallResultTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]factflow.CallResultTarget, len(targets))
	for i := range targets {
		out[i], _ = lowerCallResultTarget(targets[i], callResultTargetCallSiteEvidence)
	}
	return out
}

func lowerCallResultTarget(target semantics.CallResultTarget, policy callResultTargetLoweringPolicy) (factflow.CallResultTarget, bool) {
	targetKind := factflow.CallResultTargetUnknown
	switch target.Kind {
	case semantics.CallResultTargetLocalAssignment:
		targetKind = factflow.CallResultTargetLocalAssignment
	case semantics.CallResultTargetOrdinaryAssignment:
		targetKind = factflow.CallResultTargetOrdinaryAssignment
	case semantics.CallResultTargetReturn:
		return factflow.NewCallResultTarget(factflow.CallResultTargetReturn, target.Index, target.ResultIndex, 0, path.Path{}), true
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
	if policy == callResultTargetProducerStrict {
		switch targetKind {
		case factflow.CallResultTargetLocalAssignment:
			if targetSymbol == 0 {
				return factflow.CallResultTarget{}, false
			}
		case factflow.CallResultTargetOrdinaryAssignment:
			if targetSymbol == 0 || len(targetPath.Segments) != 0 {
				return factflow.CallResultTarget{}, false
			}
		default:
			return factflow.CallResultTarget{}, false
		}
	}
	return factflow.NewCallResultTarget(targetKind, target.Index, target.ResultIndex, targetSymbol, targetPath), true
}
