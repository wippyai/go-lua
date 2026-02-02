package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// This file provides utilities for applying type mutations (field assignments,
// indexer assignments, array mutations) to type overlays during return inference.
//
// When analyzing nested functions, field assignments and mutations performed
// by called functions must be reflected in the types visible to the caller.
// These utilities merge mutation information into type overlays.

// MergeFieldAssignments merges src into dst.
//
// Field assignments from different sources (different called functions,
// different branches) are merged using join.Two to produce union types.
// This ensures that all possible field types are captured.
func MergeFieldAssignments(
	dst map[cfg.SymbolID]map[string]typ.Type,
	src map[cfg.SymbolID]map[string]typ.Type,
) {
	for _, sym := range cfg.SortedSymbolIDs(src) {
		fields := src[sym]
		if dst[sym] == nil {
			dst[sym] = make(map[string]typ.Type)
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			fieldType := fields[name]
			if existing := dst[sym][name]; existing != nil {
				dst[sym][name] = typ.JoinPreferNonSoft(existing, fieldType)
			} else {
				dst[sym][name] = fieldType
			}
		}
	}
}

// ApplyFieldMergeToOverlay merges collected field assignments into symbol types in the overlay.
//
// For each symbol with collected field assignments, this function:
//  1. Looks up the symbol's current type in the overlay
//  2. Merges the assigned fields into that type using MergeFieldsIntoType
//  3. Updates the overlay with the enriched type
//
// This enables field assignments from called nested functions to be reflected
// in the types visible during parent function analysis.
func ApplyFieldMergeToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	fieldAssignments map[cfg.SymbolID]map[string]typ.Type,
) {
	for _, sym := range cfg.SortedSymbolIDs(fieldAssignments) {
		fields := fieldAssignments[sym]
		if len(fields) == 0 {
			continue
		}
		baseType := overlay[sym]
		merged := MergeFieldsIntoType(baseType, fields)
		if merged != nil {
			overlay[sym] = merged
		}
	}
}

// MergeFieldsIntoType merges a set of field types into a base type.
//
// The merge strategy depends on the base type:
//   - nil base: Creates an open record with the given fields
//   - Map base: Creates an open record with map component plus fields
//   - Record base: Adds new fields, preserving existing fields and metadata
//   - Other base: Creates an open record with just the fields
//
// Field names are sorted for deterministic output. Existing record fields
// are preserved (not overwritten) since they represent more precise type info.
func MergeFieldsIntoType(baseType typ.Type, fields map[string]typ.Type) typ.Type {
	if len(fields) == 0 {
		return baseType
	}

	fieldNames := cfg.SortedFieldNames(fields)

	if baseType == nil {
		// No base type - create a fresh record with just the fields
		builder := typ.NewRecord().SetOpen(true)
		for _, name := range fieldNames {
			builder.Field(name, fields[name])
		}
		return builder.Build()
	}

	switch v := baseType.(type) {
	case *typ.Map:
		// Map base: create Record(open) with MapComponent + merged fields
		builder := typ.NewRecord().SetOpen(true)
		builder.MapComponent(v.Key, v.Value)
		for _, name := range fieldNames {
			builder.Field(name, fields[name])
		}
		return builder.Build()

	case *typ.Record:
		// Build merged record: existing fields + new fields
		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}
		existing := make(map[string]bool)
		for _, f := range v.Fields {
			builder.Field(f.Name, f.Type)
			existing[f.Name] = true
		}
		for _, name := range fieldNames {
			if !existing[name] {
				builder.Field(name, fields[name])
			}
		}
		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if v.HasMapComponent() {
			builder.MapComponent(v.MapKey, v.MapValue)
		}
		return builder.Build()

	default:
		// Base is not a record or map; create one with just the field assignments
		builder := typ.NewRecord().SetOpen(true)
		for _, name := range fieldNames {
			builder.Field(name, fields[name])
		}
		return builder.Build()
	}
}

