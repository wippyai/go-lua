package relation

import (
	luatable "github.com/wippyai/go-lua/analysis/lua/table"
	"github.com/wippyai/go-lua/analysis/type/kind"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// TypeSetJoinFunc joins nested slot types while relation-owned product
// coalescers decide which product alternatives should collapse.
type TypeSetJoinFunc func(...Type) Type

// SameJoinInput reports whether two inputs are equivalent under the relation
// policy used by type joins.
func SameJoinInput(a, b Type) bool {
	if SameUnionMember(a, b) {
		return true
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		return sameProductFamily(a, b)
	}
	return false
}

// DedupeJoinInputs removes duplicate inputs under SameJoinInput while avoiding
// structural hashing for compound, non-recursive values.
func DedupeJoinInputs(types []Type) []Type {
	if len(types) < 2 {
		return types
	}
	seen := make(map[uint64][]Type, len(types))
	identity := make(map[Type]struct{})
	out := make([]Type, 0, len(types))
	changed := false
	for _, t := range types {
		if t == nil {
			changed = true
			continue
		}
		if !ContainsRecursive(t) && !joinDedupeUsesStructuralEquality(t) {
			if _, ok := identity[t]; ok {
				changed = true
				continue
			}
			identity[t] = struct{}{}
			out = append(out, t)
			continue
		}
		hash := UnionMemberHash(t)
		duplicate := false
		for _, existing := range seen[hash] {
			if SameJoinInput(existing, t) {
				duplicate = true
				changed = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[hash] = append(seen[hash], t)
		out = append(out, t)
	}
	if !changed {
		return types
	}
	return out
}

func joinDedupeUsesStructuralEquality(t Type) bool {
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Literal, kind.Self:
		return true
	default:
		return false
	}
}

// CoalesceJoinProducts applies the relation-owned product-family coalescing
// policy used before constructing a flow-join union.
func CoalesceJoinProducts(types []Type, joinTypes TypeSetJoinFunc) []Type {
	types = CoalesceEmptyRecordWithMap(types)
	types = CoalesceEmptyRecordWithArray(types)
	types = CoalesceMaps(types, joinTypes)
	types = CoalesceRecursiveRecordFamilies(types)
	types = CoalesceCompatibleRecords(types)
	types = CoalesceRecordOpenness(types)
	types = CoalesceMaps(types, joinTypes)
	return types
}

// CoalesceEmptyRecordWithArray removes empty records when arrays are present.
func CoalesceEmptyRecordWithArray(types []Type) []Type {
	hasEmptyRecord := false
	hasArray := false
	for _, t := range types {
		if isEmptyRecordNoMap(t) {
			hasEmptyRecord = true
			continue
		}
		if isArrayLike(t) {
			hasArray = true
		}
	}
	if !hasEmptyRecord || !hasArray {
		return types
	}
	result := make([]Type, 0, len(types))
	for _, t := range types {
		if !isEmptyRecordNoMap(t) {
			result = append(result, t)
		}
	}
	return result
}

// CoalesceMaps merges multiple map types into a single map with joined key and
// value slots. The caller supplies the slot join so flow joins keep their own
// recursive orchestration while relation owns the map coalescing policy.
func CoalesceMaps(types []Type, joinTypes TypeSetJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}

	var maps []*Map
	rest := make([]Type, 0, len(types))
	for _, t := range types {
		if t == nil {
			continue
		}
		if m, ok := t.(*Map); ok {
			maps = append(maps, m)
			continue
		}
		rest = append(rest, t)
	}

	if len(maps) <= 1 {
		return types
	}

	keys := make([]Type, 0, len(maps))
	values := make([]Type, 0, len(maps))
	for _, m := range maps {
		keys = append(keys, m.Key)
		values = append(values, m.Value)
	}
	rest = append(rest, luatable.NewMap(joinTypeSet(joinTypes, keys), joinTypeSet(joinTypes, values)))
	return rest
}

func joinTypeSet(joinTypes TypeSetJoinFunc, types []Type) Type {
	if joinTypes != nil {
		return joinTypes(types...)
	}
	return NormalizeUnionForJoin(types...)
}

// CoalesceRecordOpenness converts closed records to open when joining with open
// records.
func CoalesceRecordOpenness(types []Type) []Type {
	hasOpen := false
	hasClosed := false
	for _, t := range types {
		if r, ok := t.(*Record); ok {
			if r.Open {
				hasOpen = true
			} else {
				hasClosed = true
			}
		}
	}
	if !hasOpen || !hasClosed {
		return types
	}
	result := make([]Type, 0, len(types))
	for _, t := range types {
		r, ok := t.(*Record)
		if !ok || r.Open {
			result = append(result, t)
			continue
		}
		result = append(result, luatable.RebuildRecord(RecordParts{
			Fields:        r.Fields,
			StaticMembers: r.StaticMembers,
			Metatable:     r.Metatable,
			MapKey:        r.MapKey,
			MapValue:      r.MapValue,
			Open:          true,
			AssumeSorted:  true,
		}))
	}
	return result
}

// CoalesceEmptyRecordWithMap removes empty records when map-like alternatives
// are present.
func CoalesceEmptyRecordWithMap(types []Type) []Type {
	hasEmptyRecord := false
	hasMap := false
	for _, t := range types {
		if isEmptyRecordNoMap(t) {
			hasEmptyRecord = true
		}
		if t != nil && (t.Kind() == kind.Map || t.Kind() == kind.ReadonlyMap) {
			hasMap = true
		}
	}
	if !hasEmptyRecord || !hasMap {
		return types
	}
	result := make([]Type, 0, len(types))
	for _, t := range types {
		if !isEmptyRecordNoMap(t) {
			result = append(result, t)
		}
	}
	return result
}
