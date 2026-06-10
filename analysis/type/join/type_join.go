// Package join provides type joining operations for phi node merging.
//
// When control flow merges at phi nodes, the join package computes the
// resulting type from multiple incoming branches. The join operation
// produces a type that is a supertype of all incoming types.
//
// # Coalescing Rules
//
// The join package applies several coalescing rules before creating unions:
//
//   - Empty records are removed when maps are present
//   - Open and closed records are unified to open records
//   - Records with identical field signatures merge their map components
//   - Multiple maps merge into a single map with joined key/value types
//   - Unknown types are filtered out (they don't contribute information)
//
// These rules ensure that join results are as precise as possible while
// remaining sound supertypes of all inputs.
//
// # Usage
//
// Call Types with the types from each incoming branch:
//
//	result := join.Types(branchA, branchB, branchC)
//
// For single-type or identical inputs, Types returns the input directly
// without creating unnecessary unions.
package join

import (
	"github.com/wippyai/go-lua/analysis/type/gradual"
	"github.com/wippyai/go-lua/analysis/type/relation"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Types computes the union of multiple types for phi node joining.
// Coalesces empty records with container shapes and merges multiple maps into one.
// Filters out unknown types since they don't contribute information to the join.
func Types(types ...typ.Type) typ.Type {
	if len(types) == 0 {
		return nil
	}
	if len(types) == 1 {
		return types[0]
	}

	// Filter out nil and unknown types before processing
	filtered := filterUnknown(types)
	if len(filtered) == 0 {
		// All types were unknown/nil, return unknown
		return typ.Unknown
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	filtered = FlattenUnions(filtered)
	if len(filtered) == 1 {
		return filtered[0]
	}

	first := filtered[0]
	allEqual := true
	for i := 1; i < len(filtered); i++ {
		if !sameJoinInput(first, filtered[i]) {
			allEqual = false
			break
		}
	}
	if allEqual {
		return first
	}
	beforeCoalesce := len(filtered)
	filtered = relation.CoalesceJoinProducts(filtered, Types)
	if len(filtered) < beforeCoalesce {
		filtered = dedupeJoinInputs(filtered)
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return gradual.PruneSoftUnionMembers(relation.PruneLessPreciseRefinableUnionMembers(relation.NormalizeUnionForJoin(filtered...)))
}

// FlattenUnions exposes explicit union members before coalescing. This keeps
// incremental joins equivalent to batch joins: joining (A|B) with C must give
// record/map/recursive-family coalescers the same member set as joining
// A, B, C in one call.
func FlattenUnions(types []typ.Type) []typ.Type {
	if len(types) < 2 {
		return types
	}
	var out []typ.Type
	changed := false
	var appendType func(typ.Type)
	appendType = func(t typ.Type) {
		if u, ok := t.(*typ.Union); ok {
			changed = true
			for _, member := range u.Members {
				appendType(member)
			}
			return
		}
		if t != nil {
			out = append(out, t)
		}
	}
	for _, t := range types {
		appendType(t)
	}
	if !changed {
		return types
	}
	return dedupeJoinInputs(out)
}

func dedupeJoinInputs(types []typ.Type) []typ.Type {
	return relation.DedupeJoinInputs(types)
}

func sameJoinInput(a, b typ.Type) bool {
	return relation.SameJoinInput(a, b)
}

// CoalesceEmptyRecordWithArray removes empty records when arrays are present.
//
// Lua uses the same table runtime value for map-like and list-like tables. During
// flow joins, a common pattern is {} on one path and array growth on another
// (table.insert). Keeping {} in the union loses sequence intent and creates
// downstream nil-index noise. When an array shape is present, prefer it.
func CoalesceEmptyRecordWithArray(types []typ.Type) []typ.Type {
	return relation.CoalesceEmptyRecordWithArray(types)
}

// filterUnknown removes nil and unknown types from the slice.
//
// Unknown types represent complete lack of information and don't contribute
// to a join. Including them would make the join result imprecise (Unknown
// unions with anything stays Unknown-ish).
//
// Nil types can appear from uninitialized paths and are similarly filtered.
func filterUnknown(types []typ.Type) []typ.Type {
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if typ.IsAbsentOrUnknown(t) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// CoalesceMaps merges multiple map types into a single map with unioned key/value types.
//
// When joining branches that each produce a map type, rather than creating
// a union of maps, we create a single map with the union of key types and
// union of value types. This is more precise and matches Lua semantics.
//
// Example: Joining {[string]: number} and {[string]: boolean} produces
// {[string]: number|boolean}, not {[string]: number} | {[string]: boolean}.
//
// Non-map types in the input are preserved unchanged in the result.
func CoalesceMaps(types []typ.Type) []typ.Type {
	return relation.CoalesceMaps(types, Types)
}

// CoalesceRecordOpenness converts closed records to open when joining with open records.
//
// In Lua, a closed record {x: number, y: string} can flow to code expecting an
// open record {x: number, ...}. When branches produce different record variants,
// we open all closed records to allow the join to represent "at least these fields."
//
// This prevents spurious errors when one branch produces a known-complete record
// and another produces a record that might have additional fields.
//
// If all records are closed or all are open, no transformation is needed.
func CoalesceRecordOpenness(types []typ.Type) []typ.Type {
	return relation.CoalesceRecordOpenness(types)
}

// CoalesceEmptyRecordWithMap removes empty records when maps are present.
//
// In Lua, {} (empty table) is a common initial value that gets populated later.
// When joining {} with a map type, the empty record is redundant since the map
// already represents "table with dynamic keys." Keeping the empty record would
// create an imprecise union.
//
// Example: Joining {} and {[string]: number} produces {[string]: number}, not
// {} | {[string]: number}.
func CoalesceEmptyRecordWithMap(types []typ.Type) []typ.Type {
	return relation.CoalesceEmptyRecordWithMap(types)
}
