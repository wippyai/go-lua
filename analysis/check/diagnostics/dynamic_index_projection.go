package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func assignmentTargetType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (typ.Type, bool) {
	if fact.HasPath && fact.Path.Symbol != 0 && len(fact.Path.Segments) > 0 {
		return newExpressionTyper(result, resolver).typeOf(fact.Target)
	}
	if !fact.HasContainerPath || fact.ContainerPath.Symbol == 0 {
		return nil, false
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return nil, false
	}
	return dynamicIndexAssignmentTargetType(result, resolver, point, attr)
}

func dynamicIndexAssignmentTargetType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	typer := newExpressionTyper(result, resolver)
	if t, ok := typer.typeOf(attr); ok {
		// Runtime reads from a closed record can be nil when a broad key misses;
		// dynamic writes still need the all-fields write contract below.
		if !typ.TypeEquals(t, typ.Nil) {
			return t, true
		}
	}
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := containerFlowType(result, resolver, point, attr.Object)
	if !ok {
		return nil, false
	}
	return dynamicIndexWriteValueType(container, 0)
}

// containerFlowType resolves the flow-sensitive type of a dynamic-index write's
// container expression. The static typer cannot type an unannotated local whose
// shape is only known through flow, so the boundary value is consulted before
// falling back to the static type.
func containerFlowType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, object ast.Expr) (typ.Type, bool) {
	if value, ok := result.ExpressionValueAtBoundary(point, object); ok {
		if t, ok := readmodel.New(result).ValueType(value); ok {
			return t, true
		}
	}
	return newExpressionTyper(result, resolver).typeOf(object)
}

func dynamicIndexWriteValueType(t typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch tt := transparentExpectedType(t).(type) {
	case *typ.Optional:
		return dynamicIndexWriteValueType(tt.Inner, depth+1)
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(members...), true
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.IntersectionForMeet(members...), true
	case *typ.Record:
		if tt.HasMapComponent() && tt.MapValue != nil {
			return tt.MapValue, true
		}
		return closedRecordDynamicWriteValueType(tt)
	case *typ.Map:
		if tt.Value != nil {
			return tt.Value, true
		}
	case *typ.ReadonlyMap:
		if tt.Value != nil {
			return tt.Value, true
		}
	case *typ.Array:
		if tt.Element != nil {
			return tt.Element, true
		}
	}
	return nil, false
}

// closedRecordDynamicWriteValueType resolves the admissible value type for a
// dynamic-key write into a closed record without a map component. A dynamic
// string key could match any declared field, so a sound write must produce a
// value assignable to every field the key could land on: the admissible type is
// the meet of the field domains. Records with no concrete fields carry no
// dynamic-write contract here.
func closedRecordDynamicWriteValueType(record *typ.Record) (typ.Type, bool) {
	if record == nil || record.Open || len(record.Fields) == 0 {
		return nil, false
	}
	members := make([]typ.Type, 0, len(record.Fields))
	for _, field := range record.Fields {
		if field.Type == nil {
			return nil, false
		}
		members = append(members, field.Type)
	}
	if len(members) == 1 {
		return members[0], true
	}
	return normalize.IntersectionForMeet(members...), true
}
