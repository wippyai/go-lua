package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
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
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := containerFlowType(result, resolver, point, attr.Object)
	if !ok {
		return nil, false
	}
	key, _ := newExpressionTyper(result, resolver).typeOf(attr.Key)
	return dynamicIndexWriteValueType(container, key, 0)
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

func dynamicIndexWriteValueType(t typ.Type, key typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch tt := transparentExpectedType(t).(type) {
	case *typ.Optional:
		return dynamicIndexWriteValueType(tt.Inner, key, depth+1)
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, key, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		return dynamicIndexWriteMeet(members)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, key, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		return dynamicIndexWriteMeet(members)
	case *typ.Record:
		if tt.HasMapComponent() && tt.MapValue != nil {
			return normalize.Optional(tt.MapValue), true
		}
		return closedRecordDynamicWriteValueType(tt, key, depth+1)
	case *typ.Map:
		if tt.Value != nil {
			return normalize.Optional(tt.Value), true
		}
	case *typ.ReadonlyMap:
		if tt.Value != nil {
			return normalize.Optional(tt.Value), true
		}
	case *typ.Array:
		if tt.Element != nil {
			return normalize.Optional(tt.Element), true
		}
	}
	return nil, false
}

func dynamicIndexWriteMeet(members []typ.Type) (typ.Type, bool) {
	if len(members) == 0 {
		return nil, false
	}
	if len(members) == 1 {
		return members[0], members[0] != nil
	}
	return normalize.IntersectionForMeet(members...), true
}

// closedRecordDynamicWriteValueType resolves the admissible value type for a
// dynamic-key write into a closed record without a map component. The write
// contract is the meet of every declared slot the key may hit. Key alternatives
// that miss the record impose no value contract; a broad or unknown key may hit
// any declared slot. Records with no concrete slots carry no dynamic-write
// contract here.
func closedRecordDynamicWriteValueType(record *typ.Record, key typ.Type, depth int) (typ.Type, bool) {
	if record == nil || record.Open || (len(record.Fields) == 0 && len(record.StaticMembers) == 0) {
		return nil, false
	}
	if key == nil || typ.IsAny(key) || typ.IsUnknown(key) {
		return allClosedRecordDynamicWriteTypes(record)
	}
	if depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch kk := transparentExpectedType(key).(type) {
	case *typ.Optional:
		return closedRecordDynamicWriteValueType(record, kk.Inner, depth+1)
	case *typ.Union:
		var members []typ.Type
		for _, member := range kk.Members {
			value, ok := closedRecordDynamicWriteValueType(record, member, depth+1)
			if ok {
				members = append(members, value)
			}
		}
		return dynamicIndexWriteMeet(members)
	default:
		if name, ok := literalStringKey(kk); ok {
			return closedRecordStringKeyWriteType(record, name)
		}
		if index, ok := literalIntKey(kk); ok {
			return closedRecordIntKeyWriteType(record, index)
		}
		if subtype.IsSubtype(kk, typ.String) {
			return closedRecordStringKeyWriteTypes(record)
		}
		if subtype.IsSubtype(kk, typ.Integer) || subtype.IsSubtype(kk, typ.Number) {
			return closedRecordIntKeyWriteTypes(record)
		}
	}
	return nil, false
}

func allClosedRecordDynamicWriteTypes(record *typ.Record) (typ.Type, bool) {
	members := make([]typ.Type, 0, len(record.Fields))
	for _, field := range record.Fields {
		if value := recordFieldWriteType(field); value != nil {
			members = append(members, value)
		}
	}
	for _, member := range record.StaticMembers {
		if value := recordStaticMemberWriteType(member); value != nil {
			members = append(members, value)
		}
	}
	return dynamicIndexWriteMeet(members)
}

func closedRecordStringKeyWriteTypes(record *typ.Record) (typ.Type, bool) {
	members := make([]typ.Type, 0, len(record.Fields)+len(record.StaticMembers))
	for _, field := range record.Fields {
		if value := recordFieldWriteType(field); value != nil {
			members = append(members, value)
		}
	}
	for _, member := range record.StaticMembers {
		if member.Kind == typ.StaticMemberStringIndex {
			if value := recordStaticMemberWriteType(member); value != nil {
				members = append(members, value)
			}
		}
	}
	return dynamicIndexWriteMeet(members)
}

func closedRecordIntKeyWriteTypes(record *typ.Record) (typ.Type, bool) {
	var members []typ.Type
	for _, member := range record.StaticMembers {
		if member.Kind == typ.StaticMemberIntIndex {
			if value := recordStaticMemberWriteType(member); value != nil {
				members = append(members, value)
			}
		}
	}
	return dynamicIndexWriteMeet(members)
}

func closedRecordStringKeyWriteType(record *typ.Record, name string) (typ.Type, bool) {
	if field := record.GetField(name); field != nil {
		return recordFieldWriteType(*field), field.Type != nil
	}
	if member := record.GetStaticStringIndex(name); member != nil {
		return recordStaticMemberWriteType(*member), member.Type != nil
	}
	return nil, false
}

func closedRecordIntKeyWriteType(record *typ.Record, index int64) (typ.Type, bool) {
	if member := record.GetStaticIntIndex(index); member != nil {
		return recordStaticMemberWriteType(*member), member.Type != nil
	}
	return nil, false
}

func recordFieldWriteType(field typ.Field) typ.Type {
	if field.Type == nil {
		return nil
	}
	if field.Optional {
		return normalize.Optional(field.Type)
	}
	return field.Type
}

func recordStaticMemberWriteType(member typ.StaticMember) typ.Type {
	if member.Type == nil {
		return nil
	}
	if member.Optional {
		return normalize.Optional(member.Type)
	}
	return member.Type
}

func literalStringKey(t typ.Type) (string, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

func literalIntKey(t typ.Type) (int64, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		return 0, false
	}
	index, ok := lit.Value.(int64)
	return index, ok
}
