package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
			l.typedPresenceRefinement(target, presence.Present()), true,
		), true
	case branchcond.CheckNotNil:
		return factflow.NewBranchRefinement(
			target,
			l.typedPresenceRefinement(target, presence.Present()), true,
			l.presenceRefinement(presence.Absent()), true,
		), true
	case branchcond.CheckTruthy:
		return factflow.NewBranchRefinement(
			target,
			l.typedPresenceRefinement(target, presence.Present()), true,
			factflow.ValueRefinement{}, false,
		), true
	case branchcond.CheckFalsy:
		return factflow.NewBranchRefinement(
			target,
			factflow.ValueRefinement{}, false,
			l.typedPresenceRefinement(target, presence.Present()), true,
		), true
	case branchcond.CheckLiteralEqual, branchcond.CheckLiteralNot:
		return l.literalBranchRefinement(target, fact.Check.Kind, fact.Check.LiteralString)
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		return l.typeBranchRefinement(target, fact.Check.Kind, fact.Check.TypeName)
	default:
		return factflow.BranchRefinement{}, false
	}
}

func (l *lowerer) branchRefinements(fact semantics.BranchConditionFact) []factflow.BranchRefinement {
	if fact.Check.Kind != branchcond.CheckNone {
		if lowered := l.branchRefinementsForCheck(fact.Check); len(lowered) != 0 {
			return lowered
		}
		return nil
	}
	var out []factflow.BranchRefinement
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchEdgeRefinements(check, true)...)
	}
	for _, check := range branchcond.FalsyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchEdgeRefinements(check, false)...)
	}
	return out
}

func (l *lowerer) branchRefinementsForCheck(check branchcond.Check) []factflow.BranchRefinement {
	refinement, ok := l.branchRefinement(semantics.BranchConditionFact{Check: check})
	if !ok {
		return nil
	}
	out := l.rootRefinementsForBranchRefinement(refinement)
	out = append(out, l.truthyBooleanRootRefinements(check)...)
	out = append(out, refinement)
	return out
}

func (l *lowerer) branchEdgeRefinements(check branchcond.Check, cond bool) []factflow.BranchRefinement {
	refinement, ok := l.branchEdgeRefinement(check, cond)
	if !ok {
		return nil
	}
	out := l.rootRefinementsForBranchRefinement(refinement)
	if check.Kind == branchcond.CheckTruthy || check.Kind == branchcond.CheckFalsy {
		out = append(out, l.truthyBooleanRootRefinementOnEdge(check, cond)...)
	}
	out = append(out, refinement)
	return out
}

func (l *lowerer) branchEdgeRefinement(check branchcond.Check, cond bool) (factflow.BranchRefinement, bool) {
	refinement, ok := l.branchRefinement(semantics.BranchConditionFact{Check: check})
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	value, ok := refinement.ValueForEdge(cond)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	if cond {
		return factflow.NewBranchRefinement(refinement.TargetPath(), value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(refinement.TargetPath(), factflow.ValueRefinement{}, false, value, true), true
}

func (l *lowerer) rootRefinementsForBranchRefinement(refinement factflow.BranchRefinement) []factflow.BranchRefinement {
	target := refinement.TargetPath()
	var out []factflow.BranchRefinement
	if value, ok := refinement.TrueValue(); ok && refinementHasPresence(value, presence.Present()) {
		if root, ok := l.rootPresenceRefinement(target, true); ok {
			out = append(out, root)
		}
	}
	if value, ok := refinement.FalseValue(); ok && refinementHasPresence(value, presence.Present()) {
		if root, ok := l.rootPresenceRefinement(target, false); ok {
			out = append(out, root)
		}
	}
	return out
}

func (l *lowerer) truthyBooleanRootRefinements(check branchcond.Check) []factflow.BranchRefinement {
	switch check.Kind {
	case branchcond.CheckTruthy:
		var out []factflow.BranchRefinement
		if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(true), true); ok {
			out = append(out, root)
		}
		if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(false), false); ok {
			out = append(out, root)
		}
		return out
	case branchcond.CheckFalsy:
		var out []factflow.BranchRefinement
		if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(false), true); ok {
			out = append(out, root)
		}
		if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(true), false); ok {
			out = append(out, root)
		}
		return out
	default:
		return nil
	}
}

func (l *lowerer) truthyBooleanRootRefinementOnEdge(check branchcond.Check, cond bool) []factflow.BranchRefinement {
	switch check.Kind {
	case branchcond.CheckTruthy:
		if cond {
			if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(true), true); ok {
				return []factflow.BranchRefinement{root}
			}
		} else {
			if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(false), false); ok {
				return []factflow.BranchRefinement{root}
			}
		}
	case branchcond.CheckFalsy:
		if cond {
			if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(false), true); ok {
				return []factflow.BranchRefinement{root}
			}
		} else {
			if root, ok := l.rootLiteralRefinement(check.Path, typ.LiteralBool(true), false); ok {
				return []factflow.BranchRefinement{root}
			}
		}
	}
	return nil
}

func (l *lowerer) branchPathRelations(fact semantics.BranchConditionFact) (factflow.BranchPathRelationSet, bool) {
	left := fact.Check.Path
	right := fact.Check.OtherPath
	if left.IsEmpty() || right.IsEmpty() {
		return factflow.BranchPathRelationSet{}, false
	}
	switch fact.Check.Kind {
	case branchcond.CheckPathEqual:
		return factflow.NewBranchPathRelationSet(
			factflow.NewBranchPathEquality(left, right, true, false),
			factflow.NewBranchPathInequality(left, right, false, true),
		), true
	case branchcond.CheckPathNot:
		return factflow.NewBranchPathRelationSet(
			factflow.NewBranchPathInequality(left, right, true, false),
			factflow.NewBranchPathEquality(left, right, false, true),
		), true
	default:
		return factflow.BranchPathRelationSet{}, false
	}
}

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
