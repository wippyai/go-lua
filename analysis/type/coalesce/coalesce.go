package coalesce

import (
	luatable "github.com/wippyai/go-lua/analysis/lua/table"
	"github.com/wippyai/go-lua/analysis/type/kind"
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
	rest = append(rest, luatable.NewMap(joinTypeSet(joinTypes, keys), joinTypeSet(joinTypes, values)))
	return rest
}

func joinTypeSet(joinTypes TypeSetJoinFunc, types []typ.Type) typ.Type {
	if joinTypes != nil {
		return joinTypes(types...)
	}
	return normalizeUnionForJoin(types)
}

func normalizeUnionForJoin(members []typ.Type) typ.Type {
	flat, hasNil, hasUnknown, hasAny := flattenUnionForJoin(members)
	if hasAny {
		return typ.Any
	}
	if len(flat) > 0 {
		flat = canonicalJoinPolicyMembers(flat)
		flat = applyJoinUnionSubsumption(flat)
	}
	if len(flat) == 0 && hasUnknown {
		if hasNil {
			return typ.NewOptional(typ.Unknown)
		}
		return typ.Unknown
	}
	if hasNil {
		flat = append(flat, typ.Nil)
	}
	return typ.NewUnion(flat...)
}

func flattenUnionForJoin(members []typ.Type) (flat []typ.Type, hasNil, hasUnknown, hasAny bool) {
	flat = make([]typ.Type, 0, len(members))
	var addMember func(typ.Type)
	addMember = func(member typ.Type) {
		if member == nil {
			return
		}
		unwrapped := typ.UnwrapAnnotated(member)
		if unwrapped == nil {
			return
		}
		switch unwrapped.Kind() {
		case kind.Never:
			return
		case kind.Unknown:
			hasUnknown = true
			return
		case kind.Any:
			hasAny = true
			return
		case kind.Nil:
			hasNil = true
			return
		case kind.Union:
			for _, nested := range unwrapped.(*typ.Union).Members {
				addMember(nested)
			}
			return
		case kind.Optional:
			hasNil = true
			addMember(unwrapped.(*typ.Optional).Inner)
			return
		default:
			flat = append(flat, member)
		}
	}
	for _, member := range members {
		addMember(member)
	}
	return flat, hasNil, hasUnknown, hasAny
}

func canonicalJoinPolicyMembers(members []typ.Type) []typ.Type {
	normalized := typ.NewUnion(members...)
	if u, ok := typ.UnwrapAnnotated(normalized).(*typ.Union); ok {
		out := make([]typ.Type, len(u.Members))
		copy(out, u.Members)
		return out
	}
	if normalized == typ.Never {
		return nil
	}
	return []typ.Type{normalized}
}

func applyJoinUnionSubsumption(members []typ.Type) []typ.Type {
	if len(members) < 2 {
		return members
	}
	hasNumberType := false
	for _, member := range members {
		if member.Kind() == kind.Number {
			hasNumberType = true
			break
		}
	}
	if hasNumberType {
		filtered := members[:0]
		for _, member := range members {
			if member.Kind() == kind.Integer {
				continue
			}
			filtered = append(filtered, member)
		}
		members = filtered
	}

	var baseMask uint8
	for _, member := range members {
		switch member.Kind() {
		case kind.String:
			baseMask |= 1
		case kind.Number:
			baseMask |= 2
		case kind.Integer:
			baseMask |= 4
		case kind.Boolean:
			baseMask |= 8
		}
	}
	if baseMask == 0 {
		return members
	}

	filtered := members[:0]
	for _, member := range members {
		if lit, ok := member.(*typ.Literal); ok {
			drop := false
			switch lit.Base {
			case kind.String:
				drop = baseMask&1 != 0
			case kind.Number:
				drop = baseMask&2 != 0
			case kind.Integer:
				drop = baseMask&4 != 0 || baseMask&2 != 0
			case kind.Boolean:
				drop = baseMask&8 != 0
			}
			if drop {
				continue
			}
		}
		filtered = append(filtered, member)
	}
	return filtered
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
		result = append(result, luatable.RebuildRecord(typ.RecordParts{
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
