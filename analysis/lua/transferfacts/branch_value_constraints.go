package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	value = value.WithConstraint(l.registry, l.presenceConstraint(presence.Present()))
	if scalar, ok := scalarTypeForTag(tag); ok {
		value = value.WithConstraint(l.registry, typevalue.WithWitness(l.registry, product.Top(), scalar))
	}
	return value
}

// scalarTypeForTag returns the concrete primitive type denoted by a runtime
// type() tag, for the tags whose runtime kind pins a single scalar type. It
// supplies the type witness a `type(x) == "T"` guard proves on its true edge,
// so a gradual `any` subject narrows to T rather than staying opaque.
func scalarTypeForTag(tag runtimekind.Tag) (typ.Type, bool) {
	switch tag {
	case runtimekind.String:
		return typ.String, true
	case runtimekind.Number:
		return typ.Number, true
	case runtimekind.Boolean:
		return typ.Boolean, true
	default:
		return nil, false
	}
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

func (l *lowerer) falsyAbsentRefinement() factflow.ValueRefinement {
	return factflow.NewFalsyAbsentConstraint(l.presenceConstraint(presence.Absent()))
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

func (l *lowerer) boolLiteralRefinement(value bool) factflow.ValueRefinement {
	lit := typ.LiteralBool(value)
	return factflow.NewValueConstraint(typevalue.WithWitness(l.registry, typevalue.FromType(l.registry, lit), lit))
}
