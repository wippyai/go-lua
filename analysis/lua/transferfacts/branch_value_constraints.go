package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func refinementHasPresence(refinement factflow.ValueRefinement, want presence.Value) bool {
	constraint, ok := refinement.Constraint()
	if !ok {
		return false
	}
	return presence.Equal(product.PresenceOf(constraint), want)
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
