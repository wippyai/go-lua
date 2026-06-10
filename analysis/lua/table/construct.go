package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// NewMap creates a map type with table-key normalization.
func NewMap(key, value typ.Type) *typ.Map {
	return typ.NewMap(NormalizeKey(key), value)
}

// NewReadonlyMap creates a read-only map type with table-key normalization.
func NewReadonlyMap(key, value typ.Type) *typ.ReadonlyMap {
	return typ.NewReadonlyMap(NormalizeKey(key), value)
}

// RebuildRecord rebuilds a record with table-key normalization for its map
// component.
func RebuildRecord(parts typ.RecordParts) *typ.Record {
	return typ.RebuildRecord(RecordPartsWithMapKeyNormalization(parts))
}

// RecordPartsWithMapKeyNormalization returns a copy of parts with the map key
// normalized when a map component key is present.
func RecordPartsWithMapKeyNormalization(parts typ.RecordParts) typ.RecordParts {
	if parts.MapKey != nil {
		parts.MapKey = NormalizeKey(parts.MapKey)
	}
	return parts
}
