package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// NewMap creates a map type with table-key normalization.
func NewMap(key, value typ.Type) *typ.Map {
	return typ.RebuildMap(NormalizeKey(key), value)
}

// NewReadonlyMap creates a read-only map type with table-key normalization.
func NewReadonlyMap(key, value typ.Type) *typ.ReadonlyMap {
	return typ.RebuildReadonlyMap(NormalizeKey(key), value)
}

// RebuildRecord rebuilds a record with table-key normalization for its map
// component and nil/absence normalization for optional field/static-member
// payloads.
func RebuildRecord(parts typ.RecordParts) *typ.Record {
	return typ.RebuildRecord(recordPartsWithTableNormalization(parts))
}
