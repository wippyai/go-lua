package interproc

import (
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/postflow"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldKey identifies a statically-known field/index slot in interprocedural
// product state. Boundary collectors may still speak in source field names, but
// the product lattice stores typed path segments so field identity is not an
// ad-hoc string carrier.
type FieldKey = postflow.FieldKey

// FieldValues is the canonical product carrier for field-indexed facts.
type FieldValues = postflow.FieldValues

// FieldKeyFromName lowers a source field name into the product field key.
func FieldKeyFromName(name string) (FieldKey, bool) {
	return fieldkey.FromName(name)
}

// FieldKeyStringKey projects a product field key back to a runtime Lua string
// key. Dot fields and bracket-string members both address string table keys;
// integer indexes are rejected by this projection.
func FieldKeyStringKey(key FieldKey) (string, bool) {
	return fieldkey.StringKeyFromSegment(key)
}

// SortedFieldKeys returns field keys in a deterministic order for product
// operations. The ordering is structural, not display-string based, so field and
// string-index segments remain distinct inside the lattice.
func SortedFieldKeys[T any](m map[FieldKey]T) []FieldKey {
	return fieldkey.Sorted(m)
}

// LiftTypeFieldMap admits boundary field-name facts into the typed product
// carrier.
func LiftTypeFieldMap(fields map[string]typ.Type) FieldValues {
	if len(fields) == 0 {
		return nil
	}
	out := make(FieldValues, len(fields))
	for name, fieldType := range fields {
		key, ok := FieldKeyFromName(name)
		if !ok {
			continue
		}
		out[key] = product.FromType(fieldType)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ProjectValueFieldMap projects typed product field facts back to the existing
// boundary shape consumed by store/nested APIs.
func ProjectValueFieldMap(fields FieldValues) map[string]typ.Type {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(fields))
	for _, key := range SortedFieldKeys(fields) {
		name, ok := FieldKeyStringKey(key)
		if !ok {
			continue
		}
		out[name] = fields[key].ProjectValue()
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
