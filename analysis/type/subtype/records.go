package subtype

import "github.com/wippyai/go-lua/analysis/type/typ"

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
