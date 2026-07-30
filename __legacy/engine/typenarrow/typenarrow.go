// Package typenarrow builds the value refinements that a type() comparison
// applies to its subject. The matched and unmatched refinements are shared by
// the static lowering of a literal type name (transferfacts) and the
// flow-sensitive resolution of a value path's runtime type name (factapply), so
// both paths narrow identically.
package typenarrow

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// RuntimeKindTagForType resolves a type that denotes a type-name string literal
// (the "number" in type(v) == "number") to its runtime kind tag. It reports
// false for any type that is not a string literal naming a known runtime kind.
func RuntimeKindTagForType(t typ.Type) (runtimekind.Tag, bool) {
	lit, ok := unwrap.Annotated(unwrap.Alias(t)).(*typ.Literal)
	if !ok {
		return 0, false
	}
	name, ok := lit.Value.(string)
	if !ok {
		return 0, false
	}
	return runtimekind.ParseTag(name)
}

// MatchRefinement narrows a value to the runtime kind named by tag, the matched
// edge of a type() comparison (for example the true edge of type(x) == "number").
func MatchRefinement(reg *axis.Registry, tag runtimekind.Tag) factflow.ValueRefinement {
	value := factflow.NewValueConstraint(product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(tag)))
	value = value.WithConstraint(reg, product.Set(reg, product.Top(), assertion.Key, assertion.Runtime()))
	if tag == runtimekind.Nil {
		return value.WithConstraint(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Absent()))
	}
	value = value.WithConstraint(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
	if staticType, ok := StaticTypeForTag(tag); ok {
		value = value.WithConstraint(reg, typevalue.WithWitness(reg, product.Top(), staticType))
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

// StaticTypeForTag returns the source-level type witness implied by a runtime
// kind tag when the checker has a sound top type for that runtime family.
func StaticTypeForTag(tag runtimekind.Tag) (typ.Type, bool) {
	switch tag {
	case runtimekind.String:
		return typ.String, true
	case runtimekind.Number:
		return typ.Number, true
	case runtimekind.Boolean:
		return typ.Boolean, true
	case runtimekind.Table:
		return typetable.BuiltinTopMarker(), true
	default:
		return nil, false
	}
}
