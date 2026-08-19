package typ

import (
	"sort"

	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
)

var (
	recordMapKeyHash   = hash.FnvString("$mapKey")
	recordMapValueHash = hash.FnvString("$mapValue")
	recordStaticHash   = hash.FnvString("$staticMember")
)

// RecordParts carries the structural pieces needed to rebuild a record.
type RecordParts struct {
	Fields        []Field
	StaticMembers []StaticMember
	Metatable     Type
	MapKey        Type
	MapValue      Type
	Open          bool
	AssumeSorted  bool
}

// RebuildRecord rebuilds a record from already-computed structural parts.
func RebuildRecord(parts RecordParts) *Record {
	return newCanonicalRecord(
		parts.Fields,
		parts.StaticMembers,
		parts.Metatable,
		parts.MapKey,
		parts.MapValue,
		parts.Open,
		parts.AssumeSorted,
	)
}

// typ owns hash-stable node materialization; table/normalize decide the record's semantic shape.
func newCanonicalRecord(fields []Field, staticMembers []StaticMember, metatable, mapKey, mapValue Type, open bool, assumeSorted bool) *Record {
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
		h = hash.MixHash(h, hash.FnvString(f.Name))
		h = hash.MixHash(h, f.Type.Hash())
		if f.Optional {
			h = hash.MixHash(h, 1)
		}
		if f.Readonly {
			h = hash.MixHash(h, 2)
		}
	}
	for _, m := range members {
		h = hash.MixHash(h, recordStaticHash)
		h = hash.MixHash(h, uint64(m.Kind))
		switch m.Kind {
		case StaticMemberStringIndex:
			h = hash.MixHash(h, hash.FnvString(m.Name))
		case StaticMemberIntIndex:
			h = hash.MixHash(h, uint64(m.Index))
		}
		h = hash.MixHash(h, m.Type.Hash())
		if m.Optional {
			h = hash.MixHash(h, 1)
		}
		if m.Readonly {
			h = hash.MixHash(h, 2)
		}
	}

	if metatable != nil {
		h = hash.MixHash(h, metatable.Hash())
	}
	if open {
		h = hash.MixHash(h, 3)
	}
	if mapKey != nil {
		h = hash.MixHash(h, recordMapKeyHash)
		h = hash.MixHash(h, mapKey.Hash())
	}
	if mapValue != nil {
		h = hash.MixHash(h, recordMapValueHash)
		h = hash.MixHash(h, mapValue.Hash())
	}
	props := typePropertiesOfFields(sorted)
	props.includeStaticMembers(members)
	props.includeTypes(metatable, mapKey, mapValue)

	zzProbeConstruct(uint64(kind.Record), h) // ZZPROBE
	return &Record{
		Fields:            sorted,
		StaticMembers:     members,
		Metatable:         metatable,
		MapKey:            mapKey,
		MapValue:          mapValue,
		Open:              open,
		sorted:            true,
		hash:              h,
		equalityHashCache: &equalityHashCache{},
		typeProperties:    props,
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
