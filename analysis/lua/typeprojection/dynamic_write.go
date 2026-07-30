package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/internal/typegraph"
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
	return dynamicWriteValueTypeSeen(t, key, &typegraph.Path{})
}

// DynamicWriteNilDeletionAllowed reports whether assigning nil through a
// dynamic index is a deletion operation for every possible matching slot.
func DynamicWriteNilDeletionAllowed(t typ.Type, key typ.Type) bool {
	allowed, ok := dynamicWriteNilDeletionAllowedSeen(t, key, &typegraph.Path{})
	return ok && allowed
}

func dynamicWriteValueTypeSeen(t typ.Type, key typ.Type, active *typegraph.Path) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	if !active.Enter(t, 0) {
		return nil, false
	}
	defer active.Leave(t, 0)
	switch tt := t.(type) {
	case *typ.Annotated:
		return dynamicWriteValueTypeSeen(tt.Inner, key, active)
	case *typ.Alias:
		return dynamicWriteValueTypeSeen(tt.UnaliasedTarget(), key, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return nil, false
		}
		return dynamicWriteValueTypeSeen(tt.Body, key, active)
	case *typ.Instantiated:
		next := shallowExpandWriteInstantiated(tt)
		if next == nil || next == t {
			return nil, false
		}
		return dynamicWriteValueTypeSeen(next, key, active)
	case *typ.Optional:
		return dynamicWriteValueTypeSeen(tt.Inner, key, active)
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicWriteValueTypeSeen(member, key, active)
			if ok {
				members = append(members, value)
			}
		}
		return dynamicWriteMeet(members)
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicWriteValueTypeSeen(member, key, active)
			if ok {
				members = append(members, value)
			}
		}
		return dynamicWriteMeet(members)
	case *typ.Record:
		if tt.HasMapComponent() && tt.MapValue != nil {
			return tt.MapValue, true
		}
		return closedRecordDynamicWriteValueTypeSeen(tt, key, &typegraph.Path{})
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

func dynamicWriteNilDeletionAllowedSeen(t typ.Type, key typ.Type, active *typegraph.Path) (bool, bool) {
	if t == nil {
		return false, false
	}
	if !active.Enter(t, 0) {
		return false, false
	}
	defer active.Leave(t, 0)
	switch tt := t.(type) {
	case *typ.Annotated:
		return dynamicWriteNilDeletionAllowedSeen(tt.Inner, key, active)
	case *typ.Alias:
		return dynamicWriteNilDeletionAllowedSeen(tt.UnaliasedTarget(), key, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return false, false
		}
		return dynamicWriteNilDeletionAllowedSeen(tt.Body, key, active)
	case *typ.Instantiated:
		next := shallowExpandWriteInstantiated(tt)
		if next == nil || next == t {
			return false, false
		}
		return dynamicWriteNilDeletionAllowedSeen(next, key, active)
	case *typ.Optional:
		return dynamicWriteNilDeletionAllowedSeen(tt.Inner, key, active)
	case *typ.Union:
		saw := false
		for _, member := range tt.Members {
			allowed, ok := dynamicWriteNilDeletionAllowedSeen(member, key, active)
			if !ok {
				continue
			}
			saw = true
			if !allowed {
				return false, true
			}
		}
		return true, saw
	case *typ.Intersection:
		saw := false
		for _, member := range tt.Members {
			allowed, ok := dynamicWriteNilDeletionAllowedSeen(member, key, active)
			if !ok {
				continue
			}
			saw = true
			if !allowed {
				return false, true
			}
		}
		return true, saw
	case *typ.Record:
		if tt.HasMapComponent() && tt.MapValue != nil {
			return true, true
		}
		value, ok := closedRecordDynamicWriteValueTypeSeen(tt, key, &typegraph.Path{})
		return ok && typevalue.TypeIncludesNil(value), ok
	case *typ.Map:
		return tt.Value != nil, tt.Value != nil
	case *typ.ReadonlyMap:
		return tt.Value != nil, tt.Value != nil
	case *typ.Array:
		return tt.Element != nil, tt.Element != nil
	}
	return false, false
}

func closedRecordDynamicWriteValueTypeSeen(record *typ.Record, key typ.Type, active *typegraph.Path) (typ.Type, bool) {
	if record == nil || record.Open || (len(record.Fields) == 0 && len(record.StaticMembers) == 0) {
		return nil, false
	}
	if key == nil || typ.IsAny(key) || typ.IsUnknown(key) {
		return allClosedRecordDynamicWriteTypes(record)
	}
	if !active.Enter(key, 0) {
		return nil, false
	}
	defer active.Leave(key, 0)
	switch kk := key.(type) {
	case *typ.Annotated:
		return closedRecordDynamicWriteValueTypeSeen(record, kk.Inner, active)
	case *typ.Alias:
		return closedRecordDynamicWriteValueTypeSeen(record, kk.UnaliasedTarget(), active)
	case *typ.Recursive:
		if kk.Body == nil || kk.Body == key {
			return nil, false
		}
		return closedRecordDynamicWriteValueTypeSeen(record, kk.Body, active)
	case *typ.Instantiated:
		next := shallowExpandWriteInstantiated(kk)
		if next == nil || next == key {
			return nil, false
		}
		return closedRecordDynamicWriteValueTypeSeen(record, next, active)
	case *typ.Optional:
		return closedRecordDynamicWriteValueTypeSeen(record, kk.Inner, active)
	case *typ.Union:
		var members []typ.Type
		for _, member := range kk.Members {
			value, ok := closedRecordDynamicWriteValueTypeSeen(record, member, active)
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
