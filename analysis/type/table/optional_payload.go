package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// fieldsWithOptionalPayloadsSplit returns fields with nilable optional payloads
// split into absent-vs-present table shape.
func fieldsWithOptionalPayloadsSplit(fields []typ.Field) []typ.Field {
	return withOptionalPayloadsSplit(fields, fieldWithOptionalPayloadSplit, sameFieldShape)
}

// withOptionalPayloadsSplit returns items with split applied to each element,
// preserving the original slice (copy-on-change) when every element keeps the
// same shape per sameShape.
func withOptionalPayloadsSplit[T any](items []T, split func(T) T, sameShape func(a, b T) bool) []T {
	var out []T
	for i, item := range items {
		normalized := split(item)
		if out == nil {
			if sameShape(normalized, item) {
				continue
			}
			out = make([]T, 0, len(items))
			out = append(out, items[:i]...)
		}
		out = append(out, normalized)
	}
	if out == nil {
		return items
	}
	return out
}

// fieldWithOptionalPayloadSplit returns field with nilable optional payloads
// split into absent-vs-present table shape.
func fieldWithOptionalPayloadSplit(field typ.Field) typ.Field {
	if inner, ok := splitOptionalEntryPayload(field.Type, field.Optional); ok {
		field.Type = inner
	}
	return field
}

// staticMembersWithOptionalPayloadsSplit returns static members with nilable
// optional payloads split into absent-vs-present table shape.
func staticMembersWithOptionalPayloadsSplit(members []typ.StaticMember) []typ.StaticMember {
	return withOptionalPayloadsSplit(members, staticMemberWithOptionalPayloadSplit, sameStaticMemberShape)
}

// staticMemberWithOptionalPayloadSplit returns member with nilable optional
// payloads split into absent-vs-present table shape.
func staticMemberWithOptionalPayloadSplit(member typ.StaticMember) typ.StaticMember {
	if inner, ok := splitOptionalEntryPayload(member.Type, member.Optional); ok {
		member.Type = inner
	}
	return member
}

func splitOptionalEntryPayload(t typ.Type, optional bool) (typ.Type, bool) {
	if !optional {
		return t, false
	}
	return splitNilableFieldType(t)
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
