// Package typenarrow builds the value refinements that a type() comparison
// applies to its subject. The matched and unmatched refinements are shared by
// the static lowering of a literal type name (transferfacts) and the
// flow-sensitive resolution of a value path's runtime type name (factapply), so
// both paths narrow identically.
package typenarrow

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// MatchRefinement narrows a value to the runtime kind named by tag, the matched
// edge of a type() comparison (for example the true edge of type(x) == "number").
func MatchRefinement(reg *axis.Registry, tag runtimekind.Tag) factflow.ValueRefinement {
	value := factflow.NewValueConstraint(product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(tag)))
	if tag == runtimekind.Nil {
		return value.WithConstraint(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Absent()))
	}
	value = value.WithConstraint(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	if scalar, ok := ScalarTypeForTag(tag); ok {
		value = value.WithConstraint(reg, typevalue.WithWitness(reg, product.Top(), scalar))
	}
	return value
}

// UnmatchRefinement narrows a value to exclude the runtime kind named by tag,
// the unmatched edge of a type() comparison.
func UnmatchRefinement(reg *axis.Registry, tag runtimekind.Tag) factflow.ValueRefinement {
	value := factflow.NewValueConstraint(product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Top().Without(tag)))
	if tag == runtimekind.Nil {
		return value.WithConstraint(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	}
	return value
}

// ScalarTypeForTag returns the scalar type witness for a runtime kind tag, for
// the tags whose runtime kind pins a single scalar type.
func ScalarTypeForTag(tag runtimekind.Tag) (typ.Type, bool) {
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
