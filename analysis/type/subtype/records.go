package subtype

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (c *checker) checkRecord(sub, super *typ.Record, depth int) bool {
	for _, sf := range super.Fields {
		subMember, ok := recordReadableField(sub, sf.Name)
		if !ok {
			if !sf.Optional && !unwrap.IsOptionalLike(sf.Type) {
				return false
			}
			continue
		}
		if !c.checkRecordMember(subMember, recordMemberShape{typ: sf.Type, optional: sf.Optional, readonly: sf.Readonly}, depth+1) {
			return false
		}
	}
	for _, sm := range super.StaticMembers {
		subMember, ok := recordReadableStaticMember(sub, sm)
		if !ok {
			if !sm.Optional && !unwrap.IsOptionalLike(sm.Type) {
				return false
			}
			continue
		}
		if !c.checkRecordMember(subMember, recordMemberShape{typ: sm.Type, optional: sm.Optional, readonly: sm.Readonly}, depth+1) {
			return false
		}
	}

	if super.HasMapComponent() {
		if !sub.HasMapComponent() {
			return false
		}
		if !c.check(sub.MapKey, super.MapKey, depth+1) {
			return false
		}
		if !c.check(sub.MapValue, super.MapValue, depth+1) {
			return false
		}
	}
	return c.metaSubtype(sub.Metatable, super.Metatable, depth+1)
}

type recordMemberShape struct {
	typ      typ.Type
	optional bool
	readonly bool
}

func recordReadableField(rec *typ.Record, name string) (recordMemberShape, bool) {
	if rec == nil || name == "" {
		return recordMemberShape{}, false
	}
	if field := rec.GetField(name); field != nil {
		return fieldShape(field), true
	}
	if member := rec.GetStaticStringIndex(name); member != nil {
		return staticMemberShape(member), true
	}
	return recordMemberShape{}, false
}

func recordReadableStaticMember(rec *typ.Record, member typ.StaticMember) (recordMemberShape, bool) {
	if rec == nil {
		return recordMemberShape{}, false
	}
	if found := rec.GetStaticMember(member.Kind, member.Name, member.Index); found != nil {
		return staticMemberShape(found), true
	}
	if member.Kind == typ.StaticMemberStringIndex && member.Name != "" {
		if field := rec.GetField(member.Name); field != nil {
			return fieldShape(field), true
		}
	}
	return recordMemberShape{}, false
}

func fieldShape(field *typ.Field) recordMemberShape {
	if field == nil {
		return recordMemberShape{}
	}
	return recordMemberShape{typ: field.Type, optional: field.Optional, readonly: field.Readonly}
}

func staticMemberShape(member *typ.StaticMember) recordMemberShape {
	if member == nil {
		return recordMemberShape{}
	}
	return recordMemberShape{typ: member.Type, optional: member.Optional, readonly: member.Readonly}
}

func (c *checker) checkRecordMember(sub, super recordMemberShape, depth int) bool {
	if super.optional && sub.typ != nil && sub.typ.Kind() == kind.Nil {
		return true
	}
	effectiveSuper := super.typ
	if super.optional {
		effectiveSuper = typeexpr.Optional(super.typ)
	}
	if super.readonly {
		if !c.check(sub.typ, effectiveSuper, depth+1) {
			return false
		}
	} else {
		if sub.readonly {
			return false
		}
		if !c.check(sub.typ, effectiveSuper, depth+1) {
			return false
		}
		if !c.check(effectiveSuper, sub.typ, depth+1) && !c.canWidenTo(sub.typ, effectiveSuper, depth+1) {
			return false
		}
	}
	if !super.optional && !unwrap.IsOptionalLike(super.typ) && sub.optional {
		return false
	}
	return true
}

func (c *checker) metaSubtype(subMT, superMT typ.Type, depth int) bool {
	if subMT == nil && superMT == nil {
		return true
	}
	subUnconstrained := subMT != nil && typetable.IsMetatableUnconstrained(subMT)
	superUnconstrained := superMT != nil && typetable.IsMetatableUnconstrained(superMT)
	if subUnconstrained && (superMT == nil || superUnconstrained) {
		return true
	}
	if superUnconstrained || (superMT != nil && typ.IsUnknown(superMT)) {
		return true
	}
	if subMT != nil && typ.IsUnknown(subMT) {
		return false
	}
	if subUnconstrained {
		return false
	}
	if subMT == nil || superMT == nil {
		return false
	}
	return c.check(subMT, superMT, depth)
}

func (c *checker) checkRecordToInterface(sub *typ.Record, super *typ.Interface, depth int) bool {
	if sub == nil || super == nil {
		return false
	}
	for _, method := range super.Methods {
		field := sub.GetField(method.Name)
		if field == nil {
			return false
		}
		methodType := subst.Self(method.Type, sub)
		if !c.check(field.Type, methodType, depth+1) {
			return false
		}
	}
	return true
}
