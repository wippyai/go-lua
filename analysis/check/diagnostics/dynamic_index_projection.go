package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
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
	if attr, ok := dynamicAssignmentTargetAttr(fact.Target); ok {
		return dynamicIndexAssignmentTargetType(result, resolver, point, attr)
	}
	if fact.HasPath && fact.Path.Symbol != 0 && len(fact.Path.Segments) > 0 {
		return staticPathAssignmentTargetType(result, resolver, point, fact)
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

func dynamicAssignmentTargetAttr(target ast.Expr) (*ast.AttrGetExpr, bool) {
	attr, ok := assignmentTargetAttr(target)
	if !ok || attr == nil || attr.KeySyntax != ast.AttrKeyIndex || attr.Key == nil {
		return nil, false
	}
	return attr, ast.KeyName(attr.Key) == ""
}

func staticPathAssignmentTargetType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (typ.Type, bool) {
	attr, ok := assignmentTargetAttr(fact.Target)
	if !ok || attr == nil || attr.Object == nil || attr.Key == nil {
		return declaredPathType(result, resolver, fact.Target)
	}
	name := ast.KeyName(attr.Key)
	if name == "" {
		return declaredPathType(result, resolver, fact.Target)
	}
	container, ok := declaredPathType(result, resolver, attr.Object)
	if !ok {
		return nil, false
	}
	want, ok := staticIndexWriteValueType(container, typ.LiteralString(name), 0)
	if ok && impossibleLiteralWriteMeet(want) {
		if declared, declaredOK := declaredPathType(result, resolver, fact.Target); declaredOK && finiteLiteralDomainType(declared) {
			return declared, true
		}
	}
	return want, ok
}

func dynamicIndexAssignmentTargetType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	key, _ := newExpressionTyper(result, resolver).typeOf(attr.Key)
	if declared, ok := declaredPathType(result, resolver, attr.Object); ok {
		if want, ok := dynamicIndexWriteValueType(declared, key, 0); ok {
			return want, true
		}
	}
	container, ok := containerFlowType(result, resolver, point, attr.Object)
	if !ok {
		return nil, false
	}
	return dynamicIndexWriteValueType(container, key, 0)
}

// containerFlowType resolves the flow-sensitive type of a dynamic-index write's
// container expression. The static typer cannot type an unannotated local whose
// shape is only known through flow, so the boundary value is consulted before
// falling back to the static type.
func containerFlowType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, object ast.Expr) (typ.Type, bool) {
	query := newDiagnosticQuery(result)
	if value, ok := query.ExpressionValueBeforeBoundary(point, object); ok {
		if t, ok := query.ValueType(value); ok {
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

func impossibleLiteralWriteMeet(t typ.Type) bool {
	intersection, ok := transparentExpectedType(t).(*typ.Intersection)
	if !ok || len(intersection.Members) < 2 {
		return false
	}
	var base kind.Kind
	var value any
	haveLiteral := false
	for _, member := range intersection.Members {
		lit, ok := transparentExpectedType(member).(*typ.Literal)
		if !ok {
			return false
		}
		if !haveLiteral {
			base = lit.Base
			value = lit.Value
			haveLiteral = true
			continue
		}
		if lit.Base == base && lit.Value != value {
			return true
		}
	}
	return false
}

func finiteLiteralDomainType(t typ.Type) bool {
	switch tt := transparentExpectedType(t).(type) {
	case *typ.Literal:
		return true
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if _, ok := transparentExpectedType(member).(*typ.Literal); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func staticIndexWriteValueType(t typ.Type, key typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch tt := transparentExpectedType(t).(type) {
	case *typ.Optional:
		return staticIndexWriteValueType(tt.Inner, key, depth+1)
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := staticIndexWriteValueType(member, key, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		return dynamicIndexWriteMeet(members)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := staticIndexWriteValueType(member, key, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		return dynamicIndexWriteMeet(members)
	default:
		return dynamicIndexWriteValueType(tt, key, depth)
	}
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
