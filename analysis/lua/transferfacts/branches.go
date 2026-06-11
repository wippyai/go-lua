package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchRefinement(fact semantics.BranchConditionFact) (factflow.BranchRefinement, bool) {
	target := fact.Check.Path
	if target.IsEmpty() {
		return factflow.BranchRefinement{}, false
	}
	switch fact.Check.Kind {
	case branchcond.CheckNil:
		return factflow.NewBranchRefinement(
			target,
			l.presenceRefinement(presence.Absent()), true,
			l.presenceRefinement(presence.Present()), true,
		), true
	case branchcond.CheckNotNil:
		return factflow.NewBranchRefinement(
			target,
			l.presenceRefinement(presence.Present()), true,
			l.presenceRefinement(presence.Absent()), true,
		), true
	case branchcond.CheckTruthy:
		return factflow.NewBranchRefinement(
			target,
			l.presenceRefinement(presence.Present()), true,
			factflow.ValueRefinement{}, false,
		), true
	case branchcond.CheckFalsy:
		return factflow.NewBranchRefinement(
			target,
			factflow.ValueRefinement{}, false,
			l.presenceRefinement(presence.Present()), true,
		), true
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		return l.typeBranchRefinement(target, fact.Check.Kind, fact.Check.TypeName)
	default:
		return factflow.BranchRefinement{}, false
	}
}

func (l *lowerer) typeBranchRefinement(target path.Path, kind branchcond.CheckKind, typeName string) (factflow.BranchRefinement, bool) {
	tag, ok := runtimekind.ParseTag(typeName)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	matched := l.typeMatchedRefinement(tag)
	unmatched := l.typeUnmatchedRefinement(tag)
	if kind == branchcond.CheckTypeNot {
		return factflow.NewBranchRefinement(target, unmatched, true, matched, true), true
	}
	return factflow.NewBranchRefinement(target, matched, true, unmatched, true), true
}

func (l *lowerer) typeMatchedRefinement(tag runtimekind.Tag) factflow.ValueRefinement {
	value := l.runtimeKindRefinement(runtimekind.Singleton(tag))
	if tag == runtimekind.Nil {
		return value.WithConstraint(l.registry, l.presenceConstraint(presence.Absent()))
	}
	return value.WithConstraint(l.registry, l.presenceConstraint(presence.Present()))
}

func (l *lowerer) typeUnmatchedRefinement(tag runtimekind.Tag) factflow.ValueRefinement {
	value := l.runtimeKindRefinement(runtimekind.Top().Without(tag))
	if tag == runtimekind.Nil {
		return value.WithConstraint(l.registry, l.presenceConstraint(presence.Present()))
	}
	return value
}

func (l *lowerer) presenceRefinement(value presence.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(l.presenceConstraint(value))
}

func (l *lowerer) presenceConstraint(value presence.Value) product.Value {
	return product.NewWithPresence(l.registry, product.ShapeTop, value)
}

func (l *lowerer) runtimeKindRefinement(value runtimekind.Value) factflow.ValueRefinement {
	return factflow.NewValueConstraint(l.runtimeKindConstraint(value))
}

func (l *lowerer) runtimeKindConstraint(value runtimekind.Value) product.Value {
	return product.Set(l.registry, product.Top(), runtimekind.Key, value)
}
