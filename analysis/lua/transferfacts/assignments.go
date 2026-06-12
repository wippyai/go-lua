package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) localAssignment(fact semantics.LocalAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, fact.Name)
	source := l.valueSource(fact.Source)
	if declaredValueApplies(fact) {
		if declared, ok := l.declaredValue(fact.Type); ok {
			return factflow.NewRootAssignmentWithDeclaredValue(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source, declared), true
		}
	}
	return factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source), true
}

func declaredValueApplies(fact semantics.LocalAssignmentFact) bool {
	if fact.Type == nil || fact.Source.Kind != factflow.ValueSourceExpression {
		return false
	}
	if _, ok := valueexpr.LiteralType(fact.Expr); !ok {
		return false
	}
	return true
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
	return factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, fact.Symbol, target, l.valueSource(fact.Source)), true
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

func (l *lowerer) declaredValue(expr ast.TypeExpr) (product.Value, bool) {
	if expr == nil {
		return product.Value{}, false
	}
	t, ok := newTypeResolver(l.bindings).Type(expr)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.FromType(l.registry, t), true
}
