package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/typ"
)

// This file provides utilities for applying type mutations (field assignments,
// indexer assignments, array mutations) to type overlays during return inference.
//
// When analyzing nested functions, field assignments and mutations performed
// by called functions must be reflected in the types visible to the caller.
// These utilities merge mutation information into type overlays.

// MergeFieldsIntoType merges a set of field types into a base type.
//
// The merge strategy depends on the base type:
//   - nil base: Creates an open record with the given fields
//   - Map base: Creates an open record with map component plus fields
//   - Record base: Adds new fields and joins written types into existing
//     fields, preserving metadata
//   - Other base: Creates an open record with just the fields
//
// Field names are sorted for deterministic output.
func MergeFieldsIntoType(baseType typ.Type, fields map[string]typ.Type) typ.Type {
	return overlaymut.MergeFieldsIntoType(baseType, fields)
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
	overlaymut.ApplyIndexerMergeToOverlay(overlay, indexerAssignments)
}

// JoinValueTypes joins two value types, preferring arrays over empty records.
//
// When {} (empty record) and T[] (array) are joined, the result is T[].
// This models the common Lua pattern of initializing a variable as {} and
// then using it as an array via table.insert or indexed assignment.
// The array type takes precedence because it carries more specific information.
func JoinValueTypes(a, b typ.Type) typ.Type {
	return overlaymut.JoinValueTypes(a, b)
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
	overlaymut.ApplyDirectMutationsToOverlay(overlay, mutations)
}
