package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// recordPartsWithTableNormalization returns a copy of parts with Lua table
// construction policy applied.
func recordPartsWithTableNormalization(parts typ.RecordParts) typ.RecordParts {
	parts = recordPartsWithMapKeyNormalization(parts)
	parts.Fields = fieldsWithOptionalPayloadsSplit(parts.Fields)
	parts.StaticMembers = staticMembersWithOptionalPayloadsSplit(parts.StaticMembers)
	return parts
}

// recordPartsWithMapKeyNormalization returns a copy of parts with the map key
// normalized when a map component key is present.
func recordPartsWithMapKeyNormalization(parts typ.RecordParts) typ.RecordParts {
	if parts.MapKey != nil {
		parts.MapKey = NormalizeKey(parts.MapKey)
	}
	return parts
}
