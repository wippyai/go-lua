package coalesce

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TypeSetJoinFunc joins nested slot types while callers decide the surrounding
// product-family orchestration.
type TypeSetJoinFunc func(...typ.Type) typ.Type

// CoalesceEmptyRecordWithArray removes empty records when arrays are present.
func CoalesceEmptyRecordWithArray(types []typ.Type) []typ.Type {
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
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if !isEmptyRecordNoMap(t) {
			result = append(result, t)
		}
	}
	return result
}

// CoalesceMaps merges multiple map types into a single map with joined key and
// value slots. The caller supplies the slot join so flow joins keep their own
// recursive orchestration while this package owns the map coalescing policy.
func CoalesceMaps(types []typ.Type, joinTypes TypeSetJoinFunc) []typ.Type {
	if len(types) < 2 {
		return types
	}

	var maps []*typ.Map
	rest := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if t == nil {
			continue
		}
		if m, ok := t.(*typ.Map); ok {
			maps = append(maps, m)
			continue
		}
		rest = append(rest, t)
	}

	if len(maps) <= 1 {
		return types
	}

	keys := make([]typ.Type, 0, len(maps))
	values := make([]typ.Type, 0, len(maps))
	for _, m := range maps {
		keys = append(keys, m.Key)
		values = append(values, m.Value)
	}
	rest = append(rest, typetable.NewMap(joinTypeSet(joinTypes, keys), joinTypeSet(joinTypes, values)))
	return rest
}

func joinTypeSet(joinTypes TypeSetJoinFunc, types []typ.Type) typ.Type {
	if joinTypes != nil {
		return joinTypes(types...)
	}
	return normalize.UnionForJoin(types...)
}

// CoalesceRecordOpenness converts closed records to open when joining with open
// records.
func CoalesceRecordOpenness(types []typ.Type) []typ.Type {
	hasOpen := false
	hasClosed := false
	for _, t := range types {
		if r, ok := t.(*typ.Record); ok {
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
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		r, ok := t.(*typ.Record)
		if !ok || r.Open {
			result = append(result, t)
			continue
		}
		result = append(result, typetable.RebuildRecord(typ.RecordParts{
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
func CoalesceEmptyRecordWithMap(types []typ.Type) []typ.Type {
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
	result := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if !isEmptyRecordNoMap(t) {
			result = append(result, t)
		}
	}
	return result
}

// PreferArrayOverEmptyRecord returns the array-like side when the other side is
// an empty record without a map component.
func PreferArrayOverEmptyRecord(a, b typ.Type) (typ.Type, bool) {
	if isEmptyRecordNoMap(a) && isArrayLike(b) {
		return b, true
	}
	if isEmptyRecordNoMap(b) && isArrayLike(a) {
		return a, true
	}
	return nil, false
}

func isEmptyRecordNoMap(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return isEmptyRecordNoMap(v.Target)
	case *typ.Record:
		return len(v.Fields) == 0 && len(v.StaticMembers) == 0 && !v.HasMapComponent()
	default:
		return false
	}
}

func isArrayLike(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return isArrayLike(v.Target)
	case *typ.Array:
		return true
	default:
		return false
	}
}
