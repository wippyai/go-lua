package typ

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

var (
	recordMapKeyHash   = hash.FnvString("$mapKey")
	recordMapValueHash = hash.FnvString("$mapValue")
	recordStaticHash   = hash.FnvString("$staticMember")
	freshRecordHash    = hash.FnvString("$freshEmptyTable")
)

// RecordParts carries the structural pieces needed to rebuild a record.
type RecordParts struct {
	Fields        []Field
	StaticMembers []StaticMember
	Metatable     Type
	MapKey        Type
	MapValue      Type
	Open          bool
	Fresh         bool
	AssumeSorted  bool
}

// RebuildRecord rebuilds a record from already-computed structural parts.
func RebuildRecord(parts RecordParts) *Record {
	return buildRecordType(
		parts.Fields,
		parts.StaticMembers,
		parts.Metatable,
		parts.MapKey,
		parts.MapValue,
		parts.Open,
		parts.AssumeSorted,
		parts.Fresh,
	)
}

func buildRecordType(fields []Field, staticMembers []StaticMember, metatable, mapKey, mapValue Type, open bool, assumeSorted bool, fresh bool) *Record {
	sorted := make([]Field, len(fields))
	copy(sorted, fields)
	if !assumeSorted || !fieldsSortedByName(sorted) {
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Name < sorted[j].Name
		})
	}
	for i := range sorted {
		if sorted[i].Type == nil {
			sorted[i].Type = Unknown
		}
	}
	members := make([]StaticMember, len(staticMembers))
	copy(members, staticMembers)
	if !assumeSorted || !staticMembersSorted(members) {
		sort.Slice(members, func(i, j int) bool {
			return CompareStaticMembers(members[i], members[j]) < 0
		})
	}
	for i := range members {
		if members[i].Type == nil {
			members[i].Type = Unknown
		}
	}

	if mapKey == nil && mapValue != nil {
		mapKey = Unknown
	}
	if mapValue == nil && mapKey != nil {
		mapValue = Unknown
	}

	h := uint64(kind.Record)
	for _, f := range sorted {
		h = hash.HashCombine(h, hash.FnvString(f.Name))
		h = hash.HashCombine(h, f.Type.Hash())
		if f.Optional {
			h = hash.HashCombine(h, 1)
		}
		if f.Readonly {
			h = hash.HashCombine(h, 2)
		}
	}
	for _, m := range members {
		h = hash.HashCombine(h, recordStaticHash)
		h = hash.HashCombine(h, uint64(m.Kind))
		switch m.Kind {
		case StaticMemberStringIndex:
			h = hash.HashCombine(h, hash.FnvString(m.Name))
		case StaticMemberIntIndex:
			h = hash.HashCombine(h, uint64(m.Index))
		}
		h = hash.HashCombine(h, m.Type.Hash())
		if m.Optional {
			h = hash.HashCombine(h, 1)
		}
		if m.Readonly {
			h = hash.HashCombine(h, 2)
		}
	}

	if metatable != nil {
		h = hash.HashCombine(h, metatable.Hash())
	}
	if open {
		h = hash.HashCombine(h, 3)
	}
	if mapKey != nil {
		h = hash.HashCombine(h, recordMapKeyHash)
		h = hash.HashCombine(h, mapKey.Hash())
	}
	if mapValue != nil {
		h = hash.HashCombine(h, recordMapValueHash)
		h = hash.HashCombine(h, mapValue.Hash())
	}
	if fresh {
		h = hash.HashCombine(h, freshRecordHash)
	}
	containsAny := knownAnyFields(sorted) || knownAnyStaticMembers(members) || knownAny(metatable, mapKey, mapValue)
	containsNever := knownNeverFields(sorted) || knownNeverStaticMembers(members) || knownNever(metatable, mapKey, mapValue)
	containsTypeParam := knownTypeParamFields(sorted) || knownTypeParamStaticMembers(members) || knownTypeParam(metatable, mapKey, mapValue)
	containsInstantiated := knownInstantiatedFields(sorted) || knownInstantiatedStaticMembers(members) || knownInstantiated(metatable, mapKey, mapValue)
	containsRecursive := knownRecursiveFields(sorted) || knownRecursiveStaticMembers(members) || knownRecursive(metatable, mapKey, mapValue)
	containsOpenRecursive := knownOpenRecursiveFields(sorted) || knownOpenRecursiveStaticMembers(members) || knownOpenRecursive(metatable, mapKey, mapValue)

	return &Record{
		Fields:                sorted,
		StaticMembers:         members,
		Metatable:             metatable,
		MapKey:                mapKey,
		MapValue:              mapValue,
		Open:                  open,
		Fresh:                 fresh,
		sorted:                true,
		hash:                  h,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func fieldsSortedByName(fields []Field) bool {
	for i := 1; i < len(fields); i++ {
		if fields[i-1].Name > fields[i].Name {
			return false
		}
	}
	return true
}
