package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// fieldsWithOptionalPayloadsSplit returns fields with nilable optional payloads
// split into absent-vs-present table shape.
func fieldsWithOptionalPayloadsSplit(fields []typ.Field) []typ.Field {
	var out []typ.Field
	for i, field := range fields {
		normalized := fieldWithOptionalPayloadSplit(field)
		if out == nil {
			if sameFieldShape(normalized, field) {
				continue
			}
			out = make([]typ.Field, 0, len(fields))
			out = append(out, fields[:i]...)
		}
		out = append(out, normalized)
	}
	if out == nil {
		return fields
	}
	return out
}

// fieldWithOptionalPayloadSplit returns field with nilable optional payloads
// split into absent-vs-present table shape.
func fieldWithOptionalPayloadSplit(field typ.Field) typ.Field {
	if !field.Optional {
		return field
	}
	if inner, optional := splitNilableFieldType(field.Type); optional {
		field.Type = inner
		field.Optional = true
	}
	return field
}

// staticMembersWithOptionalPayloadsSplit returns static members with nilable
// optional payloads split into absent-vs-present table shape.
func staticMembersWithOptionalPayloadsSplit(members []typ.StaticMember) []typ.StaticMember {
	var out []typ.StaticMember
	for i, member := range members {
		normalized := staticMemberWithOptionalPayloadSplit(member)
		if out == nil {
			if sameStaticMemberShape(normalized, member) {
				continue
			}
			out = make([]typ.StaticMember, 0, len(members))
			out = append(out, members[:i]...)
		}
		out = append(out, normalized)
	}
	if out == nil {
		return members
	}
	return out
}

// staticMemberWithOptionalPayloadSplit returns member with nilable optional
// payloads split into absent-vs-present table shape.
func staticMemberWithOptionalPayloadSplit(member typ.StaticMember) typ.StaticMember {
	if !member.Optional {
		return member
	}
	if inner, optional := splitNilableFieldType(member.Type); optional {
		member.Type = inner
		member.Optional = true
	}
	return member
}

func sameFieldShape(a, b typ.Field) bool {
	return a.Name == b.Name &&
		a.Optional == b.Optional &&
		a.Readonly == b.Readonly &&
		typ.SameNode(a.Type, b.Type)
}

func sameStaticMemberShape(a, b typ.StaticMember) bool {
	return a.Kind == b.Kind &&
		a.Name == b.Name &&
		a.Index == b.Index &&
		a.Optional == b.Optional &&
		a.Readonly == b.Readonly &&
		typ.SameNode(a.Type, b.Type)
}
