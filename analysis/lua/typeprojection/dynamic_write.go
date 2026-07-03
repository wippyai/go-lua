package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// DynamicWriteValueType resolves the value contract for a dynamic-index write.
// For maps and arrays it returns the declared element/member contract. For a
// closed record without a map component, a broad key may hit any declared slot,
// so the admissible value must satisfy the meet of every possible slot type.
func DynamicWriteValueType(t typ.Type, key typ.Type) (typ.Type, bool) {
	return dynamicWriteValueType(t, key, 0)
}

func dynamicWriteValueType(t typ.Type, key typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch tt := transparentWriteType(t).(type) {
	case *typ.Optional:
		return dynamicWriteValueType(tt.Inner, key, depth+1)
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicWriteValueType(member, key, depth+1)
			if ok {
				members = append(members, value)
			}
		}
		return dynamicWriteMeet(members)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicWriteValueType(member, key, depth+1)
			if ok {
				members = append(members, value)
			}
		}
		return dynamicWriteMeet(members)
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
	switch kk := transparentWriteType(key).(type) {
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
		return dynamicWriteMeet(members)
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
	members := make([]typ.Type, 0, len(record.Fields)+len(record.StaticMembers))
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
	return dynamicWriteMeet(members)
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
	return dynamicWriteMeet(members)
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
	return dynamicWriteMeet(members)
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

func dynamicWriteMeet(members []typ.Type) (typ.Type, bool) {
	if len(members) == 0 {
		return nil, false
	}
	if len(members) == 1 {
		return members[0], members[0] != nil
	}
	return normalize.IntersectionForMeet(members...), true
}

func transparentWriteType(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return typ.Unknown
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Recursive:
			if tt.Body == nil || tt.Body == t {
				return t
			}
			t = tt.Body
		case *typ.Instantiated:
			next := shallowExpandWriteInstantiated(tt)
			if next == nil || next == t {
				return t
			}
			t = next
		default:
			return t
		}
	}
	return t
}

func shallowExpandWriteInstantiated(inst *typ.Instantiated) typ.Type {
	if inst == nil || inst.Generic == nil || inst.Generic.Body == nil || len(inst.TypeArgs) != len(inst.Generic.TypeParams) {
		return inst
	}
	body := subst.Params(inst.Generic.Body, inst.Generic.TypeParams, inst.TypeArgs)
	if body == nil {
		return inst
	}
	return subst.Self(body, inst)
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
