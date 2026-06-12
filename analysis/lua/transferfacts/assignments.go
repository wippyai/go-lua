package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) localAssignment(fact semantics.LocalAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, fact.Name)
	return factflow.NewRootAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) ordinaryAssignment(fact semantics.OrdinaryAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := fact.Path
	if !fact.HasPath {
		target = path.NewPath(fact.Symbol, "")
	}
	if len(target.Segments) != 0 {
		return factflow.RootAssignment{}, false
	}
	return factflow.NewRootAssignment(fact.Symbol, target, l.valueSource(fact.Source)), true
}

func (l *lowerer) pathAssignment(fact semantics.OrdinaryAssignmentFact) (factflow.PathAssignment, bool) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return factflow.PathAssignment{}, false
	}
	return factflow.NewPathAssignment(fact.Path, l.valueSource(fact.Source)), true
}

func (l *lowerer) pathDescendantInvalidation(fact semantics.OrdinaryAssignmentFact) (factflow.PathDescendantInvalidation, bool) {
	if fact.HasPath || !fact.HasContainerPath || fact.ContainerPath.Symbol == 0 {
		return factflow.PathDescendantInvalidation{}, false
	}
	return factflow.NewPathDescendantInvalidation(fact.ContainerPath), true
}