// ApplyIndexerMergeToOverlay adds map components to symbol types based on dynamic index assignments.
//
// Dynamic index assignments (t[k] = v where k is not a literal) indicate
// map-like behavior. This function collects all indexer assignments for each
// symbol, joins the key and value types, and adds a map component to the
// symbol's type.
//
// Key types are joined across all assignments; if all keys are numbers, the
// result is a numeric map. Value types are joined with special handling:
// empty records {} are replaced by arrays when array elements are assigned.
func ApplyIndexerMergeToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	indexerAssignments map[cfg.SymbolID][]mutator.IndexerInfo,
) {
	for _, sym := range cfg.SortedSymbolIDs(indexerAssignments) {
		infos := indexerAssignments[sym]
		if len(infos) == 0 {
			continue
		}

		// Join all key types and value types, preferring array types over empty records
		var keyType, valType typ.Type
		for _, info := range infos {
			keyType = typ.JoinPreferNonSoft(keyType, info.KeyType)
			valType = JoinValueTypes(valType, info.ValType)
		}
		if keyType == nil {
			keyType = typ.String
		}
		if valType == nil {
			valType = typ.Unknown
		}

		baseType := overlay[sym]
		merged := MergeMapComponentIntoType(baseType, keyType, valType)
		if merged != nil {
			overlay[sym] = merged
		}
	}
}

// JoinValueTypes joins two value types, preferring arrays over empty records.
//
// When {} (empty record) and T[] (array) are joined, the result is T[].
// This models the common Lua pattern of initializing a variable as {} and
// then using it as an array via table.insert or indexed assignment.
// The array type takes precedence because it carries more specific information.
func JoinValueTypes(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	// Check if one is an empty record and the other is an array
	aIsEmptyRecord := unwrap.IsEmptyRecord(a)
	bIsEmptyRecord := unwrap.IsEmptyRecord(b)
	_, aIsArray := a.(*typ.Array)
	_, bIsArray := b.(*typ.Array)
	aIsPlaceholder := a.Kind().IsPlaceholder()
	bIsPlaceholder := b.Kind().IsPlaceholder()

	// Prefer array over empty record
	if aIsEmptyRecord && bIsArray {
		return b
	}
	if bIsEmptyRecord && aIsArray {
		return a
	}

	// Prefer array over placeholder (unknown/any) when array evidence exists.
	if aIsPlaceholder && bIsArray {
		return b
	}
	if bIsPlaceholder && aIsArray {
		return a
	}

	return typ.JoinPreferNonSoft(a, b)
}

// MergeMapComponentIntoType adds a map component to a base type.
//
// The merge strategy depends on the base type:
//   - nil base: Creates a new Map type
//   - Map base: Joins key and value types with existing map types
//   - Record base: Adds/updates the map component while preserving fields
//   - Other base: Creates a new Map type
//
// This is used when dynamic index assignments are detected, indicating the
// variable is used as a map or has map-like access patterns.
func MergeMapComponentIntoType(baseType, keyType, valType typ.Type) typ.Type {
	if baseType == nil {
		return typ.NewMap(keyType, valType)
	}

	switch v := baseType.(type) {
	case *typ.Map:
		newKey := typ.JoinPreferNonSoft(v.Key, keyType)
		newVal := typ.JoinPreferNonSoft(v.Value, valType)
		return typ.NewMap(newKey, newVal)

	case *typ.Record:
		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}
		for _, f := range v.Fields {
			builder.Field(f.Name, f.Type)
		}
		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if v.HasMapComponent() {
			newKey := typ.JoinPreferNonSoft(v.MapKey, keyType)
			newVal := typ.JoinPreferNonSoft(v.MapValue, valType)
			builder.MapComponent(newKey, newVal)
		} else {
			builder.MapComponent(keyType, valType)
		}
		return builder.Build()

	default:
		return typ.NewMap(keyType, valType)
	}
}

// ApplyDirectMutationsToOverlay widens array element types based on table.insert mutations.
//
// When table.insert(t, v) is called, the array element type of t should include
// the type of v. This function applies such mutations by widening the element
// type of each affected symbol's type.
//
// This is separate from field assignments because table.insert modifies the
// array portion of a table, not named fields.
func ApplyDirectMutationsToOverlay(
	overlay map[cfg.SymbolID]typ.Type,
	mutations map[cfg.SymbolID]typ.Type,
) {
	for _, sym := range cfg.SortedSymbolIDs(mutations) {
		elemType := mutations[sym]
		if elemType == nil {
			continue
		}
		baseType := overlay[sym]
		merged := flow.WidenArrayElementType(baseType, elemType, typ.JoinPreferNonSoft)
		if merged != nil {
			overlay[sym] = merged
		}
	}
}
